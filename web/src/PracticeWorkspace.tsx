import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { BrowserPractice } from './practice';
import type { EditCommand, PracticeFacade, PracticeResponse, Snapshot, WaveWindow } from './practice';

export default function PracticeWorkspace({ source, onClose, createFacade = () => new BrowserPractice() }: { source?: File; onClose: () => void; createFacade?: () => PracticeFacade }) {
 const facade = useRef<PracticeFacade | null>(null);
 const sourceInput = useRef<HTMLInputElement>(null);
 const projectInput = useRef<HTMLInputElement>(null);
 const [selectedSource, setSelectedSource] = useState(source);
 const [projectFile, setProjectFile] = useState<File>();
 const [snapshot, setSnapshot] = useState<Snapshot>();
 const [wave, setWave] = useState<WaveWindow>();
 const [busy, setBusy] = useState(false);
 const [error, setError] = useState('');
 const [status, setStatus] = useState('Choose an original WAV and optionally a saved project.');
 const [start, setStart] = useState('0');
 const [end, setEnd] = useState('0');
 const [speed, setSpeed] = useState('100');
 const [pitch, setPitch] = useState('0');
 const [label, setLabel] = useState('Phrase');
 const [notes, setNotes] = useState('');
 const [phraseNotes, setPhraseNotes] = useState('');
 const [markerLabel, setMarkerLabel] = useState('Section');
 const [detailView, setDetailView] = useState(false);
 const install = (response: PracticeResponse, syncForms = false) => {
  if (response.snapshot) {
   const current = response.snapshot;
   setSnapshot(current); if (syncForms) { setSpeed(String(current.project.settings.speed * 100)); setPitch(String(current.project.settings.pitch));
   setStart(String(current.project.settings.loop.start)); setEnd(String(current.project.settings.loop.end)); setNotes(current.project.annotations.notes ?? ''); }
  }
  if (response.error) throw new Error(`${response.error.code}: ${response.error.message}`);
  if (response.window) setWave(response.window);
  return response;
 };
 const run = async (action: () => Promise<void>) => { setBusy(true); setError(''); try { await action(); } catch (failure) { setError(String(failure instanceof Error ? failure.message : failure)); } finally { setBusy(false); } };
 const open = async (original: File, saved?: File) => {
  const client = facade.current;
  if (!client) return;
  setWave(undefined); setDetailView(false);
  const result = install(await client.open(original, saved), true);
  if (result.snapshot) {
   install(await client.waveform(0, result.snapshot.project.track.audio.frames), false);
   setStatus(saved ? 'Saved project reopened and original identity verified.' : 'Original fingerprinted. Practice changes stay local.');
  }
 };
 useEffect(() => {
  const client = createFacade(); facade.current = client;
  if (source) void run(() => open(source));
  return () => { client.dispose(); facade.current = null; };
  // One worker per mounted workspace. Injected facades are fixed for its lifetime.
  // eslint-disable-next-line react-hooks/exhaustive-deps
 }, []);
 const edit = (command: EditCommand) => run(async () => {
  if (!facade.current || !snapshot) return;
  install(await facade.current.command(snapshot, command)); setStatus('Changes applied by the shared core. Download the project to keep them.');
 });
 const parameters = (event: FormEvent) => { event.preventDefault(); void edit({ type: 'set_parameters', speed: Number(speed) / 100, pitch: Number(pitch) }); };
 const loop = (enabled: boolean) => edit({ type: 'set_loop', loop: { enabled, start: Number(start), end: Number(end) } });
 const annotations = snapshot?.project.annotations ?? {};
 // Region and named-loop references are built together; the core validates them.
 const savePhrase = () => {
  const regionId = `region-${crypto.randomUUID()}`;
  void edit({ type: 'set_annotations', annotations: { ...annotations, regions: [...(annotations.regions ?? []), { id: regionId, label, start: Number(start), end: Number(end), notes: phraseNotes }], loops: [...(annotations.loops ?? []), { id: `loop-${crypto.randomUUID()}`, label, regionId }] } });
 };
 const loadProject = (file?: File) => { if (!file) return; setProjectFile(file); if (selectedSource) void run(() => open(selectedSource, file)); };
 const chooseSource = (file?: File) => { if (!file) return; setSelectedSource(file); void run(() => open(file, projectFile)); };
 const exportProject = () => run(async () => {
  if (!facade.current) return;
  const result = install(await facade.current.exportProject());
  if (result.document) {
   const url = URL.createObjectURL(new Blob([result.document], { type: 'application/json' }));
   const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'practice.riffine.json'; anchor.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
   setStatus('Project downloaded. Reopen it with the matching original audio.');
  }
 });
 const zoom = () => run(async () => {
  if (!facade.current || !snapshot) return;
  const rate = snapshot.project.track.audio.sampleRate;
  install(await facade.current.waveform(Math.round(Number(start) * rate), Math.round(Number(end) * rate), true), false); setDetailView(true);
 });
 const overview = () => run(async () => {
  if (!facade.current || !snapshot) return;
  install(await facade.current.waveform(0, snapshot.project.track.audio.frames), false); setDetailView(false);
 });
 const duration = snapshot ? snapshot.project.track.audio.frames / snapshot.project.track.audio.sampleRate : 0;
 const viewStart = wave?.requestedStart ?? 0;
 const viewEnd = wave?.requestedEnd ?? 1;
 const viewLength = viewEnd - viewStart;
 const selectInView = (event: React.MouseEvent<SVGSVGElement>) => {
  if (!wave) return;
  const rect = event.currentTarget.getBoundingClientRect();
  const frame = viewStart + Math.round(((event.clientX - rect.left) / rect.width) * viewLength);
  const value = String(frame / wave.sampleRate);
  if (event.shiftKey) setEnd(value); else setStart(value);
 };
 return <section className="practice-workspace" aria-label="Practice workspace">
  <div className="practice-heading"><div><h2>Practice workspace</h2><p>{snapshot?.project.name ?? 'Open your original audio'}</p></div><button type="button" onClick={onClose}>Close practice</button></div>
  <div className="practice-actions">
   <button type="button" disabled={busy} onClick={() => sourceInput.current?.click()}>Choose original WAV / relink</button>
   <button type="button" disabled={busy} onClick={() => void run(async () => {
    const response = await fetch('/practice/demo.wav'); if (!response.ok) throw new Error('Generated demo is missing; run the browser build script.');
    const file = new File([await response.blob()], 'demo.wav', { type: 'audio/wav' }); setSelectedSource(file); setProjectFile(undefined); await open(file);
   })}>Try generated demo</button>
   <button type="button" disabled={busy} onClick={() => projectInput.current?.click()}>Reopen project</button>
   <button type="button" disabled={busy || !snapshot} onClick={() => void exportProject()}>Download project</button>
   {busy && <button type="button" onClick={() => facade.current?.cancel()}>Cancel operation</button>}
  </div>
  <input className="visually-hidden" ref={sourceInput} type="file" accept=".wav,.wave,audio/wav" onChange={event => { chooseSource(event.target.files?.[0]); event.target.value = ''; }} />
  <input className="visually-hidden" ref={projectInput} type="file" accept=".json,application/json" onChange={event => { loadProject(event.target.files?.[0]); event.target.value = ''; }} />
  {error && <p role="alert" className="practice-error">{error}</p>}<p role="status">{busy ? 'Reading local source…' : status}</p>
  {snapshot && <>
   <p className="practice-capability">{snapshot.capabilities.formats.join(', ')} · {duration.toFixed(3)} source seconds · Practice audio playback is unavailable in this browser adapter.</p>
   <div className="practice-actions"><button type="button" disabled={!snapshot.capabilities.liveTransport || busy}>Play / pause</button><button type="button" disabled={busy} onClick={() => void overview()}>Full overview</button><button type="button" disabled={busy} onClick={() => void zoom()}>Exact selection zoom</button></div>
   {wave && <><svg className="practice-waveform" viewBox="0 0 1000 160" role="img" aria-label={`${detailView ? 'Exact selection' : 'Track overview'} waveform; click sets selection start, Shift-click sets end`} onClick={selectInView}>
    <rect x="0" y="0" width="1000" height="160" fill="transparent" />
    {wave.cells.map((cell, index) => cell.channels.map((peak, channel) => <line key={`${index}-${channel}`} x1={(cell.startFrame - viewStart) / viewLength * 1000} x2={(cell.startFrame - viewStart) / viewLength * 1000} y1={40 + channel * 80 - peak.max * 35} y2={40 + channel * 80 - peak.min * 35} stroke="currentColor" strokeWidth="2" />))}
   </svg><p>{(viewStart / wave.sampleRate).toFixed(6)}–{(viewEnd / wave.sampleRate).toFixed(6)} source seconds · {wave.framesPerCell} frames per cell</p></>}
   <form className="practice-form" onSubmit={parameters}><label>Speed (%)<input type="number" step="0.1" value={speed} onChange={event => setSpeed(event.target.value)} disabled={busy} /></label><label>Pitch (semitones)<input type="number" step="0.1" value={pitch} onChange={event => setPitch(event.target.value)} disabled={busy} /></label><button disabled={busy}>Apply speed / pitch</button></form>
   <div className="practice-form"><label>Selection start (source seconds)<input type="number" step={1 / snapshot.project.track.audio.sampleRate} value={start} onChange={event => setStart(event.target.value)} disabled={busy} /></label><label>Selection end (source seconds)<input type="number" step={1 / snapshot.project.track.audio.sampleRate} value={end} onChange={event => setEnd(event.target.value)} disabled={busy} /></label><button type="button" disabled={busy} onClick={() => void loop(true)}>Apply A/B loop</button><button type="button" disabled={busy} onClick={() => void loop(false)}>Disable loop</button></div>
   <p>Saved loop: {snapshot.project.settings.loop.enabled ? 'enabled' : 'disabled'} · {snapshot.project.settings.loop.start.toFixed(6)}–{snapshot.project.settings.loop.end.toFixed(6)} source seconds</p>
   <div className="practice-form"><label>Phrase name<input value={label} onChange={event => setLabel(event.target.value)} disabled={busy} /></label><button type="button" disabled={busy} onClick={savePhrase}>Save named phrase / loop</button></div>
   <label className="practice-notes">Phrase notes<textarea value={phraseNotes} onChange={event => setPhraseNotes(event.target.value)} disabled={busy} /></label>
   <label className="practice-notes">Project notes<textarea value={notes} onChange={event => setNotes(event.target.value)} disabled={busy} /></label><button type="button" disabled={busy} onClick={() => void edit({ type: 'set_annotations', annotations: { ...annotations, notes } })}>Save project notes</button>
   <div className="practice-form"><label>Section marker name<input value={markerLabel} onChange={event => setMarkerLabel(event.target.value)} disabled={busy} /></label><button type="button" disabled={busy} onClick={() => void edit({ type: 'set_annotations', annotations: { ...annotations, markers: [...(annotations.markers ?? []), { id: `marker-${crypto.randomUUID()}`, kind: 'section', label: markerLabel, position: Number(start) }] } })}>Add marker at start</button></div>
   <ul className="practice-regions">{annotations.regions?.map(region => <li key={region.id}><button type="button" disabled={busy} onClick={() => { setStart(String(region.start)); setEnd(String(region.end)); setLabel(region.label); setPhraseNotes(region.notes ?? ''); }}>{region.label}: {region.start.toFixed(3)}–{region.end.toFixed(3)}s</button></li>)}</ul>
  </>}
 </section>;
}
