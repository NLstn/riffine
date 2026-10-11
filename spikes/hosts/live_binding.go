package hosts

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/NLstn/riffine/adapters/binding"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
)

// Synthetic presentation is deliberately confined to the feasibility harness.
// Production factories must attest actual device presentation, not submission.
type bindingDevice struct {
	transportOutput
	source    practice.VerifiedSource
	rate      int
	failStart bool
	closed    bool
}

func (d *bindingDevice) OutputSampleRate() int { return d.rate }
func (d *bindingDevice) Start(context.Context) error {
	if d.failStart {
		return errors.New("synthetic device start failure")
	}
	return nil
}
func (d *bindingDevice) Close() error { d.closed = true; return nil }
func (d *bindingDevice) OpenLive(_ context.Context, source practice.VerifiedSource) (practice.LiveDevice, error) {
	d.source = source
	d.closed = false
	return d, nil
}

func liveBindingConformance(ctx context.Context, host *memory, f fixture) ([]binding.Response, error) {
	device := &bindingDevice{rate: f.Audio.SampleRate * 2}
	bridge := binding.New(practice.Service{Resources: fileMemory{host}, Store: host, Decoder: audio.WAVDecoder{}, LiveDevice: device})
	defer bridge.Close()
	var trace []binding.Response
	call := func(request binding.Request) binding.Response {
		data, err := json.Marshal(request)
		if err != nil {
			panic(err)
		}
		response := bridge.Handle(ctx, data)
		trace = append(trace, response)
		return response
	}
	command := func(sequence uint64, kind string, speed *float64, loop *project.Loop) binding.Response {
		data, _ := json.Marshal(practice.Command{Version: 1, Sequence: sequence, Type: kind, Speed: speed, Loop: loop})
		return call(binding.Request{Version: 1, Type: "command", Command: data})
	}
	require := func(response binding.Response) error {
		if response.Error != nil {
			return response.Error
		}
		if response.Snapshot == nil {
			return errors.New("binding omitted snapshot")
		}
		return nil
	}
	if err := require(call(binding.Request{Version: 1, Type: "import", Source: "source", Document: "project"})); err != nil {
		return trace, err
	}
	loop := project.Loop{Enabled: true, Start: float64(f.Render.StartFrame) / float64(f.Audio.SampleRate), End: float64(f.Render.EndFrame) / float64(f.Audio.SampleRate)}
	if err := require(command(1, "set_loop", nil, &loop)); err != nil {
		return trace, err
	}
	attached := call(binding.Request{Version: 1, Type: "attach_transport"})
	if err := require(attached); err != nil {
		return trace, err
	}
	if attached.Snapshot.Sequence != 2 || !attached.Snapshot.Capabilities.LiveTransport {
		return trace, errors.New("attachment sequence/capability differs")
	}
	if err := require(command(3, "play", nil, nil)); err != nil {
		return trace, err
	}
	unacknowledged := call(binding.Request{Version: 1, Type: "poll_transport"})
	if err := require(unacknowledged); err != nil {
		return trace, err
	}
	if unacknowledged.Snapshot.Transport.CompletedLoops != 0 {
		return trace, errors.New("unacknowledged binding counted playback")
	}
	first := device.schedules[0]
	ack := engine.OutputAcknowledgement{Generation: first.Generation, Sequence: first.Sequence, ConsumedOutputFrame: first.OutputEnd}
	device.acks = []engine.OutputAcknowledgement{ack, ack}
	completed := call(binding.Request{Version: 1, Type: "poll_transport"})
	if err := require(completed); err != nil {
		return trace, err
	}
	if completed.Snapshot.Transport.CompletedLoops != 1 || completed.Snapshot.Sequence != 3 {
		return trace, errors.New("binding completion differs")
	}
	device.failStart = true
	speed := .75
	failed := command(4, "set_parameters", &speed, nil)
	if failed.Error == nil || failed.Error.Code != practice.HostFailure || failed.Snapshot.Sequence != 3 || failed.Snapshot.State != "error" || failed.Snapshot.Project.Settings.Speed != 1 {
		return trace, errors.New("binding committed failed playback edit")
	}
	device.failStart = false
	recovered := command(4, "recover", nil, nil)
	if err := require(recovered); err != nil {
		return trace, err
	}
	if recovered.Snapshot.Transport.Config.Speed != 1 || recovered.Snapshot.State != "stopped" {
		return trace, errors.New("binding recovery retained uncommitted edit")
	}
	if err := require(command(5, "play", nil, nil)); err != nil {
		return trace, err
	}
	if err := require(command(6, "pause", nil, nil)); err != nil {
		return trace, err
	}
	if err := require(call(binding.Request{Version: 1, Type: "events"})); err != nil {
		return trace, err
	}
	exported := call(binding.Request{Version: 1, Type: "export"})
	if err := require(exported); err != nil {
		return trace, err
	}
	if exported.Document == "" {
		return trace, errors.New("binding export is missing")
	}
	detached := call(binding.Request{Version: 1, Type: "detach_transport"})
	if err := require(detached); err != nil {
		return trace, err
	}
	if detached.Snapshot.Sequence != 7 || detached.Snapshot.Transport != nil || !device.closed {
		return trace, errors.New("binding detach retained transport")
	}
	if err := require(call(binding.Request{Version: 1, Type: "attach_transport"})); err != nil {
		return trace, err
	}
	if err := bridge.Close(); err != nil {
		return trace, err
	}
	if !device.closed {
		return trace, errors.New("bridge teardown leaked live device")
	}
	return trace, nil
}
