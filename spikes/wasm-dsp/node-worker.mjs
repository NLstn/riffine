import { parentPort, workerData } from 'node:worker_threads';
import { pathToFileURL } from 'node:url';
import { performance } from 'node:perf_hooks';

try {
  const started = performance.now();
  const { default: createModule } = await import(pathToFileURL(workerData.modulePath).href);
  let lines = [], errors = [];
  const module = await createModule({ noInitialRun: true, print: line => lines.push(line), printErr: line => errors.push(line) });
  const startupMs = performance.now() - started;
  const heapBytes = module.HEAPU8.byteLength;
  const runsMs = [], reports = [];
  for (let i = 0; i < 20; i++) {
    lines = []; errors = [];
    const before = performance.now();
    const status = module._main();
    runsMs.push(performance.now() - before);
    if (status !== 0) throw new Error(`DSP probe status ${status}: ${errors.join('\n')}`);
    reports.push(JSON.parse(lines.join('\n')));
    if (module.HEAPU8.byteLength !== heapBytes) throw new Error('WASM heap grew during processing');
  }
  parentPort.postMessage({ ok: true, startupMs, heapBytes, runsMs, reports });
} catch (error) {
  parentPort.postMessage({ ok: false, error: String(error) });
}
