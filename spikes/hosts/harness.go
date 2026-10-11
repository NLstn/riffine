// Package hosts is an executable feasibility harness, not a production binding.
// All clients pass fixture JSON to this Go code and compare the resulting JSON.
package hosts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"sync"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
)

const MaxFixtureBytes = 1 << 20
const MaxFixtureSteps = 256

type fixture struct {
	FixtureVersion int    `json:"fixtureVersion"`
	APIVersion     int    `json:"apiVersion"`
	Name           string `json:"name"`
	Audio          struct {
		Name                 string
		Seconds, Hz          float64
		SampleRate, Channels int
	} `json:"audio"`
	Steps []struct {
		Command      practice.Command
		Error        practice.ErrorCode
		Sequence     uint64
		Speed, Pitch float64
		Loop         project.Loop
	} `json:"steps"`
	Events []practice.Event `json:"events"`
	Render struct {
		StartFrame, EndFrame int64
		Speed, Pitch         float64
	} `json:"render"`
}

type Step struct {
	Snapshot practice.Snapshot  `json:"snapshot"`
	Error    practice.ErrorCode `json:"error,omitempty"`
}
type Report struct {
	Transport         TransportReport             `json:"transport"`
	Version           int                         `json:"version"`
	OK                bool                        `json:"ok"`
	Error             string                      `json:"error,omitempty"`
	Steps             []Step                      `json:"steps"`
	Events            []practice.Event            `json:"events"`
	Project           json.RawMessage             `json:"project"`
	Cancellation      bool                        `json:"cancellation"`
	ResourceOwnership bool                        `json:"resourceOwnership"`
	Render            engine.RenderRequest        `json:"render"`
	LoopCompletion    *engine.TraversalCompletion `json:"loopCompletion"`
	DuplicateIgnored  bool                        `json:"duplicateIgnored"`
	StaleIgnored      bool                        `json:"staleIgnored"`
}

// Run always returns JSON. No filesystem, UI or device API is used by the harness.
// The recording renderer and fake device exercise control boundaries only.
func Run(input string) string {
	r, err := execute(input)
	if err != nil {
		r.Error = err.Error()
	} else {
		r.OK = true
	}
	b, err := json.Marshal(r)
	if err != nil {
		return `{"version":1,"ok":false,"error":"report encoding failed"}`
	}
	return string(b)
}
func execute(input string) (r Report, err error) {
	r.Version = 1
	if len(input) > MaxFixtureBytes {
		return r, errors.New("fixture exceeds 1 MiB")
	}
	var f fixture
	dec := json.NewDecoder(bytes.NewBufferString(input))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&f); err != nil {
		return r, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return r, errors.New("unexpected trailing fixture data")
	}
	if len(f.Steps) == 0 || len(f.Steps) > MaxFixtureSteps {
		return r, errors.New("fixture requires 1–256 steps")
	}
	if f.Audio.Seconds*float64(f.Audio.SampleRate) > 192000 {
		return r, errors.New("spike tone exceeds 192,000 source frames")
	}
	// The fixture's nested fields are defined by the shared schema, not a host UI.
	if f.FixtureVersion != 1 || f.APIVersion != practice.APIVersion {
		return r, errors.New("unsupported fixture version")
	}
	var tone bytes.Buffer
	if err = audio.WriteTone(&tone, f.Audio.Seconds, f.Audio.Hz, f.Audio.SampleRate, f.Audio.Channels); err != nil {
		return r, err
	}
	host := &memory{data: map[string][]byte{"source": tone.Bytes()}, name: f.Audio.Name}
	renderer := &recordingRenderer{}
	service := practice.Service{Resources: host, Store: host, Decoder: audio.WAVDecoder{}, Renderer: renderer}
	ctx := context.Background()
	session, err := service.Import(ctx, "source", "project")
	if err != nil {
		return r, err
	}
	for _, step := range f.Steps {
		snapshot, e := session.Dispatch(ctx, step.Command)
		var code practice.ErrorCode
		if e != nil {
			var apiError *practice.Error
			if !errors.As(e, &apiError) {
				return r, e
			}
			code = apiError.Code
		}
		r.Steps = append(r.Steps, Step{Snapshot: snapshot, Error: code})
		if code != step.Error || snapshot.Sequence != step.Sequence || snapshot.Project.Settings != (project.Settings{Speed: step.Speed, Pitch: step.Pitch, Loop: step.Loop}) {
			return r, fmt.Errorf("command %d does not conform", step.Command.Sequence)
		}
	}
	batch := session.DrainEvents()
	r.Events = batch.Events
	if batch.Dropped != 0 || !reflect.DeepEqual(r.Events, f.Events) {
		return r, errors.New("event trace differs")
	}
	if err = session.SaveAs(ctx, "project"); err != nil {
		return r, err
	}
	reopened, err := service.Open(ctx, "project")
	if err != nil {
		return r, err
	}
	if reopened.Snapshot().Project != session.Snapshot().Project {
		return r, errors.New("project round-trip differs")
	}
	r.Project = append([]byte(nil), host.data["project"]...)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	before := reopened.Snapshot()
	speed := 1.0
	_, e := reopened.Dispatch(cancelled, practice.Command{Version: 1, Sequence: 1, Type: "set_parameters", Speed: &speed})
	r.Cancellation = errors.Is(e, context.Canceled) && reflect.DeepEqual(before, reopened.Snapshot())
	if !r.Cancellation {
		return r, errors.New("cancelled command committed")
	}
	if err = reopened.Render(ctx, "output"); err != nil {
		return r, err
	}
	r.Render = renderer.request
	if r.Render.StartFrame != f.Render.StartFrame || r.Render.EndFrame != f.Render.EndFrame || r.Render.Speed != f.Render.Speed || r.Render.Pitch != f.Render.Pitch {
		return r, errors.New("render range differs")
	}
	// A fake output device consumes one scheduled enabled loop. This exercises a
	// Go acknowledgement gate; it does not establish audible/live playback.
	frames := int64(float64(r.Render.EndFrame-r.Render.StartFrame)/r.Render.Speed + .5)
	schedule := engine.Schedule{Generation: 1, Sequence: 1, Source: "source", StartFrame: r.Render.StartFrame, EndFrame: r.Render.EndFrame, OutputStart: 0, OutputEnd: frames, Speed: r.Render.Speed, Pitch: r.Render.Pitch}
	traversal, err := engine.NewTraversal(schedule)
	if err != nil {
		return r, err
	}
	device := &fakeOutput{}
	if err = device.Schedule(ctx, schedule); err != nil {
		return r, err
	}
	partial := engine.OutputAcknowledgement{Generation: 1, Sequence: 1, ConsumedOutputFrame: frames / 2}
	completion, e := traversal.Acknowledge(partial)
	if e != nil || completion != nil {
		return r, errors.New("partial traversal counted")
	}
	stale := engine.OutputAcknowledgement{Generation: 0, Sequence: 1, ConsumedOutputFrame: frames}
	completion, e = traversal.Acknowledge(stale)
	r.StaleIgnored = e == nil && completion == nil
	acknowledgements, err := device.DrainAcknowledgements(ctx, 1)
	if err != nil {
		return r, err
	}
	r.LoopCompletion, err = traversal.Acknowledge(acknowledgements[0])
	if err != nil {
		return r, err
	}
	completion, e = traversal.Acknowledge(acknowledgements[0])
	r.DuplicateIgnored = e == nil && completion == nil
	r.ResourceOwnership = host.opened == host.closed
	if r.LoopCompletion == nil || !r.DuplicateIgnored || !r.StaleIgnored || !r.ResourceOwnership {
		return r, errors.New("acknowledgement/resource contract differs")
	}
	r.Transport, err = transportConformance(ctx, r.Render, session.Snapshot().Project.Track.Audio.Frames, session.Snapshot().Project.Track.Audio.SampleRate)
	return r, err
}

