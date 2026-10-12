// Package practice owns host-neutral session operations and product validation.
package practice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
	"github.com/NLstn/riffine/core/waveform"
)

const APIVersion = 1
const EventCapacity = 32

type ErrorCode string

const (
	InvalidCommand        ErrorCode = "invalid_command"
	UnsupportedVersion    ErrorCode = "unsupported_version"
	InvalidProject        ErrorCode = "invalid_project"
	ResourceUnavailable   ErrorCode = "resource_unavailable"
	IdentityMismatch      ErrorCode = "identity_mismatch"
	UnsupportedCapability ErrorCode = "unsupported_capability"
	OutOfOrder            ErrorCode = "out_of_order"
	Cancelled             ErrorCode = "cancelled"
	HostFailure           ErrorCode = "host_failure"
	RevisionConflict      ErrorCode = "revision_conflict"
	DocumentBusy          ErrorCode = "document_busy"
)

type Error struct {
	Version int       `json:"version"`
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	cause   error
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }
func (e *Error) Unwrap() error { return e.cause }
func failure(code ErrorCode, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = Cancelled
	}
	return &Error{Version: APIVersion, Code: code, Message: err.Error(), cause: err}
}

type Capabilities struct {
	Formats       []string `json:"formats"`
	OfflineRender bool     `json:"offlineRender"`
	LiveTransport bool     `json:"liveTransport"`
	StreamPreview bool     `json:"streamPreview,omitempty"`
}

// Command sequences start at 1, are contiguous and apply only on success.
// Settings use speed ratios and pitch semitones; loop bounds use source seconds.
type Command struct {
	Version       int                     `json:"version"`
	Sequence      uint64                  `json:"sequence"`
	Type          string                  `json:"type"`
	Speed         *float64                `json:"speed,omitempty"`
	Pitch         *float64                `json:"pitch,omitempty"`
	Loop          *project.Loop           `json:"loop,omitempty"`
	Annotations   *project.Annotations    `json:"annotations,omitempty"`
	PracticePlans *[]project.PracticePlan `json:"practicePlans,omitempty"`
	PlanID        *string                 `json:"planId,omitempty"`
	Position      *int64                  `json:"position,omitempty"`
}
type Snapshot struct {
	Version            int                       `json:"version"`
	Sequence           uint64                    `json:"sequence"`
	State              string                    `json:"state"`
	Project            project.Project           `json:"project"`
	Capabilities       Capabilities              `json:"capabilities"`
	Revision           string                    `json:"revision,omitempty"`
	Transport          *engine.TransportSnapshot `json:"transport,omitempty"`
	ActivePracticePlan *project.PracticePlan     `json:"activePracticePlan,omitempty"`
}
type Event struct {
	Version   int                    `json:"version"`
	Sequence  uint64                 `json:"sequence"`
	Type      string                 `json:"type"`
	Transport *engine.TransportEvent `json:"transport,omitempty"`
}
type EventBatch struct {
	Version  int      `json:"version"`
	Events   []Event  `json:"events"`
	Dropped  uint64   `json:"dropped"`
	Snapshot Snapshot `json:"snapshot"`
}

type Service struct {
	Resources       resource.Resources
	Store           resource.Store
	Decoder         audio.Decoder
	Renderer        engine.Renderer
	SourceRenderer  engine.SourceRenderer
	Previewer       engine.Previewer
	SourcePreviewer engine.SourcePreviewer
	LiveDevice      LiveDeviceFactory
}

