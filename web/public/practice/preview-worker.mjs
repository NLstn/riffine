import createModule from './dsp.mjs';
import { Selection } from './selection.mjs';
import { OutputWriter, QueueState, CountersBusyError } from './output-queue.mjs';
export const ready = createModule();
const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

// Runs only on the Go/resource/processing worker. readPCM is a bounded Go worker
// callback, never invoked by the AudioWorklet. The worklet only reads shared PCM.
export function run(options, readPCM, complete, failed) {
 let cancelled = false, writer, selection;
 const abort = () => { cancelled = true; writer?.close(); };
 void (async () => {
  const started = performance.now();
  let startupMilliseconds = 0, startedOutput = false, report, failure;
  try {
   const module = await ready;
   if (cancelled) throw new Error('Preview cancelled.');
   writer = new OutputWriter(self.riffinePreviewQueue);
   if (writer.channels !== options.channels) throw new Error('Output queue channel mismatch.');
   selection = new Selection(module, options.channels, options.sampleRate, options.sourceFrames, options.speed, options.pitch);
   const expected = selection.info().outputFrames;
   let queued = 0, copied = 0, progressAt = performance.now();
   const observe = async () => {
    if (cancelled) throw new Error('Preview cancelled.');
    let state;
    for (;;) {
     if (cancelled) throw new Error('Preview cancelled.');
     try { state = writer.snapshot(); break; }
     catch (error) {
      if (!(error instanceof CountersBusyError)) throw error;
      if (performance.now() - progressAt > 10000) throw new Error('Audio output counters made no progress.');
      await delay(1);
     }
    }
    if (state.state === QueueState.closed || state.state === QueueState.error) throw new Error('Audio output stopped.');
    if (state.copiedFrames !== copied) { copied = state.copiedFrames; progressAt = performance.now(); }
    if (state.state === QueueState.pausing || state.state === QueueState.paused) progressAt = performance.now();
    if (startedOutput && performance.now() - progressAt > 10000) throw new Error('Audio output made no progress for 10 seconds.');
    return state;
   };
   for (;;) {
    await observe();
    const result = selection.read(4096);
    if (result.frames) {
     while (!writer.write(result.pcm)) { await observe(); await delay(4); }
     queued += result.frames;
     if (!startedOutput && queued >= Math.min(4096, expected)) {
      writer.start(); startedOutput = true; startupMilliseconds = performance.now() - started;
     }
    }
    if (result.needsInput) {
     if (cancelled) throw new Error('Preview cancelled.');
     const pcm = await new Promise((resolve, reject) => readPCM(resolve, reject));
     if (cancelled) throw new Error('Preview cancelled.');
     selection.push(pcm);
    } else if (!result.frames) break;
   }
   if (queued !== expected) throw new Error('Processing output length mismatch.');
   writer.sealEOF();
   while ((await observe()).state !== QueueState.done) await delay(4);
   const result = writer.snapshot();
   if (result.copiedFrames !== expected) throw new Error('Audio output copied an unexpected frame count.');
   report = { version: 1, channels: options.channels, sampleRate: options.sampleRate, outputFrames: expected,
    copiedFrames: result.copiedFrames, underrunFrames: result.underrunFrames, decodedBeforeStart: 0,
    startupMilliseconds, backend: 'webaudio', mode: 'device' };
  } catch (error) { writer?.close(); failure = String(error); }
  finally { selection?.close(); }
  // Complete only after releasing the processor; Go uses this as a lifetime fence.
  if (failure) failed(failure); else complete(report);
 })();
 return abort;
}
