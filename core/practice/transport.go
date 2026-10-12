package practice

import (
	"context"
	"errors"
	"math"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
)

// VerifiedSource borrows Reader until LiveDevice.Close completes. Handle is an
// opaque host locator; Info describes the verified original bytes, including
// decoder-specific offsets. A factory must not reopen a mutable path instead of
// consuming the verified reader, or retain the request context as its lifetime.
type VerifiedSource struct {
	Handle string
	Reader resource.Reader
	Info   audio.Info
}

// LiveDevice is a presentation-aware control-worker port. Close must quiesce
// output and release every borrowed resource before returning, even on error.
// No device operation may re-enter Session. Audio callbacks never call Go core.
type LiveDevice interface {
	engine.TransportDevice
	OutputSampleRate() int
	Close() error
}

type LiveDeviceFactory interface {
	OpenLive(context.Context, VerifiedSource) (LiveDevice, error)
}

type liveAttachment struct {
	transport *engine.Transport
	device    LiveDevice
	reader    resource.Reader
	// A failed adapter operation may have installed settings before failing.
	// Recovery reloads committed project settings rather than trusting that state.
	dirty      bool
	activePlan *project.PracticePlan
}

func (s *Session) transportConfig(p project.Project, outputRate int) engine.TransportConfig {
	rate := float64(p.Track.Audio.SampleRate)
	return engine.TransportConfig{Source: s.source, Frames: p.Track.Audio.Frames,
		SampleRate: p.Track.Audio.SampleRate, OutputRate: outputRate, Speed: p.Settings.Speed,
		Pitch: p.Settings.Pitch, LoopEnabled: p.Settings.Loop.Enabled,
		LoopStart: int64(math.Round(p.Settings.Loop.Start * rate)), LoopEnd: int64(math.Round(p.Settings.Loop.End * rate))}
}

// AttachTransport opens and verifies original bytes once and transfers device
// ownership to the session. Only a successful attachment advertises live support.
// This ordered operation consumes one session sequence; factory/load failures
// release the device before its reader and leave the edit session unchanged.
func (s *Session) AttachTransport(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return failure(Cancelled, err)
	}
	if s.live != nil {
		return failure(InvalidCommand, errors.New("transport is already attached"))
	}
	if s.service.LiveDevice == nil {
		return failure(UnsupportedCapability, errors.New("live transport is unavailable"))
	}
	if s.sequence == math.MaxUint64 {
		return failure(OutOfOrder, errors.New("session sequence exhausted"))
	}
	reader, info, err := s.openVerifiedAudio(ctx, s.source)
	if err != nil {
		return err
	}
	device, err := s.service.LiveDevice.OpenLive(ctx, VerifiedSource{Handle: s.source, Reader: reader, Info: info})
	cleanup := func(cause error) error {
		if device != nil {
			cause = errors.Join(cause, device.Close())
		}
		cause = errors.Join(cause, reader.Close())
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
			return failure(Cancelled, cause)
		}
		return failure(HostFailure, cause)
	}
	if err != nil {
		return cleanup(err)
	}
	if device == nil {
		return cleanup(errors.New("live factory returned no device"))
	}
	transport, err := engine.NewTransport(device)
	if err != nil {
		return cleanup(err)
	}
	if err = transport.Load(ctx, s.transportConfig(s.p, device.OutputSampleRate())); err != nil {
		return cleanup(err)
	}
	if err = ctx.Err(); err != nil {
		return cleanup(err)
	}
	s.live = &liveAttachment{transport: transport, device: device, reader: reader}
	s.sequence++
	s.emit("transport_attached")
	return nil
}

// DetachTransport ends the live runtime, leaving persisted project settings
// intact. It is idempotent. Close errors still clear the capability because the
// device contract guarantees quiescence; such failures do not consume a command
// sequence. No pending playback counter is persisted as practice activity.
func (s *Session) DetachTransport() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live == nil {
		return nil
	}
	if s.sequence == math.MaxUint64 {
		return failure(OutOfOrder, errors.New("session sequence exhausted"))
	}
	if err := s.detachTransport(); err != nil {
		return err
	}
	s.sequence++
	s.emit("transport_detached")
	return nil
}

// CloseTransport releases the attachment without consuming a command sequence.
// Hosts use it for unconditional session teardown, even at sequence exhaustion.
func (s *Session) CloseTransport() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detachTransport()
}
func (s *Session) detachTransport() error {
	if s.live == nil {
		return nil
	}
	attachment := s.live
	s.live = nil
	deviceErr := attachment.device.Close()
	readerErr := attachment.reader.Close()
	if err := errors.Join(deviceErr, readerErr); err != nil {
		return failure(HostFailure, err)
	}
	return nil
}

func (s *Session) collectTransportEvents() {
	if s.live == nil {
		return
	}
	for _, event := range s.live.transport.DrainEvents() {
		value := event
		s.emitEvent(Event{Version: APIVersion, Sequence: s.sequence, Type: event.Type, Transport: &value})
	}
}

// PollTransport advances only from device presentation acknowledgements. Hosts
// pump this bounded operation on a control worker; UI timestamps cannot advance
// source position or completed-loop counts. Poll never consumes an edit sequence.
func (s *Session) PollTransport(ctx context.Context) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return s.snapshot(), failure(Cancelled, err)
	}
	if s.live == nil {
		return s.snapshot(), failure(UnsupportedCapability, errors.New("live transport is unavailable"))
	}
	_, err := s.live.transport.Poll(ctx)
	s.collectTransportEvents()
	if err != nil {
		s.live.dirty = s.live.transport.Snapshot().State == "error"
		return s.snapshot(), failure(HostFailure, err)
	}
	return s.snapshot(), nil
}

func (s *Session) dispatchTransport(ctx context.Context, command Command, p project.Project) error {
	attachment := s.live
	if command.Type == "select_practice_plan" || command.Type == "resume_practice_plan" || command.Type == "clear_practice_plan" {
		return s.dispatchPracticePlan(ctx, command, p)
	}
	before := attachment.transport.Snapshot()
	if command.Type == "recover" {
		if before.State != "error" && !attachment.dirty {
			return failure(InvalidCommand, errors.New("recovery requires an error"))
		}
		// Load discards runtime plans/counts and restores only committed settings.
		if err := attachment.transport.Load(ctx, s.transportConfig(p, attachment.device.OutputSampleRate())); err != nil {
			attachment.dirty = true
			return failure(HostFailure, err)
		}
		attachment.activePlan = nil
		before = attachment.transport.Snapshot()
	} else if before.State == "error" || attachment.dirty {
		return failure(HostFailure, errors.New("transport requires recovery"))
	}
	transportCommand := engine.TransportCommand{Version: engine.TransportVersion,
		Sequence: before.Sequence + 1, Type: command.Type, Position: command.Position,
		Speed: command.Speed, Pitch: command.Pitch}
	if command.Type == "recover" {
		transportCommand.Type = "stop"
	}
	if command.Loop != nil {
		config := s.transportConfig(p, before.Config.OutputRate)
		transportCommand.Loop = &engine.TransportLoop{Enabled: config.LoopEnabled, StartFrame: config.LoopStart, EndFrame: config.LoopEnd}
	}
	after, err := attachment.transport.Dispatch(ctx, transportCommand)
	if err != nil {
		attachment.dirty = after.State == "error" || command.Type == "recover"
		s.collectTransportEvents()
		if attachment.dirty {
			return failure(HostFailure, err)
		}
		return failure(InvalidCommand, err)
	}
	attachment.dirty = false
	return nil
}
