package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/ladder"
	"github.com/NLstn/riffine/core/timing"
)

func (f *transportFake) SupportsCues() bool      { return true }
func (f *transportFake) SupportsMetronome() bool { return true }
func cueOptions() *timing.CueDefinition {
	return &timing.CueDefinition{Version: 1, CountInBeats: 4, RestSeconds: 1.25, ClickLevel: .7, Metronome: true, Grid: &timing.Definition{Version: 1, SourceBPM: 120, BeatsPerBar: 4}}
}
func setupCues(t *testing.T) (*Transport, *transportFake) {
	t.Helper()
	tr, f := setupTransport(t)
	c := tr.Snapshot().Config
	c.Cues = cueOptions()
	if err := tr.LoadPlan(context.Background(), c, ladder.Definition{StartSpeed: .6, TargetSpeed: .65, IncrementPoints: 5, LoopsPerStep: 2, EndBehavior: "stop"}); err != nil {
		t.Fatal(err)
	}
	return tr, f
}
func TestCuesCountOnlyAudioAndRestSecondsDoNotScale(t *testing.T) {
	tr, f := setupCues(t)
	start := command(t, tr, "play", nil)
	if start.Phase != "count_in" || len(f.schedules) != 2 {
		t.Fatal(start, f.schedules)
	}
	count := f.schedules[0]
	if count.Phase != "count_in" || count.OutputEnd != 320000 || len(count.Clicks) != 4 || count.Clicks[1].OutputFrame != 80000 {
		t.Fatal(count)
	}
	f.acks = []OutputAcknowledgement{consumed(count, count.OutputEnd), consumed(count, count.OutputEnd)}
	if s := poll(t, tr); s.CompletedLoops != 0 || s.Plan.CompletedLoops != 0 || s.Phase != "audio" {
		t.Fatal(s)
	}
	for ordinal := 1; ordinal <= 4; ordinal++ {
		var audio Schedule
		for _, schedule := range f.schedules {
			if schedule.Sequence > count.Sequence && schedule.Audio() {
				audio = schedule
				break
			}
		}
		speed := .6
		if ordinal > 2 {
			speed = .65
		}
		if audio.Speed != speed {
			t.Fatal(ordinal, audio.Speed)
		}
		f.acks = []OutputAcknowledgement{consumed(audio, audio.OutputEnd)}
		s := poll(t, tr)
		if s.CompletedLoops != uint64(ordinal) || s.Plan.CompletedLoops != uint64(ordinal) {
			t.Fatal(s)
		}
		count = audio
		if ordinal == 4 {
			if s.State != "stopped" || s.Phase != "" {
				t.Fatal(s)
			}
			break
		}
		var rest Schedule
		for _, schedule := range f.schedules {
			if schedule.Sequence > audio.Sequence && schedule.Phase == "rest" {
				rest = schedule
				break
			}
		}
		if ordinal == 2 && tr.Snapshot().Config.Speed != .65 {
			t.Fatal("rest did not display the upcoming ladder speed", tr.Snapshot())
		}
		if rest.OutputEnd-rest.OutputStart != 120000 || len(rest.Clicks) != 0 {
			t.Fatal("rest seconds scaled with speed", rest)
		}
		f.acks = []OutputAcknowledgement{consumed(rest, rest.OutputEnd)}
		if s = poll(t, tr); s.CompletedLoops != uint64(ordinal) || s.Phase != "audio" {
			t.Fatal(s)
		}
	}
	if len(f.schedules) != 8 {
		t.Fatal("extra count-in or trailing rest", f.schedules)
	}
	events := tr.DrainEvents()
	audio, phases := 0, 0
	for _, event := range events {
		if event.Type == "traversal_completed" {
			audio++
		} else if event.Type == "phase_completed" {
			phases++
		}
	}
	if audio != 4 || phases != 4 {
		t.Fatal(events)
	}
}
func TestPauseResumeCueRetainsExactRemainingFramesAndClickPhase(t *testing.T) {
	tr, f := setupCues(t)
	command(t, tr, "play", nil)
	first := f.schedules[0]
	f.acks = []OutputAcknowledgement{consumed(first, 12345)}
	paused := command(t, tr, "pause", nil)
	if paused.Phase != "count_in" || paused.PhaseRemainingFrames != first.OutputEnd-12345 || paused.CompletedLoops != 0 {
		t.Fatal(paused)
	}
	if s := poll(t, tr); !reflect.DeepEqual(s, paused) {
		t.Fatal("wall-time advanced paused cue")
	}
	command(t, tr, "play", nil)
	resumed := f.schedules[len(f.schedules)-2]
	if resumed.Phase != "count_in" || resumed.CueOffsetFrames != 12345 || resumed.OutputEnd-resumed.OutputStart != first.OutputEnd-12345 || !reflect.DeepEqual(resumed.Clicks, first.Clicks) {
		t.Fatal(resumed)
	}
	f.acks = []OutputAcknowledgement{consumed(first, first.OutputEnd), consumed(resumed, resumed.OutputEnd)}
	if s := poll(t, tr); s.CompletedLoops != 0 || s.Phase != "audio" {
		t.Fatal(s)
	}
	audio := f.schedules[len(f.schedules)-2]
	f.acks = []OutputAcknowledgement{consumed(audio, audio.OutputEnd)}
	poll(t, tr)
	rest := f.schedules[len(f.schedules)-2]
	if rest.Phase != "rest" {
		t.Fatal(rest)
	}
	f.acks = []OutputAcknowledgement{consumed(rest, rest.OutputStart+2000)}
	paused = command(t, tr, "pause", nil)
	if paused.Phase != "rest" || paused.PhaseRemainingFrames != 118000 || paused.CompletedLoops != 1 {
		t.Fatal(paused)
	}
	command(t, tr, "play", nil)
	resumed = f.schedules[len(f.schedules)-2]
	if resumed.Phase != "rest" || resumed.CueOffsetFrames != 2000 || resumed.OutputEnd-resumed.OutputStart != 118000 {
		t.Fatal(resumed)
	}
	command(t, tr, "stop", nil)
	if tr.Snapshot().Phase != "" {
		t.Fatal("stop retained cue progress")
	}
	command(t, tr, "play", nil)
	if f.schedules[len(f.schedules)-2].Phase != "count_in" {
		t.Fatal("new run omitted count-in")
	}
}
func TestCueMetronomeMatchesGridAndLiveSpeedEditRetainsCue(t *testing.T) {
	tr, f := setupCues(t)
	command(t, tr, "play", nil)
	first := f.schedules[0]
	f.acks = []OutputAcknowledgement{consumed(first, 10000)}
	speed := .8
	command(t, tr, "set_parameters", func(c *TransportCommand) { c.Speed = &speed })
	resumed, audio := f.schedules[len(f.schedules)-2], f.schedules[len(f.schedules)-1]
	if resumed.CueOffsetFrames != 10000 || resumed.OutputEnd != first.OutputEnd-10000 || audio.Speed != speed {
		t.Fatal(resumed, audio)
	}
	clicks, err := audio.ClickWindow(context.Background(), 0, audio.OutputEnd-audio.OutputStart)
	if err != nil || len(clicks) != 1 || clicks[0].OutputFrame != 30000 {
		t.Fatal(clicks, err)
	}
	f.acks = []OutputAcknowledgement{consumed(resumed, resumed.OutputEnd)}
	if s := poll(t, tr); s.Config.Speed != speed {
		t.Fatal("old cue tempo replaced edited song speed", s)
	}
	command(t, tr, "seek", func(c *TransportCommand) { position := int64(20000); c.Position = &position })
	if f.schedules[len(f.schedules)-2].Phase != "" {
		t.Fatal("seek restarted preparation phase")
	}
}

