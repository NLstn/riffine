// Package a pinned, independently replaceable audio decoder; no CDN at runtime.
import { readFile, writeFile, mkdir, copyFile, cp, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const root = new URL('../', import.meta.url);
// Upstream 6.x no longer ships AAC/ALAC. Keep the last fat build explicitly
// pinned for M4A; Opus uses the maintained 6.x module below.
const source = new URL('web/node_modules/libav-m4a/dist/', root);
const target = new URL('web/public/practice/media/', root);
const version = '4.10.6.1.1';
await rm(target, { recursive: true, force: true });
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
// LGPL corresponding sources, dependency sources and build recipes accompany
// every production copy. They are not loaded by the player.
await cp(new URL('web/node_modules/libav-m4a/sources/', root), new URL('sources/m4a/', target), {recursive:true});
const opusSource = new URL('web/node_modules/libav.js/dist/', root);
const opusWrapper = await readFile(new URL('libav-opus.mjs', opusSource), 'utf8');
const opusVersion = opusWrapper.match(/libav\.VER="([\d.]+)"/)?.[1];
if (!opusVersion) throw new Error('Cannot identify the pinned Opus decoder build.');
const opusFactory = await readFile(new URL(`libav-${opusVersion}-opus.wasm.mjs`, opusSource), 'utf8');
// 6.x owns its memory instead of importing wasmMemory. Bound its allocator to
// the same 64 MiB as the M4A decoder, and fail visibly if this hook changes.
const heapLimit = 'var getHeapMax=()=>2147483648;';
if (!opusFactory.includes(heapLimit)) throw new Error('Recheck the Opus decoder heap limit.');
await writeFile(new URL('opus-libav.mjs', target), opusWrapper);
await writeFile(new URL('opus-factory.mjs', target), opusFactory.replace(heapLimit, 'var getHeapMax=()=>67108864;'));
await copyFile(new URL(`libav-${opusVersion}-opus.wasm.wasm`, opusSource), new URL(`libav-${opusVersion}-opus.wasm.wasm`, target));
await writeFile(new URL('LICENSES.txt', target), factory.slice(0, factory.indexOf('var LibAVFactory')) + '\n' + opusFactory.slice(0, opusFactory.indexOf('async function LibAVFactory')));
await cp(new URL('web/node_modules/libav.js/sources/', root), new URL('sources/opus/', target), {recursive:true});
await writeFile(new URL('SOURCES.txt', target), `M4A: libav.js 4.10.6 (build ${version}), fat variant, FFmpeg 6.1.1.\nOpus: libav.js build ${opusVersion}, opus variant.\nComplete corresponding sources, dependency sources and build recipes: sources/m4a/ and sources/opus/ (including libav.js.tar.xz in each).\nUpstream: https://github.com/Yahweasel/libav.js/releases\nModule adaptation and allocator-limit recipe: https://github.com/NLstn/riffine/blob/main/scripts/build-browser-media.mjs\nEach decoder is an independent module and may be replaced or rebuilt.\n`);
console.log(`Packaged browser M4A/Opus decoder in ${fileURLToPath(target)}`);
