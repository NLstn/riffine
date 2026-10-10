package practice

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"sync"
	"testing"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
)

//go:embed testdata/session-v1.json
var sessionFixture []byte

type memoryReader struct {
	*bytes.Reader
	name string
	host *memoryHost
}

func (r memoryReader) Name() string { return r.name }
func (r memoryReader) Close() error { r.host.closed++; return nil }

type memoryHost struct {
	data           map[string][]byte
	opened, closed int
	failWrite      bool
}

func (h *memoryHost) Open(ctx context.Context, handle string) (resource.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, ok := h.data[handle]
	if !ok {
		return nil, fs.ErrNotExist
	}
	h.opened++
	return memoryReader{bytes.NewReader(append([]byte(nil), b...)), "tone.wav", h}, nil
}
func (*memoryHost) Resolve(ctx context.Context, _, ref string) (string, error) { return ref, ctx.Err() }
func (*memoryHost) Reference(ctx context.Context, _, handle string) (string, error) {
	return handle, ctx.Err()
}
func (h *memoryHost) Read(ctx context.Context, handle string, max int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, ok := h.data[handle]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if len(b) > max {
		return nil, errors.New("too large")
	}
	return append([]byte(nil), b...), nil
}
func (h *memoryHost) Create(ctx context.Context, handle string, b []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if h.failWrite {
		return errors.New("injected storage failure")
	}
	if _, ok := h.data[handle]; ok {
		return fs.ErrExist
	}
	h.data[handle] = append([]byte(nil), b...)
	return nil
}

type recorder struct {
	request engine.RenderRequest
	called  bool
}

