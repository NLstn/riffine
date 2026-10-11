package hosts

import (
	"context"
	"errors"

	"github.com/NLstn/riffine/core/engine"
)

// All transport decisions execute in Go, including in the WASM worker and C
// archive. This adapter provides explicit synthetic consumption, not audio.
type transportOutput struct {
	schedules []engine.Schedule
	acks      []engine.OutputAcknowledgement
}

func (d *transportOutput) Schedule(_ context.Context, s engine.Schedule) error {
	d.schedules = append(d.schedules, s)
	return nil
}
func (d *transportOutput) Reset(context.Context, uint64) error { d.acks = nil; return nil }
func (*transportOutput) Start(context.Context) error           { return nil }
func (d *transportOutput) Suspend(context.Context) ([]engine.OutputAcknowledgement, error) {
	a := d.acks
	d.acks = nil
	return a, nil
}
func (d *transportOutput) DrainAcknowledgements(context.Context, int) ([]engine.OutputAcknowledgement, error) {
	a := d.acks
	d.acks = nil
	return a, nil
}

type TransportReport struct {
	Paused    engine.TransportSnapshot `json:"paused"`
	Resumed   engine.TransportSnapshot `json:"resumed"`
	Completed engine.TransportSnapshot `json:"completed"`
	Events    []engine.TransportEvent  `json:"events"`
}

func transportConformance(ctx context.Context, q engine.RenderRequest, frames int64, rate int) (TransportReport, error) {
	var result TransportReport
	device := &transportOutput{}
	tr, err := engine.NewTransport(device)
	if err != nil {
		return result, err
	}
	err = tr.Load(ctx, engine.TransportConfig{Source: q.Source, Frames: frames, SampleRate: rate, OutputRate: rate, Speed: q.Speed, Pitch: q.Pitch, LoopEnabled: true, LoopStart: q.StartFrame, LoopEnd: q.EndFrame})
	if err != nil {
		return result, err
	}
	dispatch := func(kind string) (engine.TransportSnapshot, error) {
		return tr.Dispatch(ctx, engine.TransportCommand{Version: 1, Sequence: tr.Snapshot().Sequence + 1, Type: kind})
	}
	if _, err = dispatch("play"); err != nil {
		return result, err
	}
	first := device.schedules[0]
	device.acks = []engine.OutputAcknowledgement{{Generation: first.Generation, Sequence: first.Sequence, ConsumedOutputFrame: first.OutputEnd / 2}}
	result.Paused, err = dispatch("pause")
	if err != nil {
		return result, err
	}
	result.Resumed, err = dispatch("play")
	if err != nil {
		return result, err
	}
	partial, full := device.schedules[2], device.schedules[3]
	device.acks = []engine.OutputAcknowledgement{
		{Generation: first.Generation, Sequence: first.Sequence, ConsumedOutputFrame: first.OutputEnd},
		{Generation: partial.Generation, Sequence: partial.Sequence, ConsumedOutputFrame: partial.OutputEnd},
		{Generation: full.Generation, Sequence: full.Sequence, ConsumedOutputFrame: full.OutputEnd},
		{Generation: full.Generation, Sequence: full.Sequence, ConsumedOutputFrame: full.OutputEnd},
	}
	result.Completed, err = tr.Poll(ctx)
	if err != nil {
		return result, err
	}
	result.Events = tr.DrainEvents()
	if result.Paused.State != "paused" || result.Paused.SourceFrame <= q.StartFrame || result.Resumed.State != "playing" || result.Completed.CompletedLoops != 1 || len(result.Events) != 1 {
		return result, errors.New("shared transport conformance differs")
	}
	return result, nil
}
