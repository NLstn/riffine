package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strconv"

	"github.com/NLstn/riffine/core/engine"
)

// boundedReport drains stdout with fixed memory, even for a faulty helper.
type boundedReport struct {
	buffer   bytes.Buffer
	overflow bool
}

func (b *boundedReport) Len() int { return b.buffer.Len() }

func (b *boundedReport) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 8192 - b.buffer.Len()
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
	}
	_, _ = b.buffer.Write(p)
	return n, nil
}

// Preview leaves PCM, decode, DSP and device output entirely in the helper.
// The only process messages are the request and one bounded diagnostic report.
func (a Adapter) Preview(ctx context.Context, r engine.PreviewRequest) (engine.PreviewReport, error) {
	var report engine.PreviewReport
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if a.Executable == "" || r.Source == "" || r.StartFrame < 0 || r.EndFrame <= r.StartFrame || r.EndFrame-r.StartFrame > 1<<48 ||
		math.IsNaN(r.Speed) || math.IsInf(r.Speed, 0) || r.Speed < .25 || r.Speed > 2 ||
		math.IsNaN(r.Pitch) || math.IsInf(r.Pitch, 0) || r.Pitch < -12 || r.Pitch > 12 {
		return report, errors.New("invalid streaming preview request or helper")
	}
	mode := "system"
	if a.SilentPreview {
		mode = "null"
	}
	cmd := exec.CommandContext(ctx, a.Executable, "preview", r.Source,
		strconv.FormatInt(r.StartFrame, 10), strconv.FormatInt(r.EndFrame, 10),
		strconv.FormatFloat(r.Speed, 'g', -1, 64), strconv.FormatFloat(r.Pitch, 'g', -1, 64), mode)
	var out boundedReport
	cmd.Stdout = &out
	cmd.Stderr = a.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		return report, fmt.Errorf("preview helper: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	if out.overflow {
		return report, errors.New("preview report exceeds 8 KiB")
	}
	dec := json.NewDecoder(&out.buffer)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&report); err != nil {
		return engine.PreviewReport{}, fmt.Errorf("invalid preview report: %w", err)
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return engine.PreviewReport{}, errors.New("trailing preview report data")
	}
	want := int64(math.Round(float64(r.EndFrame-r.StartFrame) / r.Speed))
	if report.Version != 1 || report.Mode != mode || report.Backend == "" || (a.SilentPreview && report.Backend != "Null") || (!a.SilentPreview && report.Backend == "Null") ||
		report.Channels < 1 || report.Channels > 2 || report.SampleRate < 8000 || report.SampleRate > 192000 ||
		report.OutputFrames != want || report.CopiedFrames != want || report.UnderrunFrames < 0 ||
		report.DecodedBeforeStart < 1 || report.DecodedBeforeStart > r.EndFrame-r.StartFrame ||
		math.IsNaN(report.StartupMilliseconds) || math.IsInf(report.StartupMilliseconds, 0) || report.StartupMilliseconds < 0 {
		return engine.PreviewReport{}, errors.New("unexpected streaming preview diagnostics")
	}
	return report, nil
}
