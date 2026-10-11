import createModule from './stream-probe.mjs';

self.onmessage = async () => {
  try {
    const started = performance.now();
    let lines = [], errors = [];
    const module = await createModule({ noInitialRun: true, print: line => lines.push(line), printErr: line => errors.push(line) });
    const startupMs = performance.now() - started;
    const before = performance.now();
    const status = module._main();
    if (status !== 0) throw new Error(`DSP probe status ${status}: ${errors.join('\n')}`);
    // Only metadata is posted to the UI. Signals and PCM remain inside this worker.
    self.postMessage({ ok: true, startupMs, fixtureMs: performance.now() - before,
      heapBytes: module.HEAPU8.byteLength, report: JSON.parse(lines.join('\n')) });
  } catch (error) {
    self.postMessage({ ok: false, error: String(error) });
  }
};
