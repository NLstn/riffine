import { readdir, readFile, writeFile } from 'node:fs/promises';
import { resolve, relative } from 'node:path';
import { createHash } from 'node:crypto';

// A complete, versioned production shell. No user files or document requests.
export default function offlinePlugin() {
 let output, base;
 return {
  name:'riffine-offline-shell', apply:'build',
  configResolved(config) {
   output = resolve(config.root, config.build.outDir);
   base = config.base;
   if (!base.startsWith('/') || !base.endsWith('/')) throw new Error('Offline practice requires an absolute base path ending in /.');
  },
  async closeBundle() {
   const files = [];
   async function visit(directory) {
    for (const entry of await readdir(directory, {withFileTypes:true})) {
     const path = resolve(directory, entry.name), name = relative(output, path).replaceAll('\\','/');
     if (name === 'practice/media/sources' || name.endsWith('.map') || name === 'offline-sw.js') continue;
     if (entry.isDirectory()) await visit(path); else files.push(name);
    }
   }
   await visit(output); files.sort();
   // Fail the production build when an apparently installable app is missing
   // its actual control or processing engine. Editing-only dev stays available.
   for (const asset of ['index.html','practice/core.wasm','practice/wasm_exec.js','practice/dsp.mjs','practice/riffine-browser-dsp.wasm','practice/media/libav.mjs','practice/media/factory.mjs']) {
    if (!files.includes(asset)) throw new Error(`Offline practice requires ${asset}. Run the browser core/DSP build scripts first.`);
   }
   const hash = createHash('sha256');
   for (const file of files) { hash.update(file); hash.update(await readFile(resolve(output,file))); }
   const template = await readFile(new URL('./offline-sw-template.js',import.meta.url),'utf8');
   hash.update(template); hash.update(base);
   const version = hash.digest('hex').slice(0,20);
   await writeFile(resolve(output,'offline-sw.js'), `const VERSION = ${JSON.stringify(version)};\nconst BASE = ${JSON.stringify(base)};\nconst ASSETS = ${JSON.stringify(files.map(file => base+file))};\n${template}`);
  },
 };
}
