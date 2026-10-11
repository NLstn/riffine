// Presentation-facing types mirror versioned shared-core schemas. Product
// validation, source units, sequencing decisions and persistence stay in Go.
export interface Peak { min: number; max: number }
export interface WaveWindow { version: number; sha256: string; sampleRate: number; sourceFrames: number; requestedStart: number; requestedEnd: number; framesPerCell: number; cells: { startFrame: number; endFrame: number; channels: Peak[] }[] }
export interface Region { id: string; label: string; start: number; end: number; notes?: string }
export interface Annotations { notes?: string; regions?: Region[]; markers?: { id: string; kind: 'section' | 'measure' | 'beat'; label: string; position: number; measure?: number; beat?: number; notes?: string }[]; loops?: { id: string; label: string; regionId: string }[]; bookmarks?: { id: string; label: string; position: number }[] }
export interface Project { version: number; name: string; track: { path: string; audio: { encoding: string; sampleRate: number; channels: number; frames: number; sha256: string } }; settings: { speed: number; pitch: number; loop: { enabled: boolean; start: number; end: number } }; annotations: Annotations; practicePlans?: unknown[] }
export interface Snapshot { version: number; sequence: number; state: string; project: Project; capabilities: { formats: string[]; offlineRender: boolean; liveTransport: boolean; streamPreview?: boolean } }
export type EditCommand = { type: 'set_parameters'; speed?: number; pitch?: number } | { type: 'set_loop'; loop: Project['settings']['loop'] } | { type: 'set_annotations'; annotations: Annotations };
export interface PracticeResponse { version: number; snapshot?: Snapshot; window?: WaveWindow; document?: string; preview?: { outputFrames: number; copiedFrames: number; underrunFrames: number; startupMilliseconds: number }; events?: { events: { sequence: number; type: string }[]; dropped: number }; error?: { version: number; code: string; message: string } }
export interface PracticeFacade {
 open(source: File, project?: File): Promise<PracticeResponse>;
 command(snapshot: Snapshot, command: EditCommand): Promise<PracticeResponse>;
 waveform(startFrame: number, endFrame: number, detail?: boolean): Promise<PracticeResponse>;
 exportProject(): Promise<PracticeResponse>;
 events(): Promise<PracticeResponse>;
 preview?(snapshot: Snapshot): Promise<PracticeResponse>;
 cancel(): void;
 dispose(): void;
}
interface OutputQueueFactory { createOutputQueue(channels: number): SharedArrayBuffer }
export class BrowserPractice implements PracticeFacade {
 private worker = new Worker('/practice/worker.js');
 private next = 0;
 private pending = new Map<number, { resolve: (result: PracticeResponse) => void; reject: (error: Error) => void }>();
 private ready: Promise<void>;
 private disposed = false;
 private audio?: AudioContext;
 private previewAbort?: (error?: Error) => void;
 private loadOutput: () => Promise<OutputQueueFactory>;
 private audioClosures = new WeakMap<AudioContext, Promise<void>>();
 private readyCancel?: (error: Error) => void;
 private readyTimeout?: ReturnType<typeof setTimeout>;
 private previewSupported = globalThis.crossOriginIsolated && typeof AudioContext === 'function' && 'audioWorklet' in AudioContext.prototype;
 constructor(options: { loadOutput?: () => Promise<OutputQueueFactory> } = {}) {
  this.loadOutput = options.loadOutput ?? (async () => {
   const url = new URL('/practice/output-queue.mjs', location.origin).href;
   return await import(/* @vite-ignore */ url) as OutputQueueFactory;
  });
  this.ready = new Promise((resolve, reject) => {
   this.readyCancel = reject;
   const timeout = this.readyTimeout = setTimeout(() => { reject(new Error('Practice core did not load.')); this.dispose(); }, 30000);
   this.worker.onmessage = (event: MessageEvent) => {
    const data = event.data;
    if (data.type === 'ready') { clearTimeout(timeout); this.readyCancel = undefined; resolve(); }
    if (data.type === 'fatal') { clearTimeout(timeout); const error = new Error(data.message); reject(error); this.shutdown(error); }
    if (data.type === 'result') { const call = this.pending.get(data.id); this.pending.delete(data.id); call?.resolve(data.response); }
   };
   this.worker.onerror = () => { clearTimeout(timeout); const error = new Error('Practice worker failed.'); reject(error); this.shutdown(error); };
  });
  // Readiness errors are surfaced by the first user request, avoiding an
  // unhandled rejection when the user keeps browsing the library.
  void this.ready.catch(() => {});
 }
 private fail(error: Error) { for (const call of this.pending.values()) call.reject(error); this.pending.clear(); }
 private async request(request: Record<string, unknown>, file?: File, document = '', queue?: SharedArrayBuffer): Promise<PracticeResponse> {
  await this.ready;
  if (this.disposed) throw new Error('Practice workspace has closed.');
  const id = ++this.next;
  return new Promise((resolve, reject) => { this.pending.set(id, { resolve, reject }); try { this.worker.postMessage({ id, request: { version: 1, ...request }, file, document, queue, previewSupported: this.previewSupported }); } catch (error) { this.pending.delete(id); reject(error instanceof Error ? error : new Error(String(error))); } });
 }
 async open(source: File, project?: File) {
  if (project && project.size > 1048576) throw new Error('Project exceeds 1 MiB.');
  return this.request({ type: project ? 'open' : 'import', source: 'source', document: 'project' }, source, project ? await project.text() : '');
 }
 command(snapshot: Snapshot, command: EditCommand) { return this.request({ type: 'command', command: { version: snapshot.version, sequence: snapshot.sequence + 1, ...command } }); }
 waveform(startFrame: number, endFrame: number, detail = false) { return this.request({ type: detail ? 'detail' : 'waveform', startFrame, endFrame, bins: detail ? 4096 : 512 }); }
 exportProject() { return this.request({ type: 'export' }); }
 events() { return this.request({ type: 'events' }); }
 async preview(snapshot: Snapshot) {
  if (this.disposed) throw new Error('Practice workspace has closed.');
  if (!this.previewSupported || !snapshot.capabilities.streamPreview) throw new Error('Audio preview is unavailable in this browser.');
  if (this.audio) throw new Error('An audio preview is already running.');
  const info = snapshot.project.track.audio;
  // Create and resume synchronously from the user's click before awaiting worker
  // readiness or module loading, preserving browser user-activation requirements.
  const context = new AudioContext({ sampleRate: info.sampleRate, latencyHint: 'interactive' });
  this.audio = context;
  let abortPreview: (error?: Error) => void = () => {};
  const cancelled = new Promise<never>((_resolve, reject) => { abortPreview = (error = new Error('cancelled: audio preview cancelled')) => reject(error); });
  void cancelled.catch(() => {});
  this.previewAbort = abortPreview;
  const resumed = context.resume();
  void resumed.catch(() => {});
  let node: AudioWorkletNode | undefined;
  try {
   if (context.sampleRate !== info.sampleRate) throw new Error('This output device cannot preview the original sample rate.');
   const queueModule = await Promise.race([this.loadOutput(), cancelled]);
   const queue = queueModule.createOutputQueue(info.channels);
   await Promise.race([context.audioWorklet.addModule('/practice/output-worklet.mjs'), cancelled]);
   node = new AudioWorkletNode(context, 'riffine-output', { numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [info.channels], processorOptions: { queue } });
   node.onprocessorerror = () => this.cancel();
   node.connect(context.destination);
   await Promise.race([resumed, cancelled]);
   if (this.disposed) throw new Error('Practice workspace has closed.');
   const result = await this.request({ type: 'preview' }, undefined, '', queue);
   // This is a conservative preview drain, not presentation acknowledgement.
   // The completed report counts callback copies and never increments loops.
   if (!result.error) await Promise.race([new Promise(resolve => setTimeout(resolve, Math.ceil((context.baseLatency + (context.outputLatency || 0)) * 1000 + 20))), cancelled]);
   return result;
  } finally {
   if (this.previewAbort === abortPreview) this.previewAbort = undefined;
   node?.disconnect();
   await this.closeAudio(context);
   if (this.audio === context) this.audio = undefined;
  }
 }
 private closeAudio(context: AudioContext) {
  let closing = this.audioClosures.get(context);
  if (!closing) { closing = context.state === 'closed' ? Promise.resolve() : context.close(); this.audioClosures.set(context, closing); }
  return closing;
 }
 cancel() { this.previewAbort?.(); if (!this.disposed) this.worker.postMessage({ type: 'cancel', previewSupported: this.previewSupported }); if (this.audio) void this.audio.suspend().catch(() => {}); }
 private shutdown(error: Error) {
  if (this.disposed) return;
  this.disposed = true;
  clearTimeout(this.readyTimeout);
  this.readyCancel?.(error); this.readyCancel = undefined;
  this.previewAbort?.(error);
  this.worker.terminate(); this.fail(error);
  if (this.audio) void this.closeAudio(this.audio).catch(() => {});
 }
 dispose() { this.shutdown(new Error('Practice workspace closed.')); }
}
