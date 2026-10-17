// Disposable metadata, separate from saved practice projects. Only content hashes
// identify entries: renamed copies hit; replacements with the same name miss.
export interface TrackTags {
  title?: string;
  artist?: string;
  album?: string;
  albumArtist?: string;
  genre?: string;
  year?: number;
  trackNumber?: number;
  duration?: number;
  format?: string;
  artwork?: Blob;
}
const MAX_ENTRIES = 256;
function validTags(value: unknown): value is TrackTags {
  if (!value || typeof value !== 'object') return false;
  const tags = value as Record<string, unknown>;
  return ['title', 'artist', 'album', 'albumArtist', 'genre', 'format'].every(key =>
    tags[key] === undefined || typeof tags[key] === 'string') &&
    ['year', 'trackNumber', 'duration'].every(key => tags[key] === undefined ||
      (typeof tags[key] === 'number' && Number.isFinite(tags[key]))) &&
    (tags.artwork === undefined || (tags.artwork instanceof Blob && tags.artwork.size <= (1 << 20)));
}
export async function tagCache(hash: string, tags?: TrackTags): Promise<TrackTags | undefined> {
  const db = await new Promise<IDBDatabase>((resolve, reject) => {
    let abandoned = false;
    const request = indexedDB.open('riffine-tags-v1', 1);
    request.onupgradeneeded = () => request.result.createObjectStore('tags', { keyPath: 'hash' });
    request.onerror = () => reject(request.error);
    request.onblocked = () => { abandoned = true; reject(new Error('Metadata cache blocked')); };
    request.onsuccess = () => {
      if (abandoned) { request.result.close(); return; }
      request.result.onversionchange = () => request.result.close();
      resolve(request.result);
    };
  });
  try {
    return await new Promise((resolve, reject) => {
      const transaction = db.transaction('tags', tags ? 'readwrite' : 'readonly');
      const store = transaction.objectStore('tags');
      let result: TrackTags | undefined;
      transaction.oncomplete = () => resolve(result);
      transaction.onabort = () => reject(transaction.error);
      if (!tags) {
        const request = store.get(hash);
        request.onsuccess = () => { result = validTags(request.result?.tags) ? request.result.tags : undefined; };
      } else {
        store.put({ hash, tags, savedAt: Date.now() });
        const entries: { hash: string; savedAt: number }[] = [];
        const request = store.openCursor();
        request.onsuccess = () => {
          const cursor = request.result;
          if (cursor) { entries.push({ hash: String(cursor.key), savedAt: cursor.value.savedAt }); cursor.continue(); return; }
          entries.sort((a, b) => b.savedAt - a.savedAt);
          for (const entry of entries.slice(MAX_ENTRIES)) store.delete(entry.hash);
        };
      }
    });
  } finally { db.close(); }
}