func (r *recorder) Render(ctx context.Context, q engine.RenderRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.request, r.called = q, true
	return nil
}
func newSession(t *testing.T) (*Session, *memoryHost, *recorder) {
	t.Helper()
	var b bytes.Buffer
	if err := audio.WriteTone(&b, 1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	host := &memoryHost{data: map[string][]byte{"opaque:original": b.Bytes()}}
	r := &recorder{}
	s, err := (Service{Resources: host, Store: host, Decoder: audio.WAVDecoder{}, Renderer: r}).Import(context.Background(), "opaque:original", "opaque:project")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if host.opened != host.closed {
			t.Errorf("resource leak: opened %d, closed %d", host.opened, host.closed)
		}
	})
	return s, host, r
}
func errorCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	var e *Error
	if !errors.As(err, &e) || e.Code != want {
		t.Fatalf("error=%v, want %s", err, want)
	}
}
func TestSharedConformanceFixture(t *testing.T) {
	var fixture struct {
		FixtureVersion int `json:"fixtureVersion"`
		APIVersion     int `json:"apiVersion"`
		Steps          []struct {
			Command      Command
			Error        ErrorCode
			Sequence     uint64
			Speed, Pitch float64
			Loop         project.Loop
		}
		Events []Event
		Render struct {
			StartFrame, EndFrame int64
			Speed, Pitch         float64
		}
	}
	if err := json.Unmarshal(sessionFixture, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.FixtureVersion != 1 || fixture.APIVersion != APIVersion {
		t.Fatal("unsupported fixture")
	}
	s, _, r := newSession(t)
	for _, step := range fixture.Steps {
		got, err := s.Dispatch(context.Background(), step.Command)
		errorCode(t, err, step.Error)
		if got.Version != APIVersion || got.State != "ready" || got.Sequence != step.Sequence || got.Project.Settings != (project.Settings{Speed: step.Speed, Pitch: step.Pitch, Loop: step.Loop}) {
			t.Fatalf("unexpected snapshot: %+v", got)
		}
	}
	batch := s.DrainEvents()
	if batch.Dropped != 0 || !reflect.DeepEqual(batch.Events, fixture.Events) {
		t.Fatalf("events: %+v", batch)
	}
	if err := s.Render(context.Background(), "opaque:output"); err != nil {
		t.Fatal(err)
	}
	want := fixture.Render
	if !r.called || r.request.StartFrame != want.StartFrame || r.request.EndFrame != want.EndFrame || r.request.Speed != want.Speed || r.request.Pitch != want.Pitch {
		t.Fatalf("wrong request: %+v", r.request)
	}
	if len(s.DrainEvents().Events) != 0 {
		t.Fatal("render generated playback events")
	}
}
func TestMemoryRoundTripAndRelink(t *testing.T) {
	s, host, _ := newSession(t)
	speed, pitch := .5, -2.0
	_, err := s.Dispatch(context.Background(), Command{Version: 1, Sequence: 1, Type: "set_parameters", Speed: &speed, Pitch: &pitch})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAs(context.Background(), "opaque:project"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveAs(context.Background(), "opaque:original"); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("overwrote audio: %v", err)
	}
	opened, err := s.service.Open(context.Background(), "opaque:project")
	if err != nil {
		t.Fatal(err)
	}
	if opened.Snapshot().Project != s.Snapshot().Project {
		t.Fatal("round-trip changed saved units/identity")
	}
	host.data["opaque:moved"] = host.data["opaque:original"]
	delete(host.data, "opaque:original")
	errorCode(t, opened.Verify(context.Background()), ResourceUnavailable)
	if err = opened.Relink(context.Background(), "opaque:moved"); err != nil {
		t.Fatal(err)
	}
	if err = opened.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	w, err := opened.Waveform(context.Background(), 20)
	if err != nil || w.Frames != 48000 || len(w.Channels[0]) != 20 {
		t.Fatalf("waveform: %+v, %v", w, err)
	}
	host.data["opaque:changed"] = append([]byte(nil), host.data["opaque:moved"]...)
	host.data["opaque:changed"][100] ^= 1
	before := opened.Snapshot()
	errorCode(t, opened.Relink(context.Background(), "opaque:changed"), IdentityMismatch)
	if !reflect.DeepEqual(before, opened.Snapshot()) {
		t.Fatal("failed relink mutated session")
	}
}
func TestCancellationAndFailedSave(t *testing.T) {
	s, host, r := newSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	speed := .5
	before := s.Snapshot()
	_, err := s.Dispatch(ctx, Command{Version: 1, Sequence: 1, Type: "set_parameters", Speed: &speed})
	errorCode(t, err, Cancelled)
	if !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation cause")
	}
	errorCode(t, s.SaveAs(ctx, "cancelled"), Cancelled)
	errorCode(t, s.Render(ctx, "output"), Cancelled)
	if r.called || !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("cancelled operation committed")
	}
	host.failWrite = true
	errorCode(t, s.SaveAs(context.Background(), "failed"), HostFailure)
	if _, exists := host.data["failed"]; exists {
		t.Fatal("failed save published partial document")
	}
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("failed save mutated session")
	}
}
func TestBoundedEventsAndSnapshotOwnership(t *testing.T) {
	s, _, _ := newSession(t)
	speed := .75
	for i := uint64(1); i <= EventCapacity+5; i++ {
		if _, err := s.Dispatch(context.Background(), Command{Version: 1, Sequence: i, Type: "set_parameters", Speed: &speed}); err != nil {
			t.Fatal(err)
		}
	}
	b := s.DrainEvents()
	if len(b.Events) != EventCapacity || b.Dropped != 5 || b.Events[0].Sequence != 6 || b.Snapshot.Sequence != EventCapacity+5 {
		t.Fatalf("overflow contract: %+v", b)
	}
	b.Snapshot.Capabilities.Formats[0] = "bogus"
	if s.Snapshot().Capabilities.Formats[0] != "wav-pcm" {
		t.Fatal("caller mutated capabilities")
	}
	if len(s.DrainEvents().Events) != 0 || s.DrainEvents().Dropped != 0 {
		t.Fatal("drain did not reset queue")
	}
}
func TestConcurrentCommandsHaveOneWinner(t *testing.T) {
	s, _, _ := newSession(t)
	speed := .5
	var wg sync.WaitGroup
	results := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Dispatch(context.Background(), Command{Version: 1, Sequence: 1, Type: "set_parameters", Speed: &speed})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			errorCode(t, err, OutOfOrder)
		}
	}
	if winners != 1 || s.Snapshot().Sequence != 1 {
		t.Fatal("command race applied more than once")
	}
}
func TestMissingPortsAndUnsupportedRenderer(t *testing.T) {
	_, err := (Service{}).Open(context.Background(), "project")
	errorCode(t, err, HostFailure)
	s, _, _ := newSession(t)
	s.service.Renderer = nil
	if s.Snapshot().Capabilities.LiveTransport || s.Snapshot().Capabilities.OfflineRender {
		t.Fatal("advertised unavailable capability")
	}
	errorCode(t, s.Render(context.Background(), "output"), UnsupportedCapability)
}

func TestStrictCommandDecode(t *testing.T) {
	for _, input := range []string{
		`{"version":1,"sequence":1,"type":"set_parameters","surprise":true}`,
		`{"version":1} {}`,
		string(bytes.Repeat([]byte(" "), MaxCommandBytes+1)),
	} {
		_, err := DecodeCommand(bytes.NewBufferString(input))
		errorCode(t, err, InvalidCommand)
	}
	_, err := DecodeCommand(bytes.NewBufferString(`{"version":2,"sequence":1,"type":"set_parameters","speed":1}`))
	errorCode(t, err, UnsupportedVersion)
	c, err := DecodeCommand(bytes.NewBufferString(`{"version":1,"sequence":1,"type":"set_parameters","speed":0.75}`))
	if err != nil || c.Speed == nil || *c.Speed != .75 {
		t.Fatalf("decode: %+v, %v", c, err)
	}
	s, _, _ := newSession(t)
	before := s.Snapshot()
	loop := project.Loop{Enabled: true, Start: 0, End: .5}
	speed := .5
	_, err = s.Dispatch(context.Background(), Command{Version: 1, Sequence: 1, Type: "set_parameters", Speed: &speed, Loop: &loop})
	errorCode(t, err, InvalidCommand)
	if !reflect.DeepEqual(before, s.Snapshot()) {
		t.Fatal("invalid payload mutated state")
	}
}
