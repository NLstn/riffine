package practice

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
)

// This is a control-port fake, not audio-device or listening evidence.
type sessionDevice struct {
	rate                            int
	source                          VerifiedSource
	schedules                       []engine.Schedule
	pending                         []engine.OutputAcknowledgement
	generation                      uint64
	running                         bool
	closed                          int
	failStart, failReset, failClose bool
	closeReaderCount                int
	host                            *memoryHost
}

func (d *sessionDevice) OutputSampleRate() int { return d.rate }
func (d *sessionDevice) Reset(ctx context.Context, generation uint64) error {
	d.running = false
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.failReset {
		return errors.New("reset failed")
	}
	d.generation = generation
	d.schedules, d.pending = nil, nil
	return nil
}
func (d *sessionDevice) Schedule(ctx context.Context, schedule engine.Schedule) error {
	if err := ctx.Err(); err != nil {
		d.running = false
		return err
	}
	d.schedules = append(d.schedules, schedule)
	return nil
}
func (d *sessionDevice) Start(ctx context.Context) error {
	d.running = false
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.failStart {
		return errors.New("start failed")
	}
	d.running = true
	return nil
}
func (d *sessionDevice) DrainAcknowledgements(ctx context.Context, limit int) ([]engine.OutputAcknowledgement, error) {
	if err := ctx.Err(); err != nil {
		d.running = false
		return nil, err
	}
	if len(d.pending) > limit {
		d.running = false
		return nil, errors.New("ack overflow")
	}
	result := append([]engine.OutputAcknowledgement(nil), d.pending...)
	d.pending = nil
	return result, nil
}
func (d *sessionDevice) Suspend(ctx context.Context) ([]engine.OutputAcknowledgement, error) {
	d.running = false
	return d.DrainAcknowledgements(ctx, 128)
}
func (d *sessionDevice) Close() error {
	d.closed++
	d.running = false
	d.closeReaderCount = d.host.closed
	var sample [1]byte
	if _, err := d.source.Reader.ReadAt(sample[:], 0); err != nil {
		return err
	}
	if d.failClose {
		return errors.New("close failed after quiescence")
	}
	return nil
}

type sessionFactory struct {
	device *sessionDevice
	err    error
	opens  int
}

