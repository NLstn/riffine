// Presentation-facing types mirror versioned shared-core schemas. Product
// validation, source units, sequencing decisions and persistence stay in Go.
export interface Peak { min: number; max: number }
export interface WaveWindow { version: number; sha256: string; sampleRate: number; sourceFrames: number; requestedStart: number; requestedEnd: number; framesPerCell: number; cells: { startFrame: number; endFrame: number; channels: Peak[] }[] }
export interface Region { id: string; label: string; start: number; end: number; notes?: string }
export interface Annotations { notes?: string; regions?: Region[]; markers?: { id: string; kind: 'section' | 'measure' | 'beat'; label: string; position: number; measure?: number; beat?: number; notes?: string }[]; loops?: { id: string; label: string; regionId: string }[]; bookmarks?: { id: string; label: string; position: number }[] }
export interface Project { version: number; name: string; track: { path: string; audio: { encoding: string; sampleRate: number; channels: number; frames: number; sha256: string } }; settings: { speed: number; pitch: number; loop: { enabled: boolean; start: number; end: number } }; annotations: Annotations; practicePlans?: unknown[] }
export interface Snapshot { version: number; sequence: number; state: string; transport?: { generation: number; state: string; sourceFrame: number; consumedOutputFrame: number; completedLoops: number; error?: string }; project: Project; capabilities: { formats: string[]; offlineRender: boolean; liveTransport: boolean; streamPreview?: boolean } }
// A successful source open starts a new session with sequence zero. Only
// in-session replies can be compared by sequence to suppress stale UI updates.
export function presentationSnapshot(previous: Snapshot | undefined, current: Snapshot, newSession = false): Snapshot {
 return !newSession && previous && previous.sequence > current.sequence ? previous : current;
}
export type EditCommand = { type: 'set_parameters'; speed?: number; pitch?: number } | { type: 'set_loop'; loop: Project['settings']['loop'] } | { type: 'set_annotations'; annotations: Annotations };
export interface PracticeResponse { version: number; snapshot?: Snapshot; window?: WaveWindow; document?: string; preview?: { outputFrames: number; copiedFrames: number; underrunFrames: number; startupMilliseconds: number }; events?: { events: { sequence: number; type: string }[]; dropped: number }; error?: { version: number; code: string; message: string } }
export type LiveCommand = { type: 'play' | 'pause' | 'stop' | 'restart' | 'recover' } | { type: 'seek'; position: number };
export interface PracticeFacade {
 open(source: File, project?: File): Promise<PracticeResponse>;
 command(snapshot: Snapshot, command: EditCommand): Promise<PracticeResponse>;
 waveform(startFrame: number, endFrame: number, detail?: boolean): Promise<PracticeResponse>;
 exportProject(): Promise<PracticeResponse>;
 liveSupported?: boolean;
 transport?(snapshot: Snapshot, command: LiveCommand): Promise<PracticeResponse>;
 pollTransport?(): Promise<PracticeResponse>;
 detachTransport?(): Promise<PracticeResponse>;
 events(): Promise<PracticeResponse>;
 preview?(snapshot: Snapshot, controlsReady?: () => void): Promise<PracticeResponse>;
 pausePreview?(): Promise<boolean>;
 resumePreview?(): Promise<void>;
 cancel(): void;
 dispose(): void;
}
interface PresentationMonitorPort { failure?: Error; finish(expectedFrames: number): Promise<void>; waitForFrames(expectedFrames: number): Promise<number>; close(): void }
interface LiveHostPort { context: AudioContext; reset(): Promise<{queue: SharedArrayBuffer; presented: SharedArrayBuffer}>; start(): Promise<void>; suspend(): Promise<void>; fail(error: Error): Promise<void>; close(): Promise<void> }
interface OutputBoundaryPort { pause(): Promise<boolean>; resume(): Promise<void>; close(): void }
interface OutputQueueFactory {
 createOutputQueue(channels: number): SharedArrayBuffer;
 createPresentationTimeline?(): SharedArrayBuffer;
 OutputBoundary?: new (context: AudioContext, queue: SharedArrayBuffer, presentation: PresentationMonitorPort) => OutputBoundaryPort;
 PresentationMonitor?: new (context: AudioContext, timeline: SharedArrayBuffer, onError: (error: Error) => void) => PresentationMonitorPort;
}
export class BrowserPractice implements PracticeFacade {
 private worker = new Worker('/practice/worker.js');
 private next = 0;
 private serial: Promise<unknown> = Promise.resolve();
 private liveHost?: LiveHostPort;
 private liveSetup?: Promise<void>;
 liveSupported = globalThis.crossOriginIsolated && typeof AudioContext === 'function' && typeof AudioContext.prototype.getOutputTimestamp === 'function';
 private pending = new Map<number, { resolve: (result: PracticeResponse) => void; reject: (error: Error) => void }>();
 private ready: Promise<void>;
 private disposed = false;
 private audio?: AudioContext;
 private outputBoundary?: OutputBoundaryPort;
 private previewAbort?: (error?: Error) => void;
 private loadOutput: () => Promise<OutputQueueFactory>;
 private loadLive: () => Promise<{LiveOutputHost: new (context: AudioContext, channels: number, onFailure: (error: Error) => void) => LiveHostPort}>;
 private audioClosures = new WeakMap<AudioContext, Promise<void>>();
 private readyCancel?: (error: Error) => void;
 private readyTimeout?: ReturnType<typeof setTimeout>;
 private previewSupported = globalThis.crossOriginIsolated && typeof AudioContext === 'function' && 'audioWorklet' in AudioContext.prototype;
 constructor(options: { loadOutput?: () => Promise<OutputQueueFactory>; loadLive?: BrowserPractice['loadLive'] } = {}) {
  this.loadLive = options.loadLive ?? (() => import(/* @vite-ignore */ new URL('/practice/live-output-host.mjs', location.origin).href));
  this.loadOutput = options.loadOutput ?? (async () => {
   const url = new URL('/practice/output-queue.mjs', location.origin).href;
   const [queue, timeline, monitor, boundary] = await Promise.all([
    import(/* @vite-ignore */ url),
    import(/* @vite-ignore */ new URL('/practice/presentation-timeline.mjs', location.origin).href),
    import(/* @vite-ignore */ new URL('/practice/presentation-monitor.mjs', location.origin).href),
    import(/* @vite-ignore */ new URL('/practice/output-boundary.mjs', location.origin).href),
   ]);
   return { ...queue, ...timeline, ...monitor, ...boundary } as OutputQueueFactory;
  });
  this.ready = new Promise((resolve, reject) => {
   this.readyCancel = reject;
   const timeout = this.readyTimeout = setTimeout(() => { reject(new Error('Practice core did not load.')); this.dispose(); }, 30000);
   this.worker.onmessage = (event: MessageEvent) => {
    const data = event.data;
    if (data.type === 'live_call') void this.handleLiveCall(data);
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
 private request(request: Record<string, unknown>, file?: File, document = '', queue?: SharedArrayBuffer): Promise<PracticeResponse> {
  const invoke = async (): Promise<PracticeResponse> => {
   await this.ready;
   if (this.disposed) throw new Error('Practice workspace has closed.');
   const id = ++this.next;
   return new Promise((resolve, reject) => { this.pending.set(id, { resolve, reject }); try { this.worker.postMessage({ id, request: { version: 1, ...request }, file, document, queue, previewSupported: this.previewSupported, liveSupported: this.liveSupported }); } catch (error) { this.pending.delete(id); reject(error instanceof Error ? error : new Error(String(error))); } });
  };
  const result = this.serial.then(invoke); this.serial = result.catch(() => {}); return result;
 }
 async open(source: File, project?: File) {
  if (project && project.size > 1048576) throw new Error('Project exceeds 1 MiB.');
  return this.request({ type: project ? 'open' : 'import', source: 'source', document: 'project' }, source, project ? await project.text() : '');
 }
 command(snapshot: Snapshot, command: EditCommand) { return this.request({ type: 'command', command: { version: snapshot.version, sequence: snapshot.sequence + 1, ...command } }); }
 waveform(startFrame: number, endFrame: number, detail = false) { return this.request({ type: detail ? 'detail' : 'waveform', startFrame, endFrame, bins: detail ? 4096 : 512 }); }
 exportProject() { return this.request({ type: 'export' }); }
 events() { return this.request({ type: 'events' }); }
 private async handleLiveCall(message: { id: number; method: string; args?: {channels?: number; sampleRate?: number; message?: string} }) {
  try {
   if (!Number.isSafeInteger(message.id) || message.id < 1) throw new Error('Invalid live host request.');
   let result: unknown = {};
   if (message.method === 'open') {
    await this.liveSetup;
    if (!this.liveHost || this.liveHost.context.sampleRate !== message.args?.sampleRate) throw new Error('Live output was not prepared from a user action.');
   } else if (message.method === 'close') {
    const host = this.liveHost;
    await host?.close();
    if (this.liveHost === host) this.liveHost = undefined;
    if (this.audio === host?.context) this.audio = undefined;
   } else {
    const host = this.liveHost; if (!host) throw new Error('Live output host is unavailable.');
    switch (message.method) {
    case 'reset': result = await host.reset(); break;
    case 'start': await host.start(); break;
    case 'suspend': await host.suspend(); break;
    case 'fail': await host.fail(new Error(message.args?.message || 'Live device failed.')); break;
    default: throw new Error('Unknown live host operation.');
    }
   }
   if (!this.disposed) this.worker.postMessage({type:'live_result', id:message.id, result});
  } catch (error) { if (!this.disposed) this.worker.postMessage({type:'live_result', id:message.id, error:String(error)}); }
 }
 private prepareLive(snapshot: Snapshot, beforeInstall: Promise<unknown> = Promise.resolve()) {
  if (!this.liveSupported || !snapshot.capabilities.streamPreview) throw new Error('Live output is unavailable in this browser.');
  const context = new AudioContext({sampleRate:snapshot.project.track.audio.sampleRate, latencyHint:'interactive'});
  const resumed = context.resume(); void resumed.catch(() => {});
  let abort: (error?: Error) => void = () => {};
  const cancelled = new Promise<never>((_resolve,reject) => { abort = (error = new Error('cancelled: live output setup cancelled')) => reject(error); });
  void cancelled.catch(() => {}); this.previewAbort = abort;
  const ready = (async () => {
   try {
    if (context.sampleRate !== snapshot.project.track.audio.sampleRate) throw new Error('This device does not support the original live sample rate.');
    const module = await Promise.race([this.loadLive(),cancelled]);
    await Promise.race([context.audioWorklet.addModule('/practice/output-worklet.mjs'),cancelled]);
    await Promise.race([Promise.all([resumed,beforeInstall]),cancelled]);
    if (this.disposed) throw new Error('Practice workspace closed.');
    this.audio = context;
    this.liveHost = new module.LiveOutputHost(context,snapshot.project.track.audio.channels,() => {});
   } catch (error) { await this.closeAudio(context); if (this.audio === context) this.audio = undefined; if (this.liveHost?.context === context) this.liveHost = undefined; throw error; }
  })();
  this.liveSetup = ready; void ready.catch(() => {});
  return {ready, abort, context};
 }
 async transport(snapshot: Snapshot, command: LiveCommand): Promise<PracticeResponse> {
  if (this.disposed) throw new Error('Practice workspace closed.');
  let current = snapshot, prepared: ReturnType<BrowserPractice['prepareLive']> | undefined;
  try {
   if (command.type === 'recover') {
    // Rearm context from this user action before any await. Old-device close
    // retains its own host until detach replies; it cannot close the new context.
    const detached = this.request({type:'detach_transport'});
    prepared = this.prepareLive(snapshot, detached);
    const response = await detached; if (response.error) { prepared.abort(new Error(response.error.message)); await prepared.ready.catch(() => {}); await this.liveHost?.close(); await this.closeAudio(prepared.context); if (this.audio === prepared.context) this.audio = undefined; this.liveHost = undefined; return response; }
    current = response.snapshot ?? current;
   }
   if (!current.capabilities.liveTransport) {
    if (!prepared) {
     if (this.audio) throw new Error('An audio preview is already running.');
     prepared = this.prepareLive(current);
    }
    await prepared.ready;
    const attached = await this.request({type:'attach_transport'});
    if (attached.error) { await this.liveHost?.close(); this.liveHost = undefined; if (this.audio === prepared.context) this.audio = undefined; return attached; }
    current = attached.snapshot ?? current;
   }
   return await this.request({type:'command',command:{version:current.version,sequence:current.sequence+1,...(command.type === 'recover' ? {type:'stop'} : command)}});
  } finally { if (prepared && this.previewAbort === prepared.abort) this.previewAbort = undefined; }
 }
 pollTransport() { return this.request({type:'poll_transport'}); }
 detachTransport() { return this.request({type:'detach_transport'}); }
 async preview(snapshot: Snapshot, controlsReady?: () => void) {
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
  let presentation: PresentationMonitorPort | undefined;
  let boundary: OutputBoundaryPort | undefined;
  try {
   if (context.sampleRate !== info.sampleRate) throw new Error('This output device cannot preview the original sample rate.');
   const queueModule = await Promise.race([this.loadOutput(), cancelled]);
   const queue = queueModule.createOutputQueue(info.channels);
   const timeline = typeof context.getOutputTimestamp === 'function' && queueModule.PresentationMonitor ? queueModule.createPresentationTimeline?.() : undefined;
   await Promise.race([context.audioWorklet.addModule('/practice/output-worklet.mjs'), cancelled]);
   node = new AudioWorkletNode(context, 'riffine-output', { numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [info.channels], processorOptions: { queue, timeline } });
   node.onprocessorerror = () => this.cancel();
   node.connect(context.destination);
   await Promise.race([resumed, cancelled]);
   if (this.disposed) throw new Error('Practice workspace has closed.');
   if (timeline && queueModule.PresentationMonitor) presentation = new queueModule.PresentationMonitor(context, timeline, error => { abortPreview(error); this.cancel(); });
   if (presentation && queueModule.OutputBoundary) { boundary = new queueModule.OutputBoundary(context, queue, presentation); this.outputBoundary = boundary; controlsReady?.(); }
   const result = await this.request({ type: 'preview' }, undefined, '', queue);
   // Always await the core reply above, even after a timestamp failure, so its
   // borrowed-reader/callback teardown fence completes before closing the host.
   if (presentation?.failure) throw presentation.failure;
   if (!result.error) {
    if (presentation && result.preview) await Promise.race([presentation.finish(result.preview.outputFrames), cancelled]);
    else await Promise.race([new Promise(resolve => setTimeout(resolve, Math.ceil((context.baseLatency + (context.outputLatency || 0)) * 1000 + 20))), cancelled]);
   }
   // Reports remain callback-copy diagnostics; no transport/loop acknowledgements
   // are emitted by preview, including browsers with output timestamps.
   return result;
  } finally {
   if (this.previewAbort === abortPreview) this.previewAbort = undefined;
   boundary?.close();
   if (this.outputBoundary === boundary) this.outputBoundary = undefined;
   presentation?.close();
   node?.disconnect();
   await this.closeAudio(context);
   if (this.audio === context) this.audio = undefined;
  }
 }
 pausePreview() { return this.outputBoundary?.pause() ?? Promise.resolve(false); }
 resumePreview() { return this.outputBoundary?.resume() ?? Promise.resolve(); }
 private closeAudio(context: AudioContext) {
  let closing = this.audioClosures.get(context);
  if (!closing) { closing = context.state === 'closed' ? Promise.resolve() : context.close(); this.audioClosures.set(context, closing); }
  return closing;
 }
 cancel() { this.outputBoundary?.close(); this.previewAbort?.(); if (!this.disposed) this.worker.postMessage({ type: 'cancel', previewSupported: this.previewSupported, liveSupported: this.liveSupported }); if (this.liveHost) void this.liveHost.fail(new Error('cancelled: live command cancelled')).catch(() => {}); else if (this.audio) void this.audio.suspend().catch(() => {}); }
 private shutdown(error: Error) {
  if (this.disposed) return;
  this.disposed = true;
  clearTimeout(this.readyTimeout);
  this.readyCancel?.(error); this.readyCancel = undefined;
  this.outputBoundary?.close();
  this.previewAbort?.(error);
  this.worker.terminate(); this.fail(error);
  void this.liveHost?.close().catch(() => {});
  if (this.audio && this.audio !== this.liveHost?.context) void this.closeAudio(this.audio).catch(() => {});
 }
 dispose() { this.shutdown(new Error('Practice workspace closed.')); }
}
