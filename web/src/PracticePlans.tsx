import { useRef, useState } from 'react';
import type { FormEvent } from 'react';
import type { EditCommand, LiveCommand, PracticePlan, Snapshot } from './practice';

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
 const plans = snapshot.project.practicePlans ?? [];
 const regions = snapshot.project.annotations.regions ?? [];
 const progress = snapshot.transport?.plan;
 const selected = snapshot.activePracticePlan;
 const loadDefinition = (plan: PracticePlan) => {
  if (editor.current) editor.current.open = true;
  setId(plan.id); setLabel(plan.label); setRegionId(plan.regionId);
  setStart(String(plan.ladder.startSpeed * 100)); setTarget(String(plan.ladder.targetSpeed * 100));
  setIncrement(String(plan.ladder.incrementPoints)); setRepetitions(String(plan.ladder.loopsPerStep)); setEndBehavior(plan.ladder.endBehavior);
 };
 const save = (event: FormEvent) => {
  event.preventDefault();
  const plan: PracticePlan = { id: id || `plan-${crypto.randomUUID()}`, label, regionId: regionId || regions[0]?.id || '', ladder: { startSpeed: Number(start) / 100, targetSpeed: Number(target) / 100, incrementPoints: Number(increment), loopsPerStep: Number(repetitions), endBehavior } };
  void edit({type:'set_practice_plans',practicePlans:id ? plans.map(saved => saved.id === id ? plan : saved) : [...plans,plan]});
 };
 return <section aria-label="Saved practice plans">
  <h3>Practice plans</h3>
  <p>Save a named phrase first. A speed ladder repeats that phrase, advances after completed loops, then stops or keeps repeating at its target. Download the project to keep its definitions.</p>
  {selected && progress && <div className="practice-plan-progress">
   <p aria-label="Practice plan progress">{selected.label} · {progress.state} · step {progress.step} of {progress.totalSteps} · {progress.completedLoops} of {progress.totalLoops} planned loops · {(progress.speed * 100).toFixed(1)}% ladder speed</p>
   {progress.state === 'completed' && <p>Plan finished. Restart begins a new run from its starting speed.</p>}
   <p>Next ladder speed: {(progress.nextSpeed * 100).toFixed(1)}%. Manual speed changes pause the ladder. Resume ladder restores its current step; Play resumes paused playback.</p>
   <div className="practice-actions"><button type="button" disabled={busy || snapshot.transport?.state === 'error' || progress.state !== 'paused' || !snapshot.transport?.config.loopEnabled} onClick={() => void transport({type:'resume_practice_plan'})}>Resume ladder</button><button type="button" disabled={busy || snapshot.transport?.state === 'error'} onClick={() => void transport({type:'clear_practice_plan'})}>Return to manual practice</button></div>
  </div>}
  <ul className="practice-regions">{plans.map(plan => <li key={plan.id}>
   <span>{plan.label} · {regions.find(region => region.id === plan.regionId)?.label} · {plan.ladder.startSpeed * 100}–{plan.ladder.targetSpeed * 100}% · {plan.ladder.loopsPerStep} loops per step</span>
   <button type="button" disabled={busy || !liveAvailable} onClick={() => void transport({type:'select_practice_plan',planId:plan.id})}>Use {plan.label}</button>
   <button type="button" disabled={busy} onClick={() => loadDefinition(plan)}>Edit {plan.label}</button>
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
    <button disabled={busy || !regions.length}>{id ? 'Save plan changes' : 'Save practice plan'}</button>
    {id && <button type="button" disabled={busy} onClick={() => setId('')}>Create another plan</button>}
   </form>
   {id && <p>Saving a definition keeps the current run unchanged. Use the plan again to start with the updated definition.</p>}
  </details>
 </section>;
}