func (f *sessionFactory) OpenLive(ctx context.Context, source VerifiedSource) (LiveDevice, error) {
	f.opens++
	if f.device != nil {
		f.device.source = source
	}
	return f.device, f.err
}
func liveSession(t *testing.T) (*Session, *memoryHost, *sessionDevice, *sessionFactory) {
	t.Helper()
	session, host, _ := newSession(t)
	device := &sessionDevice{rate: 96000, host: host}
	factory := &sessionFactory{device: device}
	session.service.LiveDevice = factory
	t.Cleanup(func() { session.DetachTransport() })
	return session, host, device, factory
}
func dispatchLive(t *testing.T, session *Session, command Command) Snapshot {
	t.Helper()
	command.Version, command.Sequence = APIVersion, session.Snapshot().Sequence+1
	snapshot, err := session.Dispatch(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
func TestLiveSessionCommandsAndAcknowledgedSourceTime(t *testing.T) {
	session, host, device, factory := liveSession(t)
	if session.Snapshot().Capabilities.LiveTransport {
		t.Fatal("factory alone advertised playback")
	}
	loop := project.Loop{Enabled: true, Start: .25, End: .75}
	dispatchLive(t, session, Command{Type: "set_loop", Loop: &loop})
	if err := session.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot := session.Snapshot()
	if snapshot.Sequence != 2 || !snapshot.Capabilities.LiveTransport || snapshot.Transport.Config.LoopStart != 12000 || snapshot.Transport.Config.LoopEnd != 36000 || factory.opens != 1 || host.opened-host.closed != 1 {
		t.Fatalf("attachment: %+v, resources %d/%d", snapshot, host.opened, host.closed)
	}
	if device.source.Info.SHA256 != snapshot.Project.Track.Audio.SHA256 || device.source.Info.DataOffset == 0 {
		t.Fatal("factory did not receive inspected metadata")
	}
	dispatchLive(t, session, Command{Type: "set_annotations", Annotations: &project.Annotations{}})
	snapshot = dispatchLive(t, session, Command{Type: "play"})
	if snapshot.Sequence != 4 || snapshot.Transport.Sequence != 1 || !device.running || len(device.schedules) != 2 {
		t.Fatalf("play: %+v", snapshot)
	}
	// UI polling without device acknowledgements cannot count a loop or move time.
	for range 3 {
		polled, err := session.PollTransport(context.Background())
		if err != nil || polled.Transport.SourceFrame != 12000 || polled.Transport.CompletedLoops != 0 || polled.Sequence != 4 {
			t.Fatalf("poll: %+v %v", polled, err)
		}
	}
	first := device.schedules[0]
	device.pending = []engine.OutputAcknowledgement{{Generation: first.Generation, Sequence: first.Sequence, ConsumedOutputFrame: first.OutputEnd}}
	snapshot, err := session.PollTransport(context.Background())
	if err != nil || snapshot.Transport.CompletedLoops != 1 || snapshot.Transport.ConsumedOutputFrame != 48000 || factory.opens != 1 || host.opened-host.closed != 1 {
		t.Fatalf("completion: %+v %v", snapshot, err)
	}
	batch := session.DrainEvents()
	found := false
	for _, event := range batch.Events {
		if event.Transport != nil && event.Transport.CompletedLoops == 1 {
			found = true
			if event.Sequence != 4 || event.Transport.Generation != first.Generation {
				t.Fatalf("event: %+v", event)
			}
		}
	}
	if !found {
		t.Fatalf("missing completion event: %+v", batch)
	}
	position := int64(24000)
	snapshot = dispatchLive(t, session, Command{Type: "seek", Position: &position})
	if snapshot.Transport.SourceFrame != position || snapshot.Transport.CompletedLoops != 0 {
		t.Fatalf("seek: %+v", snapshot)
	}
	oldGeneration := first.Generation
	device.pending = []engine.OutputAcknowledgement{{Generation: oldGeneration, Sequence: first.Sequence, ConsumedOutputFrame: first.OutputEnd}}
	snapshot, err = session.PollTransport(context.Background())
	if err != nil || snapshot.Transport.CompletedLoops != 0 {
		t.Fatalf("stale ack: %+v %v", snapshot, err)
	}
	snapshot = dispatchLive(t, session, Command{Type: "pause"})
	if snapshot.State != "paused" || device.running {
		t.Fatal("pause did not suspend")
	}
	snapshot = dispatchLive(t, session, Command{Type: "stop"})
	if snapshot.State != "stopped" || snapshot.Transport.SourceFrame != 12000 {
		t.Fatal("stop did not restore source selection")
	}
	if err := session.DetachTransport(); err != nil {
		t.Fatal(err)
	}
	if session.Snapshot().Capabilities.LiveTransport || session.Snapshot().Transport != nil || device.closed != 1 || host.opened != host.closed || device.closeReaderCount != host.closed-1 {
		t.Fatal("detach ownership order")
	}
	if err := session.DetachTransport(); err != nil || device.closed != 1 {
		t.Fatal("detach is not idempotent")
	}
}
func TestLiveSessionFailureDoesNotCommitEditsAndRecoveryRestoresProject(t *testing.T) {
	session, _, device, _ := liveSession(t)
	if err := session.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	dispatchLive(t, session, Command{Type: "play"})
	before := session.Snapshot()
	speed := .75
	device.failStart = true
	failed, err := session.Dispatch(context.Background(), Command{Version: 1, Sequence: before.Sequence + 1, Type: "set_parameters", Speed: &speed})
	errorCode(t, err, HostFailure)
	if failed.Sequence != before.Sequence || failed.Project.Settings != before.Project.Settings || failed.State != "error" || failed.Transport.Config.Speed != .75 || device.running {
		t.Fatalf("failure: %+v", failed)
	}
	// Edits unrelated to playback remain available while the device is suspended.
	dispatchLive(t, session, Command{Type: "set_annotations", Annotations: &project.Annotations{}})
	device.failStart = false
	recovered := dispatchLive(t, session, Command{Type: "recover"})
	if recovered.State != "stopped" || recovered.Transport.Config.Speed != before.Project.Settings.Speed || recovered.Transport.CompletedLoops != 0 || device.running {
		t.Fatalf("recovery: %+v", recovered)
	}
	dispatchLive(t, session, Command{Type: "play"})
	if device.schedules[0].Speed != before.Project.Settings.Speed {
		t.Fatal("recovery played uncommitted settings")
	}
}
func TestLiveSessionRejectsBeforeDeviceEffects(t *testing.T) {
	session, _, device, _ := liveSession(t)
	if err := session.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	before := session.Snapshot()
	badSpeed, position := math.NaN(), int64(48000)
	tests := []Command{
		{Type: "set_parameters", Speed: &badSpeed}, {Type: "play", Position: &position},
		{Type: "seek", Position: &position}, {Type: "seek"}, {Type: "recover"},
		{Type: "set_loop", Loop: &project.Loop{Enabled: true, Start: .9, End: .1}},
	}
	for _, command := range tests {
		command.Version, command.Sequence = 1, before.Sequence+1
		got, err := session.Dispatch(context.Background(), command)
		errorCode(t, err, InvalidCommand)
		if !reflect.DeepEqual(got, before) || len(device.schedules) != 0 {
			t.Fatalf("rejected command changed state: %+v", command)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := session.Dispatch(ctx, Command{Version: 1, Sequence: before.Sequence + 1, Type: "play"})
	errorCode(t, err, Cancelled)
	if !reflect.DeepEqual(session.Snapshot(), before) {
		t.Fatal("cancelled command changed state")
	}
}
func TestLiveAttachmentFailuresReleaseOwnedPorts(t *testing.T) {
	for _, mode := range []string{"factory", "rate", "reset", "identity", "close"} {
		t.Run(mode, func(t *testing.T) {
			session, host, device, factory := liveSession(t)
			before := session.Snapshot()
			switch mode {
			case "factory":
				factory.err = errors.New("open failed")
			case "rate":
				device.rate = 0
			case "reset":
				device.failReset = true
			case "identity":
				host.data["opaque:original"][50] ^= 1
			case "close":
				device.failClose = true
			}
			err := session.AttachTransport(context.Background())
			if mode == "close" {
				if err != nil {
					t.Fatal(err)
				}
				err = session.DetachTransport()
				errorCode(t, err, HostFailure)
				if session.Snapshot().Capabilities.LiveTransport || device.running {
					t.Fatal("failed close retained capability")
				}
			} else {
				if err == nil {
					t.Fatal("attachment unexpectedly succeeded")
				}
				if !reflect.DeepEqual(session.Snapshot(), before) {
					t.Fatal("failed attachment changed session")
				}
			}
			if host.opened != host.closed {
				t.Fatalf("reader leak %d/%d", host.opened, host.closed)
			}
			if mode == "identity" {
				if factory.opens != 0 || device.closed != 0 {
					t.Fatal("opened unverified source")
				}
			} else if device.closed != 1 || device.closeReaderCount != host.closed-1 {
				t.Fatal("device was not closed before reader")
			}
		})
	}
}
func TestRelinkEndsOldTransportAfterVerifyingReplacement(t *testing.T) {
	session, host, device, factory := liveSession(t)
	if err := session.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	dispatchLive(t, session, Command{Type: "play"})
	host.data["replacement"] = append([]byte(nil), host.data["opaque:original"]...)
	host.data["wrong"] = []byte("not audio")
	if err := session.Relink(context.Background(), "wrong"); err == nil || device.closed != 0 || !device.running {
		t.Fatal("bad replacement disturbed playback")
	}
	before := session.Snapshot().Sequence
	if err := session.Relink(context.Background(), "replacement"); err != nil {
		t.Fatal(err)
	}
	if session.Snapshot().Sequence != before+1 || session.Snapshot().Capabilities.LiveTransport || device.closed != 1 || host.opened != host.closed {
		t.Fatal("relink did not release old live resource")
	}
	device.failClose = false
	if err := session.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	if factory.opens != 2 || device.source.Handle != "replacement" {
		t.Fatal("reattachment used old resource")
	}
}

func TestUnconditionalLiveCleanupAtSequenceExhaustion(t *testing.T) {
	session, host, device, _ := liveSession(t)
	if err := session.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	session.sequence = math.MaxUint64
	if err := session.CloseTransport(); err != nil {
		t.Fatal(err)
	}
	if session.Snapshot().Sequence != math.MaxUint64 || device.closed != 1 || host.opened != host.closed {
		t.Fatal("unconditional cleanup failed")
	}
}
func TestMissingLiveFactoryAndCancelledAttachmentDoNotOpenResources(t *testing.T) {
	session, host, _ := newSession(t)
	before := session.Snapshot()
	errorCode(t, session.AttachTransport(context.Background()), UnsupportedCapability)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opened := host.opened
	errorCode(t, session.AttachTransport(ctx), Cancelled)
	if host.opened != opened || !reflect.DeepEqual(session.Snapshot(), before) {
		t.Fatal("rejected attachment disturbed session")
	}
	_, err := session.Dispatch(context.Background(), Command{Version: 1, Sequence: 1, Type: "seek", Position: new(int64)})
	errorCode(t, err, UnsupportedCapability)
}
