import createModule from './dsp.mjs';
import { Selection } from './selection.mjs';

// One bounded DSP cursor on a serialized non-audio worker. The caller chooses
// source ranges/settings through Go; this worker only feeds/drains the C++ ABI.
let selection, busy = false;
const ready = createModule();
ready.then(module => self.postMessage({ type: 'ready', heapBytes: module.HEAPU8.byteLength }))
 .catch(error => self.postMessage({ type: 'fatal', message: String(error) }));
self.onmessage = async event => {
 const request = event.data;
 const id = request?.id;
 if (!Number.isSafeInteger(id) || id < 1) { self.postMessage({ type: 'fatal', message: 'Invalid processing request ID.' }); return; }
 if (busy) { self.postMessage({ type: 'result', id, error: 'Processing worker is busy.' }); return; }
 busy = true;
 try {
  const module = await ready;
  let result, transfers = [];
  switch (request.type) {
  case 'create': {
   if (selection) throw new Error('Close the previous selection first.');
   selection = new Selection(module, request.channels, request.sampleRate, request.sourceFrames, request.speed, request.pitch, request.edgeFadeFrames, request.outputRate);
   result = selection.info();
   break;
  }
  case 'push':
   if (!selection) throw new Error('Create a selection first.');
   selection.push(request.pcm); result = selection.info(); break;
  case 'read':
   if (!selection) throw new Error('Create a selection first.');
   result = selection.read(request.capacity);
   transfers = [result.pcm.buffer]; break;
  case 'info':
   if (!selection) throw new Error('Create a selection first.');
   result = selection.info(); break;
  case 'close':
   selection?.close(); selection = undefined; result = { closed: true }; break;
  default: throw new Error('Unknown processing operation.');
  }
  self.postMessage({ type: 'result', id, result }, transfers);
 } catch (error) { self.postMessage({ type: 'result', id, error: String(error) }); }
 finally { busy = false; }
};
