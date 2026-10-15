// Package binding supplies a bounded JSON session bridge for local host workers.
// It contains binding/handle management only; rules remain in shared core APIs.
package binding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
	"github.com/NLstn/riffine/core/waveform"
)

const Version = 1
const MaxRequestBytes = project.MaxDocumentBytes + 4096

type Request struct {
	Version    int             `json:"version"`
	Type       string          `json:"type"`
	Source     string          `json:"source,omitempty"`
	Document   string          `json:"document,omitempty"`
	Command    json.RawMessage `json:"command,omitempty"`
	StartFrame int64           `json:"startFrame,omitempty"`
	EndFrame   int64           `json:"endFrame,omitempty"`
	Bins       int             `json:"bins,omitempty"`
}
type Response struct {
	Version  int                    `json:"version"`
	Preview  *engine.PreviewReport  `json:"preview,omitempty"`
	Snapshot *practice.Snapshot     `json:"snapshot,omitempty"`
	Events   *practice.EventBatch   `json:"events,omitempty"`
	Window   *waveform.Window       `json:"window,omitempty"`
	Document string                 `json:"document,omitempty"`
	Saved    *resource.UpdateResult `json:"saved,omitempty"`
	Error    *practice.Error        `json:"error,omitempty"`
}
type Session struct {
	mu      sync.Mutex
	service practice.Service
	cache   waveform.Cache
	session *practice.Session
	pyramid *waveform.Pyramid
	detail  *practice.WaveformDetail
	closed  bool
}

func New(service practice.Service) *Session { return &Session{service: service} }

// NewWithWaveformCache enables host-owned persistent peak storage.
func NewWithWaveformCache(service practice.Service, cache waveform.Cache) *Session {
	return &Session{service: service, cache: cache}
}

func bridgeError(err error) *practice.Error {
	var api *practice.Error
	if errors.As(err, &api) {
		return api
	}
	code := practice.HostFailure
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = practice.Cancelled
	}
	return &practice.Error{Version: practice.APIVersion, Code: code, Message: err.Error()}
}
func (s *Session) Handle(ctx context.Context, data []byte) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	response := Response{Version: Version}
	reject := func(err error) Response { response.Error = bridgeError(err); return response }
	invalid := func(message string) Response {
		return reject(&practice.Error{Version: practice.APIVersion, Code: practice.InvalidCommand, Message: message})
	}
	if err := ctx.Err(); err != nil {
		return reject(err)
	}
	if s.closed {
		return invalid("session bridge is closed")
	}
	if len(data) > MaxRequestBytes {
		return invalid("binding request exceeds limit")
	}
	var request Request
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return invalid(err.Error())
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return invalid("binding request contains trailing data")
	}
	if request.Version != Version {
		return reject(&practice.Error{Version: practice.APIVersion, Code: practice.UnsupportedVersion, Message: "unsupported binding version"})
	}

	if request.Type != "import" && request.Type != "open" && request.Source != "" {
		return invalid("source handles are only valid for import/open")
	}
	if request.Type != "import" && request.Type != "open" && request.Type != "save_as" && request.Document != "" {
		return invalid("document handles are only valid for import/open/save_as")
	}
	if request.Type != "command" && len(request.Command) != 0 {
		return invalid("command payload is only valid for command")
	}
	if request.Type != "waveform" && request.Type != "detail" && (request.StartFrame != 0 || request.EndFrame != 0 || request.Bins != 0) {
		return invalid("window fields are only valid for waveform/detail")
	}
	if s.session == nil && request.Type != "import" && request.Type != "open" {
		return invalid("open or import a project first")
	}
	switch request.Type {
	case "import", "open":
		if s.session != nil {
			return invalid("use a fresh bridge to open another project")
		}
		if request.Document == "" || (request.Type == "import" && request.Source == "") {
			return invalid("resource handles are required")
		}
		var session *practice.Session
		var err error
		if request.Type == "import" {
			session, err = s.service.Import(ctx, request.Source, request.Document)
		} else {
			session, err = s.service.Open(ctx, request.Document)
			if err == nil {
				if request.Source != "" {
					err = session.Relink(ctx, request.Source)
				} else {
					err = session.Verify(ctx)
				}
			}
		}
		if err != nil {
			return reject(err)
		}
		s.session = session
	case "command":
		command, err := practice.DecodeCommand(bytes.NewReader(request.Command))
		if err != nil {
			return reject(err)
		}
		snapshot, err := s.session.Dispatch(ctx, command)
		response.Snapshot = &snapshot
		if err != nil {
			return reject(err)
		}
	case "attach_transport":
		if err := s.session.AttachTransport(ctx); err != nil {
			snapshot := s.session.Snapshot()
			response.Snapshot = &snapshot
			return reject(err)
		}
	case "detach_transport":
		if err := s.session.DetachTransport(); err != nil {
			snapshot := s.session.Snapshot()
			response.Snapshot = &snapshot
			return reject(err)
		}
	case "poll_transport":
		snapshot, err := s.session.PollTransport(ctx)
		response.Snapshot = &snapshot
		if err != nil {
			return reject(err)
		}
	case "preview":
		report, err := s.session.Preview(ctx)
		if err != nil {
			return reject(err)
		}
		response.Preview = &report
	case "snapshot":
	case "events":
		batch := s.session.DrainEvents()
		response.Events = &batch
	case "export":
		data, err := project.Encode(s.session.Snapshot().Project)
		if err != nil {
			return reject(err)
		}
		response.Document = string(data)
	case "save_as":
		if request.Document == "" {
			return invalid("a save destination is required")
		}
		if err := s.session.SaveAs(ctx, request.Document); err != nil {
			return reject(err)
		}
	case "save":
		result, err := s.session.Save(ctx)
		if err != nil {
			return reject(err)
		}
		response.Saved = &result
	case "waveform":
		info := s.session.Snapshot().Project.Track.Audio
		if request.StartFrame < 0 || request.EndFrame <= request.StartFrame || request.EndFrame > info.Frames || request.Bins < 1 || request.Bins > waveform.MaxWindowBins {
			return invalid("invalid waveform window or bin limit")
		}
		if s.pyramid == nil {
			result, err := s.session.OpenWaveform(ctx, s.cache, nil)
			if err != nil {
				return reject(err)
			}
			s.pyramid = result.Pyramid
		}
		window, err := s.pyramid.Query(ctx, request.StartFrame, request.EndFrame, request.Bins)
		if err != nil {
			return reject(err)
		}
		response.Window = &window
	case "detail":
		info := s.session.Snapshot().Project.Track.Audio
		if request.StartFrame < 0 || request.EndFrame <= request.StartFrame || request.EndFrame > info.Frames || request.EndFrame-request.StartFrame > waveform.MaxDetailFrames || request.Bins < 1 || request.Bins > waveform.MaxWindowBins {
			return invalid("invalid detail range or bin limit")
		}
		if s.detail == nil {
			detail, err := s.session.OpenWaveformDetail(ctx)
			if err != nil {
				return reject(err)
			}
			s.detail = detail
		}
		window, err := s.detail.Query(ctx, request.StartFrame, request.EndFrame, request.Bins)
		if err != nil {
			s.detail.Close()
			s.detail = nil
			return reject(err)
		}
		response.Window = &window
	default:
		return invalid("unknown binding operation")
	}
	snapshot := s.session.Snapshot()
	response.Snapshot = &snapshot
	return response
}
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.pyramid = nil
	var err error
	if s.detail != nil {
		err = s.detail.Close()
		s.detail = nil
	}
	if s.session != nil {
		err = errors.Join(err, s.session.CloseTransport())
	}
	return err
}
