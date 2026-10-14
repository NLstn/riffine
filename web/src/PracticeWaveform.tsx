import { useRef } from "react";
import type { KeyboardEvent, PointerEvent } from "react";
import type { Annotations, WaveWindow } from "./practice";

type Boundary = "start" | "end";
// Gestures use source-frame coordinates; the workspace sends accepted ranges
// through the same shared-core command path as the numeric loop controls.
export default function PracticeWaveform({
  wave,
  start,
  end,
  detail,
  disabled,
  onSelect,
  onRange,
  position = 0,
  markers = [],
}: {
  wave: WaveWindow;
  start: number;
  end: number;
  detail: boolean;
  disabled: boolean;
  onSelect: (boundary: Boundary, seconds: number) => void;
  onRange?: (start: number, end: number) => void;
  position?: number;
  markers?: Annotations["markers"];
}) {
  const dragging = useRef<Boundary | undefined>(undefined);
  const ignoreClick = useRef(false);
  const rangeAnchor = useRef<number | undefined>(undefined);
  const rangeMoved = useRef(false);
  const length = Math.max(1, wave.requestedEnd - wave.requestedStart);
  const x = (frame: number) => ((frame - wave.requestedStart) / length) * 1000;
  const startFrame = Math.round(start * wave.sampleRate),
    endFrame = Math.round(end * wave.sampleRate);
  const finiteStart = Number.isFinite(startFrame),
    finiteEnd = Number.isFinite(endFrame);
  const frameAt = (
    event: PointerEvent<SVGSVGElement> | React.MouseEvent<SVGSVGElement>,
  ) => {
    const box = event.currentTarget.getBoundingClientRect();
    return (
      wave.requestedStart +
      Math.round(
        Math.max(0, Math.min(1, (event.clientX - box.left) / box.width)) *
          length,
      )
    );
  };
  const keydown = (event: KeyboardEvent<SVGGElement>, boundary: Boundary) => {
    if (disabled) return;
    const current = boundary === "start" ? startFrame : endFrame;
    let frame: number;
    if (event.key === "Home") frame = wave.requestedStart;
    else if (event.key === "End") frame = wave.requestedEnd;
    else if (event.key === "ArrowLeft" || event.key === "ArrowDown")
      frame = current - (event.shiftKey ? 10 : 1);
    else if (event.key === "ArrowRight" || event.key === "ArrowUp")
      frame = current + (event.shiftKey ? 10 : 1);
    else return;
    event.preventDefault();
    event.stopPropagation();
    onSelect(
      boundary,
      Math.max(wave.requestedStart, Math.min(wave.requestedEnd, frame)) /
        wave.sampleRate,
    );
  };
  const grab = (event: PointerEvent<SVGGElement>, boundary: Boundary) => {
    if (disabled || event.button !== 0) return;
    event.preventDefault();
    event.stopPropagation();
    dragging.current = boundary;
    ignoreClick.current = true;
    event.currentTarget.ownerSVGElement?.setPointerCapture(event.pointerId);
    event.currentTarget.focus();
  };
  return (
    <div className="waveform-view" role="group" aria-label="Waveform selection">
      <svg
        className="practice-waveform"
        viewBox="0 0 1000 240"
        preserveAspectRatio="none"
        role="group"
        aria-label={`${detail ? "Exact selection" : "Track overview"} waveform`}
        onClick={(event) => {
          if (ignoreClick.current) {
            ignoreClick.current = false;
            return;
          }
          if (disabled || (event.target as Element).closest("[data-boundary]"))
            return;
          onSelect(
            event.shiftKey ? "end" : "start",
            frameAt(event) / wave.sampleRate,
          );
        }}
        onPointerDown={(event) => {
          ignoreClick.current = false;
          if (
            disabled ||
            event.button !== 0 ||
            (event.target as Element).closest("[data-boundary]") ||
            !onRange ||
            event.shiftKey
          )
            return;
          rangeAnchor.current = frameAt(event);
          rangeMoved.current = false;
          event.currentTarget.setPointerCapture(event.pointerId);
        }}
        onPointerMove={(event) => {
          if (disabled) return;
          const frame = frameAt(event);
          if (dragging.current)
            onSelect(dragging.current, frame / wave.sampleRate);
          else if (
            rangeAnchor.current !== undefined &&
            Math.abs(frame - rangeAnchor.current) > 0 &&
            onRange
          ) {
            rangeMoved.current = true;
            ignoreClick.current = true;
            onRange(
              Math.min(frame, rangeAnchor.current) / wave.sampleRate,
              Math.max(frame, rangeAnchor.current) / wave.sampleRate,
            );
          }
        }}
        onPointerUp={(event) => {
          if (
            !disabled &&
            rangeMoved.current &&
            rangeAnchor.current !== undefined &&
            onRange
          ) {
            const frame = frameAt(event);
            if (frame !== rangeAnchor.current)
              onRange(
                Math.min(frame, rangeAnchor.current) / wave.sampleRate,
                Math.max(frame, rangeAnchor.current) / wave.sampleRate,
              );
          }
          dragging.current = undefined;
          rangeAnchor.current = undefined;
          if (event.currentTarget.hasPointerCapture(event.pointerId))
            event.currentTarget.releasePointerCapture(event.pointerId);
        }}
        onPointerCancel={() => {
          dragging.current = undefined;
          rangeAnchor.current = undefined;
          ignoreClick.current = false;
        }}
        onLostPointerCapture={() => {
          dragging.current = undefined;
          rangeAnchor.current = undefined;
        }}
      >
        <g aria-hidden="true">
          <rect x="0" y="0" width="1000" height="240" fill="transparent" />
          {finiteStart && finiteEnd && endFrame > startFrame && (
            <rect
              x={Math.max(0, x(startFrame))}
              y="0"
              width={Math.max(
                0,
                Math.min(1000, x(endFrame)) - Math.max(0, x(startFrame)),
              )}
              height="240"
              className="practice-waveform-selection"
            />
          )}
          {[0, 250, 500, 750, 1000].map((tick) => (
            <line
              key={tick}
              x1={tick}
              x2={tick}
              y1="0"
              y2="240"
              className="waveform-grid"
            />
          ))}
          {[60, 180].map((channel) => (
            <line
              key={channel}
              x1="0"
              x2="1000"
              y1={channel}
              y2={channel}
              className="waveform-grid"
            />
          ))}
          {wave.cells.map((cell, index) =>
            cell.channels.map((peak, channel) => (
              <line
                key={`${index}-${channel}`}
                x1={x(cell.startFrame)}
                x2={x(cell.startFrame)}
                y1={
                  (cell.channels.length === 1 ? 120 : 60 + channel * 120) -
                  peak.max * 50
                }
                y2={
                  (cell.channels.length === 1 ? 120 : 60 + channel * 120) -
                  peak.min * 50
                }
                stroke="currentColor"
                strokeWidth="2"
              />
            )),
          )}
          {markers
            ?.filter(
              (marker) =>
                marker.position * wave.sampleRate >= wave.requestedStart &&
                marker.position * wave.sampleRate <= wave.requestedEnd,
            )
            .map((marker) => (
              <g key={marker.id} className="waveform-marker">
                <line
                  x1={x(marker.position * wave.sampleRate)}
                  x2={x(marker.position * wave.sampleRate)}
                  y1="26"
                  y2="240"
                />
                <text
                  x={Math.max(
                    4,
                    Math.min(880, x(marker.position * wave.sampleRate) + 5),
                  )}
                  y="22"
                >
                  {marker.label}
                </text>
              </g>
            ))}
          {position * wave.sampleRate >= wave.requestedStart &&
            position * wave.sampleRate <= wave.requestedEnd && (
              <line
                className="waveform-playhead"
                x1={x(position * wave.sampleRate)}
                x2={x(position * wave.sampleRate)}
                y1="0"
                y2="240"
              />
            )}
        </g>
        {(["start", "end"] as const).map((boundary) => {
          const frame = boundary === "start" ? startFrame : endFrame;
          if (
            !Number.isFinite(frame) ||
            frame < wave.requestedStart ||
            frame > wave.requestedEnd
          )
            return null;
          const labelX = Math.max(12, Math.min(988, x(frame))) - x(frame);
          return (
            <g
              key={boundary}
              data-boundary={boundary}
              className="practice-waveform-handle"
              transform={`translate(${x(frame)},0)`}
              role="slider"
              tabIndex={disabled ? -1 : 0}
              aria-label={`Selection ${boundary} handle`}
              aria-disabled={disabled}
              aria-valuemin={wave.requestedStart}
              aria-valuemax={wave.requestedEnd}
              aria-valuenow={frame}
              aria-valuetext={`${(frame / wave.sampleRate).toFixed(6)} source seconds`}
              onKeyDown={(event) => keydown(event, boundary)}
              onPointerDown={(event) => grab(event, boundary)}
            >
              <rect
                x={Math.max(-22, Math.min(956, x(frame)) - x(frame))}
                y="0"
                width="44"
                height="240"
                fill="transparent"
              />
              <line
                x1="0"
                x2="0"
                y1="0"
                y2="240"
                stroke="currentColor"
                strokeWidth="3"
              />
              <rect
                x={labelX - 12}
                y={boundary === "start" ? 0 : 216}
                width="24"
                height="24"
                rx="4"
                fill="currentColor"
              />
              <text
                x={labelX}
                y={boundary === "start" ? 17 : 233}
                textAnchor="middle"
                className="practice-waveform-handle-label"
              >
                {boundary === "start" ? "A" : "B"}
              </text>
            </g>
          );
        })}
      </svg>
      <div className="waveform-ruler" aria-hidden="true">
        {[0, 0.25, 0.5, 0.75, 1].map((fraction) => {
          const seconds =
            (wave.requestedStart + fraction * length) / wave.sampleRate;
          return (
            <span key={fraction}>
              {Math.floor(seconds / 60)}:
              {(seconds % 60).toFixed(1).padStart(4, "0")}
            </span>
          );
        })}
      </div>
      <p className="waveform-hint">
        Drag to select a loop · Click for A, Shift-click for B · Arrow keys
        fine-tune the handles
      </p>
    </div>
  );
}
