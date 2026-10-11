package hosts

import (
	"context"
	"errors"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/ladder"
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

type LadderReport struct {
	Events []engine.TransportEvent  `json:"events"`
	Final  engine.TransportSnapshot `json:"final"`
}

func ladderConformance(ctx context.Context, q engine.RenderRequest, frames int64, rate int) (LadderReport, error) {
	var result LadderReport
	device := &transportOutput{}
	tr, err := engine.NewTransport(device)
	if err != nil {
		return result, err
	}
	err = tr.Load(ctx, engine.TransportConfig{Source: q.Source, Frames: frames, SampleRate: rate, OutputRate: rate, Speed: q.Speed, Pitch: q.Pitch, LoopEnabled: true, LoopStart: q.StartFrame, LoopEnd: q.EndFrame})
	if err != nil {
		return result, err
	}
	definition := ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}
	_, err = tr.Dispatch(ctx, engine.TransportCommand{Version: 1, Sequence: 1, Type: "set_plan", Plan: &definition})
	if err != nil {
		return result, err
	}
	_, err = tr.Dispatch(ctx, engine.TransportCommand{Version: 1, Sequence: 2, Type: "play"})
	if err != nil {
		return result, err
	}
	compiled, _ := ladder.Compile(definition)
	for i := 0; i < 36; i++ {
		if len(device.schedules) <= i {
			return result, errors.New("ladder schedule missing")
		}
		s := device.schedules[i]
		want, _ := compiled.SpeedForLoop(uint64(i + 1))
		if s.Speed != want || s.Pitch != q.Pitch {
			return result, errors.New("ladder boundary parameters differ")
		}
		device.acks = []engine.OutputAcknowledgement{{Generation: s.Generation, Sequence: s.Sequence, ConsumedOutputFrame: s.OutputEnd}, {Generation: s.Generation, Sequence: s.Sequence, ConsumedOutputFrame: s.OutputEnd}}
		result.Final, err = tr.Poll(ctx)
		if err != nil {
			return result, err
		}
		result.Events = append(result.Events, tr.DrainEvents()...)
	}
	if result.Final.State != "stopped" || result.Final.Plan == nil || result.Final.Plan.CompletedLoops != 36 || len(result.Events) != 36 || len(device.schedules) != 36 {
		return result, errors.New("ladder completion trace differs")
	}
	return result, nil
}
