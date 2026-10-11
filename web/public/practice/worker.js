// A local worker owns File resources, core state and analysis. No realtime audio.
importScripts('./wasm_exec.js');
const go = new Go();
self.onmessage = (event) => {
  const { id, request, file, document = '', type } = event.data;
  if (type === 'cancel') { self.riffinePracticeCancel?.(); return; }
  if (typeof self.riffinePracticeRequest !== 'function') {
    self.postMessage({ type: 'result', id, response: { version: 1, error: { version: 1, code: 'host_failure', message: 'Practice worker has not loaded.' } } });
    return;
  }
  self.riffinePracticeRequest(id, JSON.stringify(request), file, document);
};
(async () => {
  try {
    const response = await fetch('./core.wasm');
    if (!response.ok) throw new Error('Shared practice core is missing. Run the browser build script.');
    const result = await WebAssembly.instantiate(await response.arrayBuffer(), go.importObject);
    void go.run(result.instance).catch(error => self.postMessage({ type: 'fatal', message: String(error) }));
  } catch (error) { self.postMessage({ type: 'fatal', message: String(error) }); }
})();
