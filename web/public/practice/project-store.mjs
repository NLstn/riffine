// Host persistence only. Go owns project encoding, migrations and validation.
export const MAX_DOCUMENT_BYTES = 1048576;
const prefix = 'browser:';
const storeName = 'documents';
const fail = (code, message) => Object.assign(new Error(message), {code});
const storageError = error => {
 if (typeof error?.code === 'string') return error;
 if (error?.name === 'QuotaExceededError') return fail('storage_quota', 'Device storage is full. Download your project, then remove saved copies or free browser storage.');
 if (error?.name === 'AbortError') return fail('cancelled', 'Device save cancelled.');
 return fail('storage_unavailable', `Device storage is unavailable. Download your project to keep it. ${error?.message ?? ''}`.trim());
};
function handle(value) {
 if (typeof value !== 'string' || !/^browser:[a-zA-Z0-9-]{1,80}(?:\.recovery-[a-f0-9]{64})?$/.test(value)) throw fail('invalid_handle', 'Invalid device project handle.');
 return value;
}
async function record(id, document) {
 handle(id);
 if (typeof document !== 'string' || !document || new TextEncoder().encode(document).length > MAX_DOCUMENT_BYTES) throw fail('document_limit', 'Project is missing or exceeds 1 MiB.');
 const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(document));
 const revision = Array.from(new Uint8Array(digest), byte => byte.toString(16).padStart(2, '0')).join('');
 // Metadata is for the saved-copy list only; it never accepts a project for use.
 // Opening always decodes and verifies the complete document in the shared core.
 const project = JSON.parse(document);
 return {id, document, revision, name: typeof project.name === 'string' ? project.name : 'Practice project', sourceName: typeof project.track?.path === 'string' ? project.track.path : 'Original audio', savedAt: Date.now()};
}
export class ProjectStore {
 constructor({indexedDB = globalThis.indexedDB, database = 'riffine-projects-v1'} = {}) { this.factory = indexedDB; this.database = database; }
 async connect(signal) {
  if (!this.factory) throw fail('storage_unavailable', 'Device saves are unavailable in this browser. Download your project to keep it.');
  if (signal?.aborted) throw fail('cancelled', 'Device operation cancelled.');
  return new Promise((resolve, reject) => {
   const request = this.factory.open(this.database, 1);
   let abandoned = false;
   const abort = () => { abandoned = true; reject(fail('cancelled', 'Device operation cancelled.')); };
   signal?.addEventListener('abort', abort, {once:true});
   const cleanup = () => signal?.removeEventListener('abort', abort);
   request.onupgradeneeded = () => { if (!request.result.objectStoreNames.contains(storeName)) request.result.createObjectStore(storeName, {keyPath:'id'}); };
   request.onblocked = () => { abandoned = true; cleanup(); reject(fail('storage_unavailable', 'Close other Riffine tabs to reopen device storage.')); };
   request.onerror = () => { cleanup(); reject(storageError(request.error)); };
   request.onsuccess = () => { cleanup(); const db = request.result; db.onversionchange = () => db.close(); if (abandoned || signal?.aborted) { db.close(); reject(fail('cancelled', 'Device operation cancelled.')); } else resolve(db); };
  });
 }
 async transaction(mode, operation, signal) {
  const db = await this.connect(signal);
  try {
   if (signal?.aborted) throw fail('cancelled', 'Device operation cancelled.');
   return await new Promise((resolve, reject) => {
    const transaction = db.transaction(storeName, mode);
    let result, failure;
    const abort = () => { try { transaction.abort(); } catch {} };
    signal?.addEventListener('abort', abort, {once:true});
    const cleanup = () => signal?.removeEventListener('abort', abort);
    transaction.oncomplete = () => { cleanup(); resolve(result); };
    transaction.onabort = () => { cleanup(); reject(failure ?? storageError(transaction.error ?? fail('cancelled', 'Device operation cancelled.'))); };
    // IDB requests must be queued synchronously inside this transaction.
    try { operation(transaction.objectStore(storeName), value => {result = value;}, error => {failure = error; transaction.abort();}); }
    catch (error) { failure = storageError(error); transaction.abort(); }
   });
  } catch (error) { throw storageError(error); }
  finally { db.close(); }
 }
 async create(id, document, signal) {
  if (id?.includes('.recovery-')) throw fail('read_only', 'Previous versions are read-only. Save a new device copy.');
  const value = await record(id, document);
  return this.transaction('readwrite', (store, done, reject) => {
   const request = store.add(value);
   request.onerror = () => { if (request.error?.name === 'ConstraintError') reject(fail('document_exists', 'This saved project already exists. Save a new copy.')); };
   request.onsuccess = () => done({revision:value.revision});
  }, signal);
 }
 async read(id, signal) {
  handle(id);
  return this.transaction('readonly', (store, done, reject) => {
   const request = store.get(id);
   request.onsuccess = () => {
    const value = request.result;
    if (!value) reject(fail('resource_unavailable', 'Saved project is missing. Reopen a downloaded copy.'));
    else if (typeof value.document !== 'string' || new TextEncoder().encode(value.document).length > MAX_DOCUMENT_BYTES) reject(fail('document_limit', 'Saved project exceeds 1 MiB.'));
    else done(value);
   };
  }, signal);
 }
 async update(id, expectedRevision, document, signal) {
  handle(id);
  if (!/^[a-f0-9]{64}$/.test(expectedRevision)) throw fail('revision_conflict', 'Reopen the saved project before saving changes.');
  if (id?.includes('.recovery-')) throw fail('read_only', 'Previous versions are read-only. Save a new device copy.');
  const value = await record(id, document);
  const loaded = await this.read(id, signal);
  const actual = await record(id, loaded.document);
  if (actual.revision !== expectedRevision) throw fail('revision_conflict', 'Saved project bytes changed. Save a new copy or reopen the current project.');
  return this.transaction('readwrite', (store, done, reject) => {
   const request = store.get(id);
   request.onsuccess = () => {
    const previous = request.result;
    if (!previous || previous.revision !== expectedRevision || previous.document !== loaded.document) { reject(fail('revision_conflict', 'This project changed in another tab or was removed. Save a new copy to keep your edits, or reopen the current saved project.')); return; }
    const recovery = `${id}.recovery-${previous.revision}`;
    const existing = store.get(recovery);
    existing.onsuccess = () => {
     if (existing.result && existing.result.document !== previous.document) { reject(fail('recovery_conflict', 'The previous-version copy changed. Download your edits before reopening.')); return; }
     if (!existing.result) store.add({...previous, id:recovery, recoveryOf:id});
     store.put(value);
     done({revision:value.revision, recovery});
    };
   };
  }, signal);
 }
 async list(signal) {
  return this.transaction('readonly', (store, done) => {
   const items = [];
   const request = store.openCursor();
   request.onsuccess = () => {
    const cursor = request.result;
    if (!cursor) { done(items.sort((a,b) => b.savedAt-a.savedAt)); return; }
    const {id, name, sourceName, savedAt, recoveryOf} = cursor.value;
    if (typeof id === 'string' && id.startsWith(prefix)) items.push({id, name, sourceName, savedAt, recoveryOf});
    cursor.continue();
   };
  }, signal);
 }
 async remove(id, signal) {
  handle(id);
  return this.transaction('readwrite', (store, done) => {
   const request = store.openCursor();
   request.onsuccess = () => {
    const cursor = request.result;
    if (!cursor) { done({}); return; }
    if (cursor.value.id === id || cursor.value.recoveryOf === id) cursor.delete();
    cursor.continue();
   };
  }, signal);
 }
}
export const projectStore = new ProjectStore();
// Cancellable callback port consumed by Go on its control worker. Abort waits
// for transaction settlement before Go releases callbacks; committed saves win.
export function runProjectStore(requestJSON, document, complete, failed) {
 const controller = new AbortController();
 const task = (async () => {
  try {
   const request = JSON.parse(requestJSON);
   let result;
   switch (request.method) {
   case 'read': result = await projectStore.read(request.handle, controller.signal); break;
   case 'create': result = await projectStore.create(request.handle, document, controller.signal); break;
   case 'update': result = await projectStore.update(request.handle, request.revision, document, controller.signal); break;
   default: throw fail('invalid_command', 'Unknown device storage operation.');
   }
   complete(result);
  } catch (error) { const cause = storageError(error); failed(`${cause.code}: ${cause.message}`); }
 })();
 return fence => {controller.abort(); void task.then(() => fence(''));};
}