func (s Service) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return failure(Cancelled, err)
	}
	if s.Resources == nil || s.Store == nil || s.Decoder == nil {
		return failure(HostFailure, errors.New("resource, storage and decoder ports are required"))
	}
	return nil
}
func (s Service) Import(ctx context.Context, source, document string) (*Session, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	r, err := s.Resources.Open(ctx, source)
	if err != nil {
		return nil, failure(ResourceUnavailable, err)
	}
	defer r.Close()
	info, err := s.Decoder.Inspect(ctx, r)
	if err != nil {
		return nil, failure(ResourceUnavailable, err)
	}
	reference, err := s.Resources.Reference(ctx, document, source)
	if err != nil {
		return nil, failure(ResourceUnavailable, err)
	}
	p, err := project.New(r.Name(), reference, info)
	if err != nil {
		return nil, failure(InvalidProject, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, failure(Cancelled, err)
	}
	return &Session{service: s, document: document, source: source, p: p}, nil
}
func (s Service) Open(ctx context.Context, document string) (*Session, error) {
	if err := s.check(ctx); err != nil {
		return nil, err
	}
	b, err := s.Store.Read(ctx, document, project.MaxDocumentBytes)
	if err != nil {
		return nil, failure(ResourceUnavailable, err)
	}
	p, err := project.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, failure(InvalidProject, err)
	}
	source, err := s.Resources.Resolve(ctx, document, p.Track.Path)
	if err != nil {
		return nil, failure(ResourceUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, failure(Cancelled, err)
	}
	return &Session{service: s, document: document, source: source, p: p, revision: resource.Revision(b)}, nil
}

// Session serializes control operations. No method belongs on an audio callback.
// A new Open/Import creates an independent sequence domain, initially ready/0.
// Long host operations hold the control lock; hosts cancel via the context and
// may run operations on a control worker to keep views responsive.
type Session struct {
	mu               sync.Mutex
	service          Service
	document, source string
	p                project.Project
	revision         string
	sequence         uint64
	events           [EventCapacity]Event
	head, count      int
	dropped          uint64
	live             *liveAttachment
}

func (s *Session) snapshot() Snapshot {
	snapshot := Snapshot{Version: APIVersion, Sequence: s.sequence, State: "ready", Revision: s.revision, Project: s.p.Clone(),
		Capabilities: Capabilities{Formats: append([]string{}, s.service.Decoder.Formats()...), OfflineRender: s.service.Renderer != nil || s.service.SourceRenderer != nil, StreamPreview: s.service.Previewer != nil || s.service.SourcePreviewer != nil}}
	if s.live != nil {
		transport := s.live.transport.Snapshot()
		if s.live.dirty {
			transport.State = "error"
			if transport.Error == "" {
				transport.Error = "transport requires recovery"
			}
		}
		snapshot.Transport = &transport
		snapshot.State = transport.State
		snapshot.Capabilities.LiveTransport = true
		if s.live.activePlan != nil {
			plan := *s.live.activePlan
			snapshot.ActivePracticePlan = &plan
		}
	}
	return snapshot
}
func (s *Session) Snapshot() Snapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.snapshot() }
func (s *Session) emit(kind string) {
	s.emitEvent(Event{Version: APIVersion, Sequence: s.sequence, Type: kind})
}
func (s *Session) emitEvent(e Event) {
	if s.count == EventCapacity {
		s.events[s.head] = e
		s.head = (s.head + 1) % EventCapacity
		s.dropped++
	} else {
		s.events[(s.head+s.count)%EventCapacity] = e
		s.count++
	}
}
func (s *Session) DrainEvents() EventBatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := EventBatch{Version: APIVersion, Events: make([]Event, s.count), Dropped: s.dropped, Snapshot: s.snapshot()}
	for i := range b.Events {
		b.Events[i] = s.events[(s.head+i)%EventCapacity]
	}
	s.head, s.count, s.dropped = 0, 0, 0
	return b
}
func (s *Session) Dispatch(ctx context.Context, c Command) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reject := func(code ErrorCode, message string) (Snapshot, error) {
		return s.snapshot(), failure(code, errors.New(message))
	}
	if err := ctx.Err(); err != nil {
		return s.snapshot(), failure(Cancelled, err)
	}
	if c.Version != APIVersion {
		return reject(UnsupportedVersion, "unsupported command version")
	}
	if c.Sequence == 0 || s.sequence == math.MaxUint64 || c.Sequence != s.sequence+1 {
		return reject(OutOfOrder, "command sequence must be the next successful sequence")
	}
	p := s.p
	if practiceAction(c.Type) {
		if c.PracticePlans != nil || c.Annotations != nil || c.Loop != nil || c.Speed != nil || c.Pitch != nil || c.Position != nil || c.PlanID != nil {
			return reject(InvalidCommand, "practice action accepts no extra fields")
		}
		if s.live == nil {
			return reject(UnsupportedCapability, "live transport is unavailable")
		}
		updated, err := s.dispatchPracticeAction(ctx, c.Type, p)
		if err != nil {
			return s.snapshot(), err
		}
		s.p, s.sequence = updated, c.Sequence
		s.collectTransportEvents()
		if c.Type == "rewind_phrase" || c.Type == "seek_backward" || c.Type == "seek_forward" {
			s.emit("transport_changed")
		} else {
			s.emit("session_changed")
		}
		return s.snapshot(), nil
	}
	if c.Position != nil && c.Type != "seek" {
		return reject(InvalidCommand, "position is only valid for seek")
	}
	if c.PlanID != nil && c.Type != "select_practice_plan" {
		return reject(InvalidCommand, "planId is only valid for select_practice_plan")
	}
	switch c.Type {
	case "select_practice_plan", "resume_practice_plan", "clear_practice_plan":
		if c.PracticePlans != nil || c.Annotations != nil || c.Loop != nil || c.Speed != nil || c.Pitch != nil || c.Position != nil || (c.Type == "select_practice_plan") != (c.PlanID != nil) {
			return reject(InvalidCommand, "practice plan command has unexpected or missing fields")
		}
		if s.live == nil {
			return reject(UnsupportedCapability, "live transport is unavailable")
		}
	case "set_parameters":
		if c.PracticePlans != nil || c.Annotations != nil || c.Loop != nil || (c.Speed == nil && c.Pitch == nil) {
			return reject(InvalidCommand, "set_parameters requires speed and/or pitch only")
		}
		if c.Speed != nil {
			p.Settings.Speed = *c.Speed
		}
		if c.Pitch != nil {
			p.Settings.Pitch = *c.Pitch
		}
	case "set_loop":
		if c.PracticePlans != nil || c.Annotations != nil || c.Loop == nil || c.Speed != nil || c.Pitch != nil {
			return reject(InvalidCommand, "set_loop requires loop only")
		}
		p.Settings.Loop = *c.Loop
	case "set_annotations":
		if c.PracticePlans != nil || c.Annotations == nil || c.Loop != nil || c.Speed != nil || c.Pitch != nil {
			return reject(InvalidCommand, "set_annotations requires annotations only")
		}
		p.Annotations = c.Annotations.Clone()
	case "set_practice_plans":
		if c.PracticePlans == nil || c.Annotations != nil || c.Loop != nil || c.Speed != nil || c.Pitch != nil {
			return reject(InvalidCommand, "set_practice_plans requires practicePlans only")
		}
		p.PracticePlans = project.ClonePlans(*c.PracticePlans)
	case "play", "pause", "stop", "restart", "recover", "seek":
		if c.PracticePlans != nil || c.Annotations != nil || c.Loop != nil || c.Speed != nil || c.Pitch != nil || (c.Type == "seek") != (c.Position != nil) {
			return reject(InvalidCommand, "transport command has unexpected or missing fields")
		}
		if s.live == nil {
			return reject(UnsupportedCapability, "live transport is unavailable")
		}
	default:
		return reject(UnsupportedCapability, fmt.Sprintf("unsupported command %q", c.Type))
	}
	if _, err := project.Encode(p); err != nil {
		return s.snapshot(), failure(InvalidCommand, err)
	}
	if err := ctx.Err(); err != nil {
		return s.snapshot(), failure(Cancelled, err)
	}
	if s.live != nil && (c.Type == "set_parameters" || c.Type == "set_loop" || c.Type == "play" || c.Type == "pause" || c.Type == "stop" || c.Type == "restart" || c.Type == "recover" || c.Type == "seek" || c.Type == "select_practice_plan" || c.Type == "resume_practice_plan" || c.Type == "clear_practice_plan") {
		if err := s.dispatchTransport(ctx, c, p); err != nil {
			return s.snapshot(), err
		}
	}
	s.p, s.sequence = p, c.Sequence
	s.collectTransportEvents()
	if c.Type == "set_parameters" || c.Type == "set_loop" || c.Type == "set_annotations" || c.Type == "set_practice_plans" {
		s.emit("session_changed")
	} else {
		s.emit("transport_changed")
	}
	return s.snapshot(), nil
}
func (s *Session) SaveAs(ctx context.Context, destination string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return failure(Cancelled, err)
	}
	p := s.p
	reference, err := s.service.Resources.Reference(ctx, destination, s.source)
	if err != nil {
		return failure(ResourceUnavailable, err)
	}
	p.Track.Path = reference
	b, err := project.Encode(p)
	if err != nil {
		return failure(InvalidProject, err)
	}
	if err = s.service.Store.Create(ctx, destination, b); err != nil {
		return failure(HostFailure, err)
	}
	return nil
}
func (s *Session) openVerified(ctx context.Context, source string) (resource.Reader, error) {
	r, _, err := s.openVerifiedAudio(ctx, source)
	return r, err
}
func (s *Session) openVerifiedAudio(ctx context.Context, source string) (resource.Reader, audio.Info, error) {
	if err := ctx.Err(); err != nil {
		return nil, audio.Info{}, failure(Cancelled, err)
	}
	r, err := s.service.Resources.Open(ctx, source)
	if err != nil {
		return nil, audio.Info{}, failure(ResourceUnavailable, err)
	}
	info, err := s.service.Decoder.Inspect(ctx, r)
	if err != nil {
		r.Close()
		return nil, audio.Info{}, failure(ResourceUnavailable, err)
	}
	if err = s.p.VerifyIdentity(info); err != nil {
		r.Close()
		return nil, audio.Info{}, failure(IdentityMismatch, err)
	}
	return r, info, nil
}
func (s *Session) verify(ctx context.Context, source string) error {
	r, err := s.openVerified(ctx, source)
	if err != nil {
		return err
	}
	if err := r.Close(); err != nil {
		return failure(HostFailure, err)
	}
	return nil
}
func (s *Session) Verify(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verify(ctx, s.source)
}

