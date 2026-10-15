package engine

import (
	"context"
	"errors"
	"math"
)

// Split only at the adapter's DSP boundary. The prefix and edited remainder
// stay in the same generation; neither interrupted piece completes a loop.
func (t *Transport) configureParameters(ctx context.Context, next TransportConfig, manualSpeed bool) (bool, error) {
	d, ok := t.device.(ParameterDevice)
	if !ok || (next.Cues != nil && next.Cues.Metronome) || len(t.plans) == 0 || !t.plans[0].schedule.Audio() || t.scheduleSequence == math.MaxUint64 {
		return false, nil
	}
	if t.planRun != nil && (t.planRun.Snapshot().State == "completed" || (!manualSpeed && t.planRun.Snapshot().State == "running")) {
		return false, nil
	}
	change, err := d.ConfigureParameters(ctx, t.scheduleSequence+1, next.Speed, next.Pitch)
	if err == nil {
		err = t.acknowledge(change.Acknowledgements)
	}
	if err != nil {
		_, stopErr := t.device.Suspend(ctx)
		return false, t.fail(errors.Join(err, stopErr))
	}
	if !change.Applied {
		return false, nil
	}
	invalid := func() (bool, error) {
		_, stopErr := t.device.Suspend(ctx)
		return false, t.fail(errors.Join(errors.New("invalid continuous parameter boundary"), stopErr))
	}
	if len(t.plans) == 0 {
		return invalid()
	}
	old := t.plans[0].schedule
	if !old.Audio() || change.Sequence != old.Sequence || change.OutputFrame <= t.snapshot.ConsumedOutputFrame || change.OutputFrame <= old.OutputStart || change.OutputFrame >= old.OutputEnd || change.SourceFrame <= old.StartFrame || change.SourceFrame >= old.EndFrame {
		return invalid()
	}
	expected := change.OutputFrame + max(int64(1), int64(math.Round(float64(old.EndFrame-change.SourceFrame)*float64(next.OutputRate)/float64(next.SampleRate)/next.Speed)))
	if change.OutputEnd != expected {
		return invalid()
	}
	// Reject a host which maps buffered source to an unrelated output boundary.
	source := old.StartFrame + int64(math.Round(float64(change.OutputFrame-old.OutputStart)*float64(old.EndFrame-old.StartFrame)/float64(old.OutputEnd-old.OutputStart)))
	if math.Abs(float64(source-change.SourceFrame)) > 2 {
		return invalid()
	}
	prefix := old
	prefix.EndFrame = change.SourceFrame
	prefix.OutputEnd = change.OutputFrame
	prefix.EdgeFadeFrames = 0
	prefix.ParameterPrefix = true
	tail := old
	tail.Sequence = t.scheduleSequence + 1
	tail.StartFrame = change.SourceFrame
	tail.OutputStart = change.OutputFrame
	tail.OutputEnd = expected
	tail.Speed = next.Speed
	tail.Pitch = next.Pitch
	tail.EdgeFadeFrames = 0
	tail.ParameterPrefix = false
	prefixGate, err := NewTraversal(prefix)
	if err != nil {
		return invalid()
	}
	if _, err = prefixGate.Acknowledge(OutputAcknowledgement{Generation: prefix.Generation, Sequence: prefix.Sequence, ConsumedOutputFrame: t.snapshot.ConsumedOutputFrame}); err != nil {
		return invalid()
	}
	tailGate, err := NewTraversal(tail)
	if err != nil {
		return invalid()
	}
	t.plans = []plannedTraversal{{prefix, prefixGate, false}, {tail, tailGate, false}}
	t.scheduleSequence = tail.Sequence
	t.snapshot.Config = next
	t.resumeEligible = false
	if t.planRun != nil && manualSpeed {
		t.planRun.Pause()
		t.autoResumePlan = false
	}
	if err = t.fill(ctx, false); err != nil {
		_, stopErr := t.device.Suspend(ctx)
		return false, t.fail(errors.Join(err, stopErr))
	}
	return true, nil
}
