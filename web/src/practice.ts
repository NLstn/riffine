// Presentation-facing types mirror versioned shared-core schemas. Product
// validation, source units, sequencing decisions and persistence stay in Go.
export interface Peak { min: number; max: number }
export interface WaveWindow { version: number; sha256: string; sampleRate: number; sourceFrames: number; requestedStart: number; requestedEnd: number; framesPerCell: number; cells: { startFrame: number; endFrame: number; channels: Peak[] }[] }
export interface Region { id: string; label: string; start: number; end: number; notes?: string }
export interface Annotations { notes?: string; regions?: Region[]; markers?: { id: string; kind: 'section' | 'measure' | 'beat'; label: string; position: number; measure?: number; beat?: number; notes?: string }[]; loops?: { id: string; label: string; regionId: string }[]; bookmarks?: { id: string; label: string; position: number }[] }
export interface Project { version: number; name: string; track: { path: string; audio: { encoding: string; sampleRate: number; channels: number; frames: number; sha256: string } }; settings: { speed: number; pitch: number; loop: { enabled: boolean; start: number; end: number } }; annotations: Annotations; practicePlans?: unknown[] }
export interface Snapshot { version: number; sequence: number; state: string; project: Project; capabilities: { formats: string[]; offlineRender: boolean; liveTransport: boolean; streamPreview?: boolean } }
export type EditCommand = { type: 'set_parameters'; speed?: number; pitch?: number } | { type: 'set_loop'; loop: Project['settings']['loop'] } | { type: 'set_annotations'; annotations: Annotations };
export interface PracticeResponse { version: number; snapshot?: Snapshot; window?: WaveWindow; document?: string; events?: { events: { sequence: number; type: string }[]; dropped: number }; error?: { version: number; code: string; message: string } }
export interface PracticeFacade {
 open(source: File, project?: File): Promise<PracticeResponse>;
 command(snapshot: Snapshot, command: EditCommand): Promise<PracticeResponse>;
 waveform(startFrame: number, endFrame: number, detail?: boolean): Promise<PracticeResponse>;
 exportProject(): Promise<PracticeResponse>;
 events(): Promise<PracticeResponse>;
 cancel(): void;
 dispose(): void;
}
export class BrowserPractice implements PracticeFacade {
 private worker = new Worker('/practice/worker.js');
 private next = 0;
 private pending = new Map<number, { resolve: (result: PracticeResponse) => void; reject: (error: Error) => void }>();
 private ready: Promise<void>;
 private disposed = false;
 constructor() {
  this.ready = new Promise((resolve, reject) => {
   const timeout = setTimeout(() => { reject(new Error('Practice core did not load.')); this.dispose(); }, 30000);
   this.worker.onmessage = (event: MessageEvent) => {
    const data = event.data;
    if (data.type === 'ready') { clearTimeout(timeout); resolve(); }
    if (data.type === 'fatal') { clearTimeout(timeout); const error = new Error(data.message); reject(error); this.fail(error); this.disposed = true; this.worker.terminate(); }
    if (data.type === 'result') { const call = this.pending.get(data.id); this.pending.delete(data.id); call?.resolve(data.response); }
   };
   this.worker.onerror = () => { clearTimeout(timeout); const error = new Error('Practice worker failed.'); reject(error); this.disposed = true; this.fail(error); this.worker.terminate(); };
  });
  // Readiness errors are surfaced by the first user request, avoiding an
  // unhandled rejection when the user keeps browsing the library.
  void this.ready.catch(() => {});
 }
 private fail(error: Error) { for (const call of this.pending.values()) call.reject(error); this.pending.clear(); }
 private async request(request: Record<string, unknown>, file?: File, document = ''): Promise<PracticeResponse> {
  await this.ready;
  if (this.disposed) throw new Error('Practice workspace has closed.');
  const id = ++this.next;
  return new Promise((resolve, reject) => { this.pending.set(id, { resolve, reject }); this.worker.postMessage({ id, request: { version: 1, ...request }, file, document }); });
 }
 async open(source: File, project?: File) {
  if (project && project.size > 1048576) throw new Error('Project exceeds 1 MiB.');
  return this.request({ type: project ? 'open' : 'import', source: 'source', document: 'project' }, source, project ? await project.text() : '');
 }
 command(snapshot: Snapshot, command: EditCommand) { return this.request({ type: 'command', command: { version: snapshot.version, sequence: snapshot.sequence + 1, ...command } }); }
 waveform(startFrame: number, endFrame: number, detail = false) { return this.request({ type: detail ? 'detail' : 'waveform', startFrame, endFrame, bins: detail ? 4096 : 512 }); }
 exportProject() { return this.request({ type: 'export' }); }
 events() { return this.request({ type: 'events' }); }
 cancel() { if (!this.disposed) this.worker.postMessage({ type: 'cancel' }); }
 dispose() { if (this.disposed) return; this.disposed = true; this.worker.terminate(); this.fail(new Error('Practice workspace closed.')); }
}