// Relink is an ordered control operation. Hosts read the resulting snapshot
// sequence before sending their next command. Only verified original bytes link.
func (s *Session) Relink(ctx context.Context, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.verify(ctx, source); err != nil {
		return err
	}
	reference, err := s.service.Resources.Reference(ctx, s.document, source)
	if err != nil {
		return failure(ResourceUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return failure(Cancelled, err)
	}
	if s.sequence == math.MaxUint64 {
		return failure(OutOfOrder, errors.New("session sequence exhausted"))
	}
	hadTransport := s.live != nil
	if err := s.detachTransport(); err != nil {
		return err
	}
	s.p.Track.Path, s.source = reference, source
	s.sequence++
	if hadTransport {
		s.emit("transport_detached")
	}
	s.emit("source_relinked")
	return nil
}
func (s *Session) Waveform(ctx context.Context, bins int) (audio.Waveform, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.openVerified(ctx, s.source)
	if err != nil {
		return audio.Waveform{}, err
	}
	defer r.Close()
	w, err := s.service.Decoder.Peaks(ctx, r, bins)
	if err != nil {
		return audio.Waveform{}, failure(HostFailure, err)
	}
	return w, nil
}
func (s *Session) Render(ctx context.Context, destination string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.service.Renderer == nil && s.service.SourceRenderer == nil {
		return failure(UnsupportedCapability, errors.New("offline rendering is unavailable"))
	}
	start, end := s.selection()
	request := engine.RenderRequest{Source: s.source, Destination: destination, StartFrame: start, EndFrame: end, Speed: s.p.Settings.Speed, Pitch: s.p.Settings.Pitch}
	var err error
	if s.service.SourceRenderer != nil {
		var reader resource.Reader
		var info audio.Info
		reader, info, err = s.openVerifiedAudio(ctx, s.source)
		if err != nil {
			return err
		}
		defer reader.Close()
		err = s.service.SourceRenderer.RenderSource(ctx, reader, info, request)
	} else {
		if err = s.verify(ctx, s.source); err != nil {
			return err
		}
		err = s.service.Renderer.Render(ctx, request)
	}
	if err != nil {
		return failure(HostFailure, err)
	}
	return nil
}

func (s *Session) selection() (int64, int64) {
	if s.p.Settings.Loop.Enabled {
		rate := float64(s.p.Track.Audio.SampleRate)
		return int64(math.Round(s.p.Settings.Loop.Start * rate)), int64(math.Round(s.p.Settings.Loop.End * rate))
	}
	return 0, s.p.Track.Audio.Frames
}

// Preview streams one verified selection at fixed settings. It neither repeats
// an enabled loop nor mutates state/sequences/events. Cancellation uses context.
// Live transport requires a separately attached presentation-aware device.
func (s *Session) Preview(ctx context.Context) (engine.PreviewReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.service.Previewer == nil && s.service.SourcePreviewer == nil {
		return engine.PreviewReport{}, failure(UnsupportedCapability, errors.New("streaming preview is unavailable"))
	}
	start, end := s.selection()
	request := engine.PreviewRequest{Source: s.source, StartFrame: start, EndFrame: end, Speed: s.p.Settings.Speed, Pitch: s.p.Settings.Pitch}
	var report engine.PreviewReport
	var err error
	if s.service.SourcePreviewer != nil {
		var reader resource.Reader
		var info audio.Info
		reader, info, err = s.openVerifiedAudio(ctx, s.source)
		if err != nil {
			return report, err
		}
		defer reader.Close()
		report, err = s.service.SourcePreviewer.PreviewSource(ctx, reader, info, request)
	} else {
		if err = s.verify(ctx, s.source); err != nil {
			return report, err
		}
		report, err = s.service.Previewer.Preview(ctx, request)
	}
	if err != nil {
		return engine.PreviewReport{}, failure(HostFailure, err)
	}
	if report.Version != 1 || report.Channels != s.p.Track.Audio.Channels || report.SampleRate != s.p.Track.Audio.SampleRate ||
		report.OutputFrames != int64(math.Round(float64(end-start)/s.p.Settings.Speed)) || report.CopiedFrames != report.OutputFrames {
		return engine.PreviewReport{}, failure(HostFailure, errors.New("preview adapter returned inconsistent source/output diagnostics"))
	}
	return report, nil
}

// Save updates the opened document using its exact byte revision. Import must
// use SaveAs and reopen first. Save does not change the edit sequence; cancellation
// racing an already committed update cannot undo publication.
func (s *Session) Save(ctx context.Context) (resource.UpdateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result resource.UpdateResult
	if err := ctx.Err(); err != nil {
		return result, failure(Cancelled, err)
	}
	store, ok := s.service.Store.(resource.UpdatingStore)
	if !ok || s.revision == "" {
		return result, failure(UnsupportedCapability, errors.New("revision-protected saving is unavailable; use SaveAs and reopen"))
	}
	data, err := project.Encode(s.p)
	if err != nil {
		return result, failure(InvalidProject, err)
	}
	result, err = store.Update(ctx, s.document, s.revision, data)
	if err != nil {
		code := HostFailure
		if errors.Is(err, resource.ErrRevisionConflict) {
			code = RevisionConflict
		}
		if errors.Is(err, resource.ErrDocumentBusy) {
			code = DocumentBusy
		}
		return result, failure(code, err)
	}
	if result.Revision != resource.Revision(data) {
		return result, failure(HostFailure, errors.New("host returned an invalid committed revision; reopen the document"))
	}
	s.revision = result.Revision
	return result, nil
}

// OpenWaveform verifies the source once and returns an immutable pyramid. Hosts
// keep that handle for window/zoom queries and reopen after a resource change.
func (s *Session) OpenWaveform(ctx context.Context, cache waveform.Cache, progress func(waveform.Progress)) (waveform.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decoder, ok := s.service.Decoder.(audio.PCMDecoder)
	if !ok {
		return waveform.Result{}, failure(UnsupportedCapability, errors.New("streaming waveform decoding is unavailable"))
	}
	r, err := s.openVerified(ctx, s.source)
	if err != nil {
		return waveform.Result{}, err
	}
	defer r.Close()
	result, err := waveform.OpenWithInfo(ctx, r, decoder, s.p.Track.Audio, cache, progress)
	if err != nil {
		return result, failure(HostFailure, err)
	}
	return result, nil
}
