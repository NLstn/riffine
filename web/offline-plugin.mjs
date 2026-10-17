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
     if (name === 'practice/licenses' || name === 'practice/media/LICENSES.txt' || name === 'practice/media/SOURCES.txt' || name === 'practice/media/sources' || name.endsWith('.map') || name === 'offline-sw.js') continue;
     if (entry.isDirectory()) await visit(path); else files.push(name);
    }
   }
   await visit(output); files.sort();
   // Fail the production build when an apparently installable app is missing
   // its actual control or processing engine. Editing-only dev stays available.
   for (const asset of ['index.html','practice/core.wasm','practice/wasm_exec.js','practice/dsp.mjs','practice/riffine-browser-dsp.wasm','practice/media/libav.mjs','practice/media/factory.mjs','practice/media/opus-libav.mjs','practice/media/opus-factory.mjs']) {
    if (!files.includes(asset)) throw new Error(`Offline practice requires ${asset}. Run the browser core/DSP build scripts first.`);
   }
   const hash = createHash('sha256'), hashes = {};
   for (const file of files) {
    const bytes = await readFile(resolve(output,file));
    hash.update(file); hash.update(bytes);
    hashes[base+file] = createHash('sha256').update(bytes).digest('hex');
   }
   // Only HTML and its small bootstrap script are needed before the isolation
   // reload. The workspace, engines and optional decoders prepare afterward.
   const html = await readFile(resolve(output,'index.html'),'utf8');
   const shell = ['index.html', ...[...html.matchAll(/(?:src|href)=["']([^"']+)["']/g)].map(match => match[1]).filter(url => url.startsWith(base)).map(url => url.slice(base.length))];
   for (const file of shell) if (!files.includes(file)) throw new Error(`Missing startup asset: ${file}`);
   const priority = file => file === 'practice/core.wasm' ? 0 : file.startsWith('assets/main-') ? 1 : file.startsWith('practice/media/') ? 3 : 2;
   const assets = [...files].sort((a,b) => priority(a)-priority(b));
   const template = await readFile(new URL('./offline-sw-template.js',import.meta.url),'utf8');
   hash.update(template); hash.update(base);
   const version = hash.digest('hex').slice(0,20);
   await writeFile(resolve(output,'offline-sw.js'), `const VERSION = ${JSON.stringify(version)};\nconst BASE = ${JSON.stringify(base)};\nconst ASSETS = ${JSON.stringify(assets.map(file => base+file))};\nconst SHELL = ${JSON.stringify(shell.map(file => base+file))};\nconst HASHES = ${JSON.stringify(hashes)};\n${template}`);
  },
 };
}
