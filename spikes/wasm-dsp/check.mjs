import { Worker } from 'node:worker_threads';
import { stat, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { strict as assert } from 'node:assert';

const output = resolve(process.argv[2] ?? 'build/wasm-dsp');
const nativePath = resolve(process.argv[3] ?? 'build/wasm-dsp-native/stream-probe');
const native = spawnSync(nativePath, { encoding: 'utf8', timeout: 30000 });
assert.equal(native.status, 0, native.stderr);
const expected = JSON.parse(native.stdout);
const worker = new Worker(new URL('./node-worker.mjs', import.meta.url), { workerData: { modulePath: `${output}/stream-probe.mjs` } });
try {
  const result = await new Promise((resolveResult, reject) => {
    const timeout = setTimeout(() => reject(new Error('DSP worker timed out')), 120000);
    worker.once('message', data => { clearTimeout(timeout); resolveResult(data); });
    worker.once('error', error => { clearTimeout(timeout); reject(error); });
    worker.once('exit', code => { if (code !== 0) { clearTimeout(timeout); reject(new Error(`DSP worker exit ${code}`)); } });
  });
  assert.equal(result.ok, true, result.error);
  assert.equal(result.reports.length, 20);
  for (const report of result.reports) {
    for (const field of ['version', 'sampleRate', 'channels', 'sourceFrames']) assert.equal(report[field], expected[field]);
    assert.equal(report.cases.length, expected.cases.length);
    for (let i = 0; i < report.cases.length; i++) {
      const actual = report.cases[i], reference = expected.cases[i];
      for (const field of ['speed', 'pitch', 'outputFrames', 'tailFrames', 'inputLatency', 'outputLatency']) assert.equal(actual[field], reference[field], `${i}: ${field}`);
      for (let c = 0; c < 2; c++) {
        assert.ok(Math.abs(actual.frequency[c] - reference.frequency[c]) <= 4, 'native/WASM frequency mismatch');
        assert.ok(Math.abs(actual.rms[c] - reference.rms[c]) <= 0.02, 'native/WASM energy mismatch');
      }
    }
  }
  const metrics = { runtime: process.version, iterations: 20,
    wasmBytes: (await stat(`${output}/stream-probe.wasm`)).size,
    jsBytes: (await stat(`${output}/stream-probe.mjs`)).size,
    startupMs: result.startupMs, heapBytes: result.heapBytes, runsMs: result.runsMs };
  await writeFile(`${output}/report.json`, JSON.stringify(result.reports[0], null, 2) + '\n');
  await writeFile(`${output}/metrics.json`, JSON.stringify(metrics, null, 2) + '\n');
  console.log(JSON.stringify(metrics, null, 2));
} finally {
  await worker.terminate();
}
