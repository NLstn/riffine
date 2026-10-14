// Verify accessible source-frame gestures in the actual React waveform.
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {mkdtemp,rm} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
const require=createRequire(new URL('../web/package.json',import.meta.url));
const {JSDOM}=require('jsdom');
const dom=new JSDOM('<div id="root"></div>',{url:'http://localhost/'});
for(const name of ['window','document','Element','HTMLElement'])globalThis[name]=name==='window'?dom.window:dom.window[name];
globalThis.IS_REACT_ACT_ENVIRONMENT=true;
const React=require('react'), {createRoot}=require('react-dom/client');
const {rolldown}=await import(pathToFileURL(require.resolve('rolldown')));
const temporary=await mkdtemp(fileURLToPath(new URL('../web/.waveform-test-',import.meta.url)));
const root=createRoot(document.getElementById('root'));let bundle;
try {
 bundle=await rolldown({input:fileURLToPath(new URL('../web/src/PracticeWaveform.tsx',import.meta.url)),external:id=>id==='react'||id.startsWith('react/'),tsconfig:fileURLToPath(new URL('../web/tsconfig.json',import.meta.url))});
 await bundle.write({file:`${temporary}/waveform.mjs`,format:'es'});
 const {default:Waveform}=await import(pathToFileURL(`${temporary}/waveform.mjs`));
 const calls=[];
 const wave={sampleRate:48000,sourceFrames:96000,requestedStart:24000,requestedEnd:48000,framesPerCell:1,cells:[{startFrame:24000,endFrame:24001,channels:[{min:-.2,max:.2}]}]};
 const props={wave,start:.6,end:.9,detail:true,disabled:false,onSelect:(boundary,seconds)=>calls.push([boundary,Math.round(seconds*48000)])};
 await React.act(async()=>root.render(React.createElement(Waveform,props)));
 const handles=document.querySelectorAll('[role="slider"]');assert.equal(handles.length,2);
 assert.equal(handles[0].getAttribute('aria-label'),'Selection start handle');
 assert.equal(handles[0].getAttribute('aria-valuetext'),'0.600000 source seconds');
 async function key(node,value,shiftKey=false){await React.act(async()=>node.dispatchEvent(new dom.window.KeyboardEvent('keydown',{key:value,shiftKey,bubbles:true,cancelable:true})));}
 await key(handles[0],'ArrowRight');assert.deepEqual(calls.pop(),['start',28801]);
 await key(handles[1],'ArrowLeft',true);assert.deepEqual(calls.pop(),['end',43190]);
 await key(handles[0],'Home');assert.deepEqual(calls.pop(),['start',24000]);
 await key(handles[1],'End');assert.deepEqual(calls.pop(),['end',48000]);
 const svg=document.querySelector('svg');svg.getBoundingClientRect=()=>({left:100,width:500});
 await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('click',{clientX:350,bubbles:true})));assert.deepEqual(calls.pop(),['start',36000]);
 await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('click',{clientX:700,shiftKey:true,bubbles:true})));assert.deepEqual(calls.pop(),['end',48000]);
 svg.setPointerCapture=()=>{};svg.hasPointerCapture=()=>true;svg.releasePointerCapture=()=>{};
 await React.act(async()=>handles[1].dispatchEvent(new dom.window.MouseEvent('pointerdown',{button:0,bubbles:true,cancelable:true})));
 await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('pointermove',{clientX:400,bubbles:true})));
 assert.deepEqual(calls.pop(),['end',38400]);
 await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('pointerup',{clientX:400,bubbles:true})));
 await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('click',{clientX:400,bubbles:true})));
 assert.equal(calls.length,0,'B dragging accidentally edited A through its synthetic click');
 await React.act(async()=>root.render(React.createElement(Waveform,{...props,disabled:true})));
 await key(handles[0],'ArrowRight');assert.equal(calls.length,0);assert.equal(handles[0].getAttribute('tabindex'),'-1');
 // Drag-to-select works in both directions and does not trigger a stray A
 // click after releasing the captured pointer.
 const ranges=[];
 await React.act(async()=>root.render(React.createElement(Waveform,{...props,onRange:(a,b)=>ranges.push([Math.round(a*48000),Math.round(b*48000)]),position:.7,markers:[{id:'verse',kind:'section',label:'Verse',position:.8}]})));
 assert.equal(document.querySelectorAll('.waveform-playhead').length,1);
 assert.equal(document.querySelector('.waveform-marker text').textContent,'Verse');
 for(const [from,to] of [[200,450],[450,200]]) {
  await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('pointerdown',{button:0,clientX:from,bubbles:true,cancelable:true})));
  await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('pointermove',{clientX:to,bubbles:true})));
  await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('pointerup',{clientX:to,bubbles:true})));
  assert.deepEqual(ranges.at(-1),[28800,40800]);
  await React.act(async()=>svg.dispatchEvent(new dom.window.MouseEvent('click',{clientX:to,bubbles:true})));
  assert.equal(calls.length,0);
 }
 console.log(JSON.stringify({keyboardSourceFramePrecision:true,shiftTenFrames:true,visibleRangeHomeEnd:true,clickCoordinatesClamped:true,dragBDoesNotChangeA:true,busyDisablesHandles:true,dragToSelectBothDirections:true,playheadAndMarkersVisible:true}));
} finally {await React.act(async()=>root.unmount());await bundle?.close();await rm(temporary,{recursive:true,force:true});}
