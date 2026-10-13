import { useRef, useState } from 'react';
import type { FormEvent } from 'react';
import type { BeatGrid, EditCommand, LiveCommand, PracticePlan, Snapshot } from './practice';

export default function PracticePlans({ snapshot, busy, liveAvailable, edit, transport }: { snapshot: Snapshot; busy: boolean; liveAvailable: boolean; edit: (command: EditCommand) => Promise<void>; transport: (command: LiveCommand) => Promise<void> }) {
 const editor = useRef<HTMLDetailsElement>(null);
 const [id, setId] = useState('');
 const [label, setLabel] = useState('Speed ladder');
 const [regionId, setRegionId] = useState('');
 const [start, setStart] = useState('60');
 const [target, setTarget] = useState('100');
 const [increment, setIncrement] = useState('5');
 const [repetitions, setRepetitions] = useState('4');
 const [endBehavior, setEndBehavior] = useState<'stop' | 'hold'>('stop');
 const [countInEnabled,setCountInEnabled]=useState(false);
 const [countInBeats,setCountInBeats]=useState('4');
 const [restEnabled,setRestEnabled]=useState(false);
 const [restSeconds,setRestSeconds]=useState('2');
 const [metronome,setMetronome]=useState(false);
 const [sourceBPM,setSourceBPM]=useState('120');
 const [origin,setOrigin]=useState('0');
 const [meter,setMeter]=useState('4');
 const [clickLevel,setClickLevel]=useState('70');
 const [savedGrid,setSavedGrid]=useState<BeatGrid>();
 const [gridSource,setGridSource]=useState<'manual'|'markers'|'inline'>('manual');
 const plans = snapshot.project.practicePlans ?? [];
 const regions = snapshot.project.annotations.regions ?? [];
 const progress = snapshot.transport?.plan;
 const selected = snapshot.activePracticePlan;
 const loadDefinition = (plan: PracticePlan) => {
  if (editor.current) editor.current.open = true;
  setId(plan.id); setLabel(plan.label); setRegionId(plan.regionId);
  setStart(String(plan.ladder.startSpeed * 100)); setTarget(String(plan.ladder.targetSpeed * 100));
  setCountInEnabled(!!plan.cues?.countInBeats);setCountInBeats(String(plan.cues?.countInBeats||4));
  setRestEnabled(!!plan.cues?.restSeconds);setRestSeconds(String(plan.cues?.restSeconds||2));setMetronome(!!plan.cues?.metronome);
  setSourceBPM(String(plan.cues?.grid?.sourceBPM??120));setOrigin(String((plan.cues?.grid?.originFrame??0)/snapshot.project.track.audio.sampleRate));
  setMeter(String(plan.cues?.grid?.beatsPerBar??4));setClickLevel(String((plan.cues?.clickLevel??.7)*100));setSavedGrid(plan.cues?.grid);setGridSource(plan.cues?.useBeatMarkers?'markers':plan.cues?.grid?.markers?.length?'inline':'manual');
  setIncrement(String(plan.ladder.incrementPoints)); setRepetitions(String(plan.ladder.loopsPerStep)); setEndBehavior(plan.ladder.endBehavior);
 };
 const save = (event: FormEvent) => {
  event.preventDefault();
  const plan: PracticePlan = { id: id || `plan-${crypto.randomUUID()}`, label, regionId: regionId || regions[0]?.id || '', ladder: { startSpeed: Number(start) / 100, targetSpeed: Number(target) / 100, incrementPoints: Number(increment), loopsPerStep: Number(repetitions), endBehavior } };
  if(countInEnabled||restEnabled||metronome){
   const grid:BeatGrid=gridSource==='markers' ? {version:1,beatsPerBar:Number(meter)} : gridSource==='inline'&&savedGrid?.markers?.length ? {...savedGrid,beatsPerBar:Number(meter)} : {version:1,sourceBPM:Number(sourceBPM),originFrame:Math.round(Number(origin)*snapshot.project.track.audio.sampleRate),beatsPerBar:Number(meter)};
   plan.cues={version:1,countInBeats:countInEnabled?Number(countInBeats):0,restSeconds:restEnabled?Number(restSeconds):0,clickLevel:Number(clickLevel)/100,metronome,grid,...(gridSource==='markers'?{useBeatMarkers:true}:{})};
  }
  void edit({type:'set_practice_plans',practicePlans:id ? plans.map(saved => saved.id === id ? plan : saved) : [...plans,plan]});
 };
 return <section aria-label="Saved practice plans">
  <h3>Practice plans</h3>
  <p>Save a named phrase first. A speed ladder repeats that phrase, advances after completed loops, then stops or keeps repeating at its target. Save on this device or download the project to keep its definitions. Count-in runs before a new run or restart. Rest gaps use fixed seconds between phrase repetitions.</p>
  {selected && progress && <div className="practice-plan-progress">
   <p aria-label="Practice plan progress">{selected.label} · {progress.state} · step {progress.step} of {progress.totalSteps} · {progress.completedLoops} of {progress.totalLoops} planned loops · {(progress.speed * 100).toFixed(1)}% ladder speed</p>
   {snapshot.transport?.phase && snapshot.transport.phase !== 'audio' && <p aria-label="Practice phase">{snapshot.transport.phase === 'count_in' ? 'Count-in' : 'Rest gap'} · {((snapshot.transport.phaseRemainingFrames??0)/(snapshot.transport.config.outputRate??snapshot.project.track.audio.sampleRate)).toFixed(2)} seconds remaining</p>}
   {progress.state === 'completed' && <p>Plan finished. Restart begins a new run from its starting speed.</p>}
   <p>Next ladder speed: {(progress.nextSpeed * 100).toFixed(1)}%. Manual speed changes pause the ladder. Resume ladder restores its current step; Play resumes paused playback.</p>
   <div className="practice-actions"><button type="button" disabled={busy || snapshot.transport?.state === 'error' || progress.state !== 'paused' || !snapshot.transport?.config.loopEnabled} onClick={() => void transport({type:'resume_practice_plan'})}>Resume ladder</button><button type="button" disabled={busy || snapshot.transport?.state === 'error'} onClick={() => void transport({type:'clear_practice_plan'})}>Return to manual practice</button></div>
  </div>}
  <ul className="practice-regions">{plans.map(plan => <li key={plan.id}>
   <span>{plan.label} · {regions.find(region => region.id === plan.regionId)?.label} · {plan.ladder.startSpeed * 100}–{plan.ladder.targetSpeed * 100}% · {plan.ladder.loopsPerStep} loops per step</span>
   <button type="button" disabled={busy || !liveAvailable} onClick={() => void transport({type:'select_practice_plan',planId:plan.id})}>Use {plan.label}</button>
   <button type="button" disabled={busy} onClick={() => loadDefinition(plan)}>Edit {plan.label}</button>
   <button type="button" disabled={busy} onClick={() => void edit({type:'set_practice_plans',practicePlans:plans.filter(saved => saved.id!==plan.id)})}>Remove {plan.label}</button>
   {plan.cues && <span>{plan.cues.countInBeats} count-in beats · {plan.cues.restSeconds} seconds rest · metronome {plan.cues.metronome?'on':'off'}</span>}
  </li>)}</ul>
  <details ref={editor}><summary>{id ? 'Edit ladder definition' : 'Create a speed ladder'}</summary>
   <form className="practice-form" onSubmit={save}>
    <label>Plan name<input value={label} disabled={busy} onChange={event => setLabel(event.target.value)} /></label>
    <label>Plan phrase<select value={regionId || regions[0]?.id || ''} disabled={busy || !regions.length} onChange={event => setRegionId(event.target.value)}>{!regions.length && <option value="">Save a named phrase first</option>}{regions.map(region => <option key={region.id} value={region.id}>{region.label}</option>)}</select></label>
    <label>Starting speed (%)<input type="number" step="any" value={start} disabled={busy} onChange={event => setStart(event.target.value)} /></label>
    <label>Target speed (%)<input type="number" step="any" value={target} disabled={busy} onChange={event => setTarget(event.target.value)} /></label>
    <label>Increase (percentage points)<input type="number" step="any" value={increment} disabled={busy} onChange={event => setIncrement(event.target.value)} /></label>
    <label>Loops per step<input type="number" step="1" value={repetitions} disabled={busy} onChange={event => setRepetitions(event.target.value)} /></label>
    <label>At target<select value={endBehavior} disabled={busy} onChange={event => setEndBehavior(event.target.value as 'stop' | 'hold')}><option value="stop">Stop after target repetitions</option><option value="hold">Keep repeating at target</option></select></label>
    <fieldset className="practice-cues" disabled={busy}><legend>Preparation and timing</legend>
     <label><input type="checkbox" checked={countInEnabled} onChange={event=>setCountInEnabled(event.target.checked)} /> Enable count-in</label>
     <label>Count-in beats<input type="number" step="1" value={countInBeats} disabled={!countInEnabled} onChange={event=>setCountInBeats(event.target.value)} /></label>
     <label><input type="checkbox" checked={restEnabled} onChange={event=>setRestEnabled(event.target.checked)} /> Rest between repeats</label>
     <label>Rest gap (seconds)<input type="number" step="0.1" value={restSeconds} disabled={!restEnabled} onChange={event=>setRestSeconds(event.target.value)} /></label>
     <label><input type="checkbox" checked={metronome} onChange={event=>setMetronome(event.target.checked)} /> Metronome during phrase</label>
     <label>Timing source<select value={gridSource} onChange={event=>setGridSource(event.target.value as 'manual'|'markers'|'inline')}><option value="manual">Original recording BPM</option><option value="markers">Saved beat markers</option>{savedGrid?.markers?.length&&<option value="inline">Imported beat grid</option>}</select></label>
     {gridSource==='markers' ? <p>Uses saved beat markers and their measure/beat numbers. Add at least two consecutive beat markers for count-in. Timing changes apply when you select the plan again.</p> : gridSource==='inline'&&savedGrid?.markers?.length ? <div><p>This plan uses {savedGrid.markers.length} saved beat markers. Its timing follows those source positions.</p><button type="button" onClick={()=>{setGridSource('manual');setSourceBPM('120');}}>Use manual BPM instead</button></div> : <>
      <label>Original recording BPM<input type="number" step="any" value={sourceBPM} onChange={event=>setSourceBPM(event.target.value)} /></label>
      <label>First downbeat (source seconds)<input type="number" step={1/snapshot.project.track.audio.sampleRate} value={origin} onChange={event=>setOrigin(event.target.value)} /></label>
     </>}
     <label>Quarter-note beats per bar<input type="number" step="1" value={meter} onChange={event=>setMeter(event.target.value)} /></label>
     <label>Click level (%)<input type="number" step="1" value={clickLevel} onChange={event=>setClickLevel(event.target.value)} /></label>
     <p>Clicks follow the recording’s tempo at the accepted playback speed. Pitch does not retune clicks. Pausing freezes preparation and rest; seeking skips preparation. A rest gap stays the same duration at every speed.</p>
    </fieldset>
    <button disabled={busy || !regions.length}>{id ? 'Save plan changes' : 'Save practice plan'}</button>
    {id && <button type="button" disabled={busy} onClick={() => setId('')}>Create another plan</button>}
   </form>
   {id && <p>Saving a definition keeps the current run unchanged. Use the plan again to start with the updated definition.</p>}
  </details>
 </section>;
}
