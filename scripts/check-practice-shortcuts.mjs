// Validate actual foreground shortcut settings and key discovery. DOM typing,
// native activation and command delivery are also checked in the browser UI.
import assert from 'node:assert/strict';
import {defaultShortcuts,validateShortcuts,loadShortcuts,saveShortcuts,matchShortcut} from '../web/src/practice-shortcuts.ts';
globalThis.Element=class {};
const data=new Map();let writes=0;
globalThis.localStorage={getItem:key=>data.get(key)??null,setItem:(key,value)=>{writes++;data.set(key,value);}};
const input=key=>({key,ctrlKey:false,metaKey:false,altKey:false,shiftKey:false,isComposing:false,target:null});
assert.deepEqual(loadShortcuts().settings,defaultShortcuts);
assert.equal(matchShortcut(input(' '),defaultShortcuts),'play_pause');
assert.equal(matchShortcut(input('r'),defaultShortcuts),'restart');
assert.equal(matchShortcut(input('S'),defaultShortcuts),'stop');
assert.equal(matchShortcut(input('Tab'),defaultShortcuts),undefined);
for(const flag of ['ctrlKey','metaKey','altKey','shiftKey','isComposing']) assert.equal(matchShortcut({...input('r'),[flag]:true},defaultShortcuts),undefined);
assert.equal(matchShortcut(input('r'),{...defaultShortcuts,enabled:false}),undefined);
const changed={...defaultShortcuts,bindings:{play_pause:'P',restart:'A',stop:''}};
assert.deepEqual(saveShortcuts(changed),changed);assert.deepEqual(loadShortcuts().settings,changed);
assert.equal(matchShortcut(input(' '),changed),undefined);assert.equal(matchShortcut(input('p'),changed),'play_pause');
const before=[...data.entries()],savedWrites=writes;
assert.throws(()=>saveShortcuts({...changed,bindings:{play_pause:'P',restart:'P',stop:'S'}}),/different key/);
assert.equal(writes,savedWrites);assert.deepEqual([...data.entries()],before);
for(const malformed of [null,{},{...changed,version:2},{...changed,enabled:1},{...changed,bindings:{...changed.bindings,restart:'Escape'}},{...changed,bindings:{...changed.bindings,stop:null}}]) assert.throws(()=>validateShortcuts(malformed));
const key=[...data.keys()][0];data.set(key,'{invalid');
assert(loadShortcuts().warning);assert.deepEqual(loadShortcuts().settings,defaultShortcuts);assert.equal(data.get(key),'{invalid');
localStorage.setItem=()=>{throw new Error('storage unavailable');};
assert.throws(()=>saveShortcuts(changed),/could not be saved/);
assert.equal(data.get(key),'{invalid');
console.log(JSON.stringify({defaultsAndRemapping:true,modifiersAndCompositionPreserved:true,disabledShortcutsIgnored:true,conflictsDoNotOverwritePreferences:true,malformedPreferencesPreserved:true,storageFailureReported:true,physicalPedalEvidence:false},null,2));
