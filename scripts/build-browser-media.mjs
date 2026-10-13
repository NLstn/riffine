// Package a pinned, independently replaceable audio decoder; no CDN at runtime.
import { readFile, writeFile, mkdir, copyFile, cp } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const root = new URL('../', import.meta.url);
const source = new URL('web/node_modules/libav.js/dist/', root);
const target = new URL('web/public/practice/media/', root);
const version = '4.10.6.1.1';
await mkdir(target, { recursive: true });
const wrapper = await readFile(new URL(`libav-${version}-fat.js`, source), 'utf8');
const factory = await readFile(new URL(`libav-${version}-fat.wasm.js`, source), 'utf8');
// Upstream 4.x is a classic script. Adapt its unchanged APIs to module workers.
await writeFile(new URL('libav.mjs', target), `var LibAV = {}, module = {exports: {}};\n${wrapper}\nexport default LibAV;\n`);
await writeFile(new URL('factory.mjs', target), `
// A local LibAV prevents upstream's standalone-worker bootstrap from replacing
// the codec worker's message handler when importScripts exists in the browser.
var LibAV = {}, require, __dirname;
if (typeof process === 'object' && process.versions?.node) {
 require = (await import('node:module')).createRequire(import.meta.url);
 __dirname = (await import('node:url')).fileURLToPath(new URL('.', import.meta.url));
}
${factory.replace(/var _scriptDir = .*?;/, 'var _scriptDir = new URL(".", import.meta.url).href;')}
export default LibAVFactory;
`);
await copyFile(new URL(`libav-${version}-fat.wasm.wasm`, source), new URL(`libav-${version}-fat.wasm.wasm`, target));
// Complete upstream license inventory is embedded before the factory code.
await writeFile(new URL('LICENSES.txt', target), factory.slice(0, factory.indexOf('var LibAVFactory')));
// LGPL corresponding sources, dependency sources and build recipes accompany
// every production copy. They are not loaded by the player.
await cp(new URL('web/node_modules/libav.js/sources/', root), new URL('sources/', target), {recursive:true});
await writeFile(new URL('SOURCES.txt', target), `libav.js 4.10.6 (build ${version}), fat variant, FFmpeg 6.1.1.\nComplete corresponding sources and build recipes: sources/libav.js.tar.xz\nDependency sources: all archives in sources/\nUpstream: https://github.com/Yahweasel/libav.js/releases/tag/v${version}\nModule adaptation recipe: https://github.com/NLstn/riffine/blob/main/scripts/build-browser-media.mjs\nThe decoder is an independent module and may be replaced or rebuilt.\n`);
console.log(`Packaged browser M4A/Opus decoder in ${fileURLToPath(target)}`);