type noCues struct{ TransportDevice }

func TestUnsupportedCueHostRejectsBeforeDeviceChanges(t *testing.T) {
	f := &transportFake{}
	tr, _ := NewTransport(noCues{f})
	config := TransportConfig{Source: "fixture", Frames: 48000, SampleRate: 48000, OutputRate: 48000, Speed: 1}
	if err := tr.Load(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	before := tr.Snapshot()
	config.Cues = cueOptions()
	if err := tr.Load(context.Background(), config); err == nil || !reflect.DeepEqual(before, tr.Snapshot()) || len(f.generations) != 1 {
		t.Fatal("unsupported cue host mutated state")
	}
}

func TestCueGateReportsPhaseCompletionAndDeviceCannotMutateCoreClicks(t *testing.T) {
	schedule := Schedule{Generation: 1, Sequence: 1, Source: "source", StartFrame: 0, EndFrame: 0, OutputStart: 0, OutputEnd: 100, Speed: 1, Phase: "rest"}
	gate, err := NewTraversal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := gate.Acknowledge(consumed(schedule, 100))
	if err != nil || completion.Type != "phase_completed" || completion.Phase != "rest" {
		t.Fatal(completion, err)
	}
	tr, f := setupCues(t)
	command(t, tr, "play", nil)
	first := f.schedules[0]
	f.schedules[0].Clicks[0].OutputFrame = 500
	f.acks = []OutputAcknowledgement{consumed(first, 100)}
	command(t, tr, "pause", nil)
	command(t, tr, "play", nil)
	if f.schedules[len(f.schedules)-2].Clicks[0].OutputFrame != 0 {
		t.Fatal("device mutated the core's resumed click schedule")
	}
}
