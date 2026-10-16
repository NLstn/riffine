package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/NLstn/riffine/adapters/binding"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/waveform"
)

// Host owns one session. A failed replacement leaves the previous session and
// retained waveform usable. Serve serializes all calls on its control worker.
type Host struct {
	Service practice.Service
	Cache   waveform.Cache
	session *binding.Session
}

func (h *Host) Handle(ctx context.Context, data []byte) any {
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(data, &probe)
	if probe.Type == "hello" {
		var q struct {
			Version int    `json:"version"`
			Type    string `json:"type"`
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		var extra any
		if err := dec.Decode(&q); err != nil || dec.Decode(&extra) != io.EOF || q.Version != Version {
			return binding.Response{Version: Version, Error: &practice.Error{Version: practice.APIVersion, Code: practice.InvalidCommand, Message: "invalid desktop handshake"}}
		}
		return struct {
			Version       int      `json:"version"`
			Formats       []string `json:"formats"`
			StreamPreview bool     `json:"streamPreview"`
		}{Version, h.Service.Decoder.Formats(), h.Service.SourcePreviewer != nil}
	}
	if probe.Type == "import" || probe.Type == "open" {
		next := binding.NewWithWaveformCache(h.Service, h.Cache)
		response := next.Handle(ctx, data)
		if response.Error != nil {
			_ = next.Close()
			return response
		}
		if h.session != nil {
			_ = h.session.Close()
		}
		h.session = next
		return response
	}
	if h.session == nil {
		h.session = binding.NewWithWaveformCache(h.Service, h.Cache)
	}
	return h.session.Handle(ctx, data)
}

func (h *Host) Close() error {
	if h.session != nil {
		return h.session.Close()
	}
	return nil
}