type memory struct {
	mu             sync.Mutex
	data           map[string][]byte
	name           string
	opened, closed int
}
type reader struct {
	*bytes.Reader
	host *memory
}

func (r reader) Name() string { return r.host.name }
func (r reader) Close() error {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	r.host.closed++
	return nil
}
func (h *memory) Open(ctx context.Context, handle string) (resource.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.data[handle]
	if !ok {
		return nil, fs.ErrNotExist
	}
	h.opened++
	return reader{bytes.NewReader(append([]byte(nil), b...)), h}, nil
}
func (*memory) Resolve(ctx context.Context, _, ref string) (string, error) { return ref, ctx.Err() }
func (*memory) Reference(ctx context.Context, _, handle string) (string, error) {
	return handle, ctx.Err()
}
func (h *memory) Read(ctx context.Context, handle string, max int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.data[handle]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if len(b) > max {
		return nil, errors.New("document exceeds limit")
	}
	return append([]byte(nil), b...), nil
}
func (h *memory) Create(ctx context.Context, handle string, b []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.data[handle]; ok {
		return fs.ErrExist
	}
	h.data[handle] = append([]byte(nil), b...)
	return nil
}

type recordingRenderer struct{ request engine.RenderRequest }

func (r *recordingRenderer) Render(ctx context.Context, q engine.RenderRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.request = q
	return nil
}

// fakeOutput implements the same bounded control port planned for native/WASM.
// No audio callback, samples, decoding or practice evaluator runs in the host.
type fakeOutput struct{ pending *engine.OutputAcknowledgement }

func (d *fakeOutput) Schedule(ctx context.Context, s engine.Schedule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d.pending != nil {
		return errors.New("fake output queue full")
	}
	d.pending = &engine.OutputAcknowledgement{Generation: s.Generation, Sequence: s.Sequence, ConsumedOutputFrame: s.OutputEnd}
	return nil
}
func (d *fakeOutput) Reset(ctx context.Context, _ uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.pending = nil
	return nil
}
func (d *fakeOutput) DrainAcknowledgements(ctx context.Context, limit int) ([]engine.OutputAcknowledgement, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 32 {
		return nil, errors.New("invalid drain limit")
	}
	if d.pending == nil {
		return []engine.OutputAcknowledgement{}, nil
	}
	result := []engine.OutputAcknowledgement{*d.pending}
	d.pending = nil
	return result, nil
}
