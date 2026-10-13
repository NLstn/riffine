import { useRef } from 'react';
import type { KeyboardEvent, PointerEvent } from 'react';
import type { WaveWindow } from './practice';

type Boundary = 'start' | 'end';
// Gesture coordinates edit the form only. The shared core accepts or rejects
// source-time selections when the user applies the A/B loop.
export default function PracticeWaveform({ wave, start, end, detail, disabled, onSelect }: {
 wave: WaveWindow; start: number; end: number; detail: boolean; disabled: boolean;
 onSelect: (boundary: Boundary, seconds: number) => void;
}) {
 const dragging = useRef<Boundary | undefined>(undefined);
 const ignoreClick = useRef(false);
 const length = wave.requestedEnd-wave.requestedStart;
 const x = (frame: number) => (frame-wave.requestedStart)/length*1000;
 const startFrame = Math.round(start*wave.sampleRate), endFrame = Math.round(end*wave.sampleRate);
 const finiteStart = Number.isFinite(startFrame), finiteEnd = Number.isFinite(endFrame);
 const frameAt = (event: PointerEvent<SVGSVGElement> | React.MouseEvent<SVGSVGElement>) => {
  const box = event.currentTarget.getBoundingClientRect();
  return wave.requestedStart+Math.round(Math.max(0,Math.min(1,(event.clientX-box.left)/box.width))*length);
 };
 const keydown = (event: KeyboardEvent<SVGGElement>, boundary: Boundary) => {
  if (disabled) return;
  const current = boundary === 'start' ? startFrame : endFrame;
  let frame: number;
  if (event.key === 'Home') frame = wave.requestedStart;
  else if (event.key === 'End') frame = wave.requestedEnd;
  else if (event.key === 'ArrowLeft' || event.key === 'ArrowDown') frame = current-(event.shiftKey ? 10 : 1);
  else if (event.key === 'ArrowRight' || event.key === 'ArrowUp') frame = current+(event.shiftKey ? 10 : 1);
  else return;
  event.preventDefault(); event.stopPropagation();
  onSelect(boundary,Math.max(wave.requestedStart,Math.min(wave.requestedEnd,frame))/wave.sampleRate);
 };
 const grab = (event: PointerEvent<SVGGElement>, boundary: Boundary) => {
  if (disabled || event.button !== 0) return;
  event.preventDefault(); event.stopPropagation(); dragging.current=boundary; ignoreClick.current=true;
  event.currentTarget.ownerSVGElement?.setPointerCapture(event.pointerId);
  event.currentTarget.focus();
 };
 return <div role="group" aria-label="Waveform selection">
  <svg className="practice-waveform" viewBox="0 0 1000 160" role="group" aria-label={`${detail ? 'Exact selection' : 'Track overview'} waveform`} onClick={event => {
   if (ignoreClick.current) {ignoreClick.current=false; return;}
   if (disabled || (event.target as Element).closest('[data-boundary]')) return;
   onSelect(event.shiftKey ? 'end' : 'start',frameAt(event)/wave.sampleRate);
  }} onPointerDown={() => {ignoreClick.current=false;}} onPointerMove={event => {if (!disabled && dragging.current) onSelect(dragging.current,frameAt(event)/wave.sampleRate);}}
  onPointerUp={event => {dragging.current=undefined; if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);}}
  onPointerCancel={() => {dragging.current=undefined; ignoreClick.current=false;}} onLostPointerCapture={() => {dragging.current=undefined;}}>
   <g aria-hidden="true">
    <rect x="0" y="0" width="1000" height="160" fill="transparent" />
    {finiteStart && finiteEnd && endFrame > startFrame && <rect x={Math.max(0,x(startFrame))} y="0" width={Math.max(0,Math.min(1000,x(endFrame))-Math.max(0,x(startFrame)))} height="160" className="practice-waveform-selection" />}
    {wave.cells.map((cell,index) => cell.channels.map((peak,channel) => <line key={`${index}-${channel}`} x1={x(cell.startFrame)} x2={x(cell.startFrame)} y1={40+channel*80-peak.max*35} y2={40+channel*80-peak.min*35} stroke="currentColor" strokeWidth="2" />))}
   </g>
   {(['start','end'] as const).map(boundary => {
    const frame = boundary === 'start' ? startFrame : endFrame;
    if (!Number.isFinite(frame) || frame < wave.requestedStart || frame > wave.requestedEnd) return null;
    const labelX = Math.max(12,Math.min(988,x(frame)))-x(frame);
    return <g key={boundary} data-boundary={boundary} className="practice-waveform-handle" transform={`translate(${x(frame)},0)`}
     role="slider" tabIndex={disabled ? -1 : 0} aria-label={`Selection ${boundary} handle`} aria-disabled={disabled}
     aria-valuemin={wave.requestedStart} aria-valuemax={wave.requestedEnd} aria-valuenow={frame} aria-valuetext={`${(frame/wave.sampleRate).toFixed(6)} source seconds`}
     onKeyDown={event => keydown(event,boundary)} onPointerDown={event => grab(event,boundary)}>
     <rect x={Math.max(-22,Math.min(956,x(frame))-x(frame))} y="0" width="44" height="160" fill="transparent" />
     <line x1="0" x2="0" y1="0" y2="160" stroke="currentColor" strokeWidth="3" />
     <rect x={labelX-12} y={boundary === 'start' ? 0 : 136} width="24" height="24" rx="4" fill="currentColor" />
     <text x={labelX} y={boundary === 'start' ? 17 : 153} textAnchor="middle" className="practice-waveform-handle-label">{boundary === 'start' ? 'A' : 'B'}</text>
    </g>;
   })}
  </svg>
  <p>{(wave.requestedStart/wave.sampleRate).toFixed(6)}–{(wave.requestedEnd/wave.sampleRate).toFixed(6)} source seconds · {wave.framesPerCell} frames per cell</p>
  <p>Drag A/B or use their arrow keys to move one source frame (Shift moves ten). Home/End use the visible range. Click the waveform for A, Shift-click for B, then Apply A/B loop.</p>
 </div>;
}
