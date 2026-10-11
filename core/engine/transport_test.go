package engine

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
)

type transportFake struct {
	schedules   []Schedule
	acks        []OutputAcknowledgement
	generations []uint64
	running     bool
	failure     error
}

func (f *transportFake) Schedule(_ context.Context, s Schedule) error {
	if f.failure != nil {
		return f.failure
	}
	f.schedules = append(f.schedules, s)
	return nil
}
func (f *transportFake) Reset(_ context.Context, g uint64) error {
	f.running = false
	f.generations = append(f.generations, g)
	f.acks = nil
	return f.failure
}
func (f *transportFake) Start(context.Context) error {
	if f.failure != nil {
		return f.failure
	}
	f.running = true
	return nil
}
func (f *transportFake) Suspend(context.Context) ([]OutputAcknowledgement, error) {
	f.running = false
	a := f.acks
	f.acks = nil
	return a, f.failure
}
func (f *transportFake) DrainAcknowledgements(_ context.Context, n int) ([]OutputAcknowledgement, error) {
	a := f.acks
	f.acks = nil
	return a, f.failure
}
func setupTransport(t *testing.T) (*Transport, *transportFake) {
	t.Helper()
	f := &transportFake{}
	tr, e := NewTransport(f)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Load(context.Background(), TransportConfig{Source: "fixture", Frames: 48000, SampleRate: 48000, OutputRate: 96000, Speed: .5, Pitch: 3, LoopEnabled: true, LoopStart: 12000, LoopEnd: 36000}); e != nil {
		t.Fatal(e)
	}
	return tr, f
}
func command(t *testing.T, tr *Transport, kind string, edit func(*TransportCommand)) TransportSnapshot {
	t.Helper()
	c := TransportCommand{Version: 1, Sequence: tr.Snapshot().Sequence + 1, Type: kind}
	if edit != nil {
		edit(&c)
	}
	s, e := tr.Dispatch(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func consumed(s Schedule, n int64) OutputAcknowledgement {
	return OutputAcknowledgement{Generation: s.Generation, Sequence: s.Sequence, ConsumedOutputFrame: n}
}
func poll(t *testing.T, tr *Transport) TransportSnapshot {
	t.Helper()
	s, e := tr.Poll(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestTransportAcknowledgedPositionAndLoops(t *testing.T) {
	tr, f := setupTransport(t)
	command(t, tr, "play", nil)
	if !f.running || len(f.schedules) != 2 {
		t.Fatal("missing planned playback")
	}
	first, second := f.schedules[0], f.schedules[1]
	if first.OutputEnd != 96000 || second.OutputStart != 96000 || second.OutputEnd != 192000 {
		t.Fatal(f.schedules)
	}
	if s := poll(t, tr); s.SourceFrame != 12000 || s.CompletedLoops != 0 {
		t.Fatal("decode/wall time advanced playback", s)
	}
	f.acks = []OutputAcknowledgement{consumed(first, 48000)}
	if s := poll(t, tr); s.SourceFrame != 24000 || s.CompletedLoops != 0 {
		t.Fatal(s)
	}
	f.acks = []OutputAcknowledgement{consumed(first, first.OutputEnd), consumed(first, first.OutputEnd), consumed(second, second.OutputEnd)}
	s := poll(t, tr)
	if s.CompletedLoops != 2 || s.SourceFrame != 12000 || len(f.schedules) != 4 {
		t.Fatal(s, f.schedules)
	}
	e := tr.DrainEvents()
	if len(e) != 2 || e[0].Sequence == e[1].Sequence || e[1].CompletedLoops != 2 {
		t.Fatal(e)
	}
}
func TestTransportPauseResumeDropsPartialTraversal(t *testing.T) {
	tr, f := setupTransport(t)
	command(t, tr, "play", nil)
	first := f.schedules[0]
	f.acks = []OutputAcknowledgement{consumed(first, 48000)}
	paused := command(t, tr, "pause", nil)
	if f.running || paused.State != "paused" || paused.SourceFrame != 24000 {
		t.Fatal(paused)
	}
	resumed := command(t, tr, "play", nil)
	last := f.schedules[len(f.schedules)-2]
	if last.StartFrame != 24000 || resumed.Generation <= first.Generation {
		t.Fatal(last, resumed)
	}
	f.acks = []OutputAcknowledgement{consumed(first, first.OutputEnd), consumed(last, last.OutputEnd)}
	if s := poll(t, tr); s.CompletedLoops != 0 {
		t.Fatal("resumed partial counted", s)
	}
	next := f.schedules[len(f.schedules)-2]
	f.acks = []OutputAcknowledgement{consumed(next, next.OutputEnd)}
	if s := poll(t, tr); s.CompletedLoops != 1 {
		t.Fatal(s)
	}
}
func TestTransportEditsSeekStopAndRestart(t *testing.T) {
	tr, f := setupTransport(t)
	command(t, tr, "play", nil)
	s := f.schedules[0]
	f.acks = []OutputAcknowledgement{consumed(s, 48000)}
	speed := 1.0
	pitch := -4.0
	edited := command(t, tr, "set_parameters", func(c *TransportCommand) { c.Speed = &speed; c.Pitch = &pitch })
	p := f.schedules[len(f.schedules)-2]
	if p.StartFrame != 24000 || p.Speed != 1 || p.Pitch != -4 || p.OutputEnd != 24000 || edited.Config.Pitch != -4 {
		t.Fatal(p, edited)
	}
	pos := int64(12000)
	seeked := command(t, tr, "seek", func(c *TransportCommand) { c.Position = &pos })
	if seeked.SourceFrame != pos || seeked.CompletedLoops != 0 {
		t.Fatal(seeked)
	}
	loop := TransportLoop{Enabled: true, StartFrame: 100, EndFrame: 101}
	looped := command(t, tr, "set_loop", func(c *TransportCommand) { c.Loop = &loop })
	if looped.SourceFrame != 100 {
		t.Fatal(looped)
	}
	tiny := f.schedules[len(f.schedules)-2]
	f.acks = []OutputAcknowledgement{consumed(tiny, tiny.OutputEnd)}
	if s := poll(t, tr); s.CompletedLoops != 1 {
		t.Fatal(s)
	}
	restarted := command(t, tr, "restart", nil)
	if restarted.State != "playing" || restarted.CompletedLoops != 0 || restarted.SourceFrame != 100 {
		t.Fatal(restarted)
	}
	stopped := command(t, tr, "stop", nil)
	if f.running || stopped.State != "stopped" || stopped.SourceFrame != 100 || stopped.CompletedLoops != 0 {
		t.Fatal(stopped)
	}
}
func TestTransportTinyPartialNeverCountsAfterPause(t *testing.T) {
	tr, f := setupTransport(t)
	loop := TransportLoop{Enabled: true, StartFrame: 0, EndFrame: 1}
	command(t, tr, "set_loop", func(c *TransportCommand) { c.Loop = &loop })
	command(t, tr, "play", nil)
	s := f.schedules[0]
	if s.OutputEnd != 4 {
		t.Fatal(s)
	}
	f.acks = []OutputAcknowledgement{consumed(s, 1)}
	command(t, tr, "pause", nil)
	command(t, tr, "play", nil)
	resumed := f.schedules[len(f.schedules)-2]
	f.acks = []OutputAcknowledgement{consumed(resumed, resumed.OutputEnd)}
	if s := poll(t, tr); s.CompletedLoops != 0 {
		t.Fatal("rounded source position counted partial", s)
	}
}
func TestTransportEndOfTrack(t *testing.T) {
	tr, f := setupTransport(t)
	command(t, tr, "set_loop", func(c *TransportCommand) { c.Loop = &TransportLoop{} })
	command(t, tr, "play", nil)
	if len(f.schedules) != 1 {
		t.Fatal(f.schedules)
	}
	s := f.schedules[0]
	f.acks = []OutputAcknowledgement{consumed(s, s.OutputEnd)}
	done := poll(t, tr)
	if done.State != "stopped" || done.SourceFrame != 48000 || f.running {
		t.Fatal(done)
	}
	command(t, tr, "play", nil)
	if f.schedules[1].StartFrame != 0 {
		t.Fatal("play at EOF must start at selection beginning")
	}
}
func TestTransportFailuresRejectInvalidAndRecover(t *testing.T) {
	tr, f := setupTransport(t)
	before := tr.Snapshot()
	invalid := math.NaN()
	if _, e := tr.Dispatch(context.Background(), TransportCommand{Version: 1, Sequence: 1, Type: "set_parameters", Speed: &invalid}); e == nil {
		t.Fatal("accepted NaN")
	}
	if tr.Snapshot() != before {
		t.Fatal("invalid command changed state")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := tr.Dispatch(ctx, TransportCommand{Version: 1, Sequence: 1, Type: "play"}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	command(t, tr, "play", nil)
	second := f.schedules[1]
	f.acks = []OutputAcknowledgement{consumed(second, second.OutputEnd)}
	if s, e := tr.Poll(context.Background()); e == nil || s.State != "error" || f.running {
		t.Fatal(s, e, "must suspend malformed acknowledgement")
	}
	recovered := command(t, tr, "recover", nil)
	if recovered.State != "stopped" || recovered.Error != "" {
		t.Fatal(recovered)
	}
	f.failure = errors.New("device lost")
	if s, e := tr.Dispatch(context.Background(), TransportCommand{Version: 1, Sequence: recovered.Sequence + 1, Type: "play"}); e == nil || s.State != "error" || s.Sequence != recovered.Sequence {
		t.Fatal(s, e)
	}
	f.failure = nil
	command(t, tr, "recover", nil)
}
func TestTransportCommandRace(t *testing.T) {
	tr, _ := setupTransport(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := tr.Dispatch(context.Background(), TransportCommand{Version: 1, Sequence: 1, Type: "play"})
			if e == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if success != 1 || tr.Snapshot().Sequence != 1 {
		t.Fatal(success, tr.Snapshot())
	}
}
func TestTransportBoundedEventOverflow(t *testing.T) {
	tr, f := setupTransport(t)
	command(t, tr, "play", nil)
	for i := 0; i < transportEventCapacity; i++ {
		s := f.schedules[i]
		f.acks = []OutputAcknowledgement{consumed(s, s.OutputEnd)}
		poll(t, tr)
	}
	s := f.schedules[transportEventCapacity]
	f.acks = []OutputAcknowledgement{consumed(s, s.OutputEnd)}
	if state, e := tr.Poll(context.Background()); e == nil || state.State != "error" || f.running {
		t.Fatal(state, e)
	}
	if len(tr.DrainEvents()) != transportEventCapacity {
		t.Fatal("lost accepted completions")
	}
}
