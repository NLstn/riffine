/* Browser worker control bridge. Go owns all fixture/project/practice rules. */
importScripts('./wasm_exec.js');
const started = performance.now();
const go = new Go();
const instance = awaitInstance();
async function awaitInstance() {
  const { instance } = await WebAssembly.instantiateStreaming(fetch('./core.wasm'), go.importObject);
  const running = go.run(instance);
  running.catch(error => self.postMessage({ type: 'fatal', error: String(error) }));
  if (typeof self.riffineRunFixture !== 'function') throw new Error('Go bridge did not initialize');
  return instance;
}
instance.then(value => {
  self.postMessage({ type: 'ready', startupMs: performance.now() - started, memoryBytes: value.exports.mem.buffer.byteLength });
}).catch(error => self.postMessage({ type: 'fatal', error: String(error) }));
self.onmessage = async ({ data }) => {
  try {
    const value = await instance;
    const before = performance.now();
    const report = JSON.parse(self.riffineRunFixture(data.fixture));
    self.postMessage({ type: 'result', id: data.id, report, runMs: performance.now() - before, memoryBytes: value.exports.mem.buffer.byteLength });
  } catch (error) {
    self.postMessage({ type: 'fatal', id: data.id, error: String(error) });
  }
};
