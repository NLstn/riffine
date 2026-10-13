package practice

import (
	"context"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/ladder"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/timing"
)

func savedPlanSession(t *testing.T) (*Session, *sessionDevice) {
	t.Helper()
	s, _, d, _ := liveSession(t)
	annotations := project.Annotations{Regions: []project.Region{{ID: "phrase", Label: "Phrase", Start: .25, End: .75}}}
	dispatchLive(t, s, Command{Type: "set_annotations", Annotations: &annotations})
	plans := []project.PracticePlan{{ID: "ladder", Label: "60–100", RegionID: "phrase", Ladder: ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}}}
	dispatchLive(t, s, Command{Type: "set_practice_plans", PracticePlans: &plans})
	if err := s.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, d
}
func TestSavedPlanSelectionRunsThirtySixAcknowledgedTraversals(t *testing.T) {
	s, d := savedPlanSession(t)
	saved := s.Snapshot().Project
	id := "ladder"
	snapshot := dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
	if snapshot.ActivePracticePlan == nil || snapshot.ActivePracticePlan.ID != id || snapshot.Transport.Plan.State != "paused" || snapshot.Transport.Config.LoopStart != 12000 || snapshot.Transport.Config.LoopEnd != 36000 || snapshot.Transport.Plan.TotalSteps != 9 || snapshot.Transport.Plan.TotalLoops != 36 || d.running {
		t.Fatalf("selected: %+v", snapshot)
	}
	snapshot.ActivePracticePlan.Label = "mutated snapshot"
	if s.Snapshot().ActivePracticePlan.Label != "60–100" {
		t.Fatal("snapshot aliases selected metadata")
	}
	dispatchLive(t, s, Command{Type: "play"})
	for ordinal := 1; ordinal <= 36; ordinal++ {
		if len(d.schedules) < ordinal {
			t.Fatalf("missing schedule %d", ordinal)
		}
		schedule := d.schedules[ordinal-1]
		expected := .6 + float64((ordinal-1)/4)*.05
		if schedule.StartFrame != 12000 || schedule.EndFrame != 36000 || schedule.Speed < expected-1e-12 || schedule.Speed > expected+1e-12 {
			t.Fatalf("traversal %d: %+v", ordinal, schedule)
		}
		d.pending = []engine.OutputAcknowledgement{{Generation: schedule.Generation, Sequence: schedule.Sequence, ConsumedOutputFrame: schedule.OutputEnd}}
		current, err := s.PollTransport(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if current.Transport.CompletedLoops != uint64(ordinal) || current.Transport.Plan.CompletedLoops != uint64(ordinal) {
			t.Fatalf("counts at %d: %+v", ordinal, current.Transport)
		}
	}
	snapshot = s.Snapshot()
	if snapshot.State != "stopped" || snapshot.Transport.Plan.State != "completed" || len(d.schedules) != 36 || !reflect.DeepEqual(snapshot.Project, saved) {
		t.Fatalf("completed: %+v schedules=%d", snapshot, len(d.schedules))
	}
	dispatchLive(t, s, Command{Type: "clear_practice_plan"})
	snapshot = s.Snapshot()
	if snapshot.ActivePracticePlan != nil || snapshot.Transport.Plan != nil || snapshot.Transport.Config.LoopEnabled || snapshot.Transport.Config.Speed != saved.Settings.Speed || !reflect.DeepEqual(snapshot.Project, saved) {
		t.Fatal("clear did not restore saved manual settings")
	}
}
func TestSavedPlanRejectsUnknownIDsAndUnexpectedFieldsWithoutDeviceChanges(t *testing.T) {
	s, d := savedPlanSession(t)
	before := s.Snapshot()
	generation := d.generation
	id := "missing"
	for _, command := range []Command{{Type: "select_practice_plan", PlanID: &id}, {Type: "select_practice_plan"}, {Type: "resume_practice_plan"}, {Type: "clear_practice_plan"}, {Type: "stop", PlanID: &id}} {
		command.Version, command.Sequence = APIVersion, before.Sequence+1
		_, err := s.Dispatch(context.Background(), command)
		errorCode(t, err, InvalidCommand)
		if !reflect.DeepEqual(s.Snapshot(), before) || d.generation != generation {
			t.Fatal("invalid plan command changed state")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	id = "ladder"
	_, err := s.Dispatch(ctx, Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "select_practice_plan", PlanID: &id})
	errorCode(t, err, Cancelled)
	if !reflect.DeepEqual(s.Snapshot(), before) || d.generation != generation {
		t.Fatal("cancelled selection changed state")
	}
}
func TestSavedPlanManualSpeedPausesAndExplicitResumeUsesCoreStep(t *testing.T) {
	s, d := savedPlanSession(t)
	id := "ladder"
	dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
	dispatchLive(t, s, Command{Type: "play"})
	speed := .8
	snapshot := dispatchLive(t, s, Command{Type: "set_parameters", Speed: &speed})
	if snapshot.Transport.Plan.State != "paused" || snapshot.Transport.Config.Speed != speed {
		t.Fatal("manual speed did not pause the ladder")
	}
	snapshot = dispatchLive(t, s, Command{Type: "resume_practice_plan"})
	if snapshot.Transport.Plan.State != "running" || snapshot.Transport.Config.Speed != .6 || !d.running || snapshot.Project.Settings.Speed != speed {
		t.Fatal("resume did not restore the core step while keeping saved manual speed")
	}
	d.failReset = true
	before := s.Snapshot()
	_, err := s.Dispatch(context.Background(), Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "select_practice_plan", PlanID: &id})
	errorCode(t, err, HostFailure)
	if s.Snapshot().Sequence != before.Sequence || s.Snapshot().State != "error" {
		t.Fatal("failed selection consumed a sequence or hid error")
	}
	d.failReset = false
	dispatchLive(t, s, Command{Type: "recover"})
	if s.Snapshot().ActivePracticePlan != nil || s.Snapshot().Transport.Plan != nil {
		t.Fatal("recovery retained runtime plan")
	}
}

func TestSavedPlanBoundariesPreserveSuspensionCompletions(t *testing.T) {
	for _, operation := range []string{"select_practice_plan", "clear_practice_plan"} {
		t.Run(operation, func(t *testing.T) {
			s, d := savedPlanSession(t)
			id := "ladder"
			dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
			dispatchLive(t, s, Command{Type: "play"})
			s.DrainEvents()
			first := d.schedules[0]
			d.pending = []engine.OutputAcknowledgement{{Generation: first.Generation, Sequence: first.Sequence, ConsumedOutputFrame: first.OutputEnd}}
			command := Command{Type: operation}
			if operation == "select_practice_plan" {
				command.PlanID = &id
			}
			current := dispatchLive(t, s, command)
			if current.Transport.CompletedLoops != 0 {
				t.Fatal("new run retained counters")
			}
			batch := s.DrainEvents()
			completions := 0
			for _, event := range batch.Events {
				if event.Type == "traversal_completed" {
					completions++
					if event.Transport.Generation != first.Generation || event.Transport.Sequence != first.Sequence || event.Transport.Speed != .6 {
						t.Fatal(event)
					}
				}
			}
			if completions != 1 || batch.Dropped != 0 {
				t.Fatalf("boundary lost completion: %+v", batch)
			}
		})
	}
}

func TestSavedPlanDefinitionEditsDoNotMutateCurrentRun(t *testing.T) {
	s, _ := savedPlanSession(t)
	id := "ladder"
	dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
	updated := s.Snapshot().Project.PracticePlans
	updated[0].Label = "New name"
	updated[0].Ladder.TargetSpeed = .8
	updated[0].Ladder.LoopsPerStep = 1
	current := dispatchLive(t, s, Command{Type: "set_practice_plans", PracticePlans: &updated})
	if current.ActivePracticePlan.Label != "60–100" || current.Transport.Plan.TotalLoops != 36 || current.Project.PracticePlans[0].Label != "New name" {
		t.Fatal(current)
	}
	current = dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
	if current.ActivePracticePlan.Label != "New name" || current.Transport.Plan.TotalLoops != 5 {
		t.Fatal(current)
	}
}

func TestCuePlanSnapshotOwnershipAndPresentedPhases(t *testing.T) {
	s, d := savedPlanSession(t)
	plans := s.Snapshot().Project.PracticePlans
	plans[0].Cues = &timing.CueDefinition{Version: 1, CountInBeats: 4, RestSeconds: 2, ClickLevel: .7, Grid: &timing.Definition{Version: 1, SourceBPM: 120, BeatsPerBar: 4}}
	dispatchLive(t, s, Command{Type: "set_practice_plans", PracticePlans: &plans})
	id := "ladder"
	snapshot := dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
	snapshot.ActivePracticePlan.Cues.Grid.SourceBPM = 300
	if s.Snapshot().ActivePracticePlan.Cues.Grid.SourceBPM != 120 {
		t.Fatal("active cue metadata aliases snapshot")
	}
	dispatchLive(t, s, Command{Type: "play"})
	count := d.schedules[0]
	if count.Phase != "count_in" || len(count.Clicks) != 4 {
		t.Fatal(count)
	}
	d.pending = []engine.OutputAcknowledgement{{Generation: count.Generation, Sequence: count.Sequence, ConsumedOutputFrame: count.OutputEnd}}
	snapshot, err := s.PollTransport(context.Background())
	if err != nil || snapshot.Transport.CompletedLoops != 0 || snapshot.Transport.Plan.CompletedLoops != 0 {
		t.Fatal(snapshot, err)
	}
	audio := d.schedules[1]
	d.pending = []engine.OutputAcknowledgement{{Generation: audio.Generation, Sequence: audio.Sequence, ConsumedOutputFrame: audio.OutputEnd}}
	snapshot, err = s.PollTransport(context.Background())
	if err != nil || snapshot.Transport.CompletedLoops != 1 || snapshot.Transport.Phase != "rest" {
		t.Fatal(snapshot, err)
	}
	if snapshot.Transport.PhaseRemainingFrames != int64(snapshot.Transport.Config.OutputRate)*2 {
		t.Fatal("fixed seconds were not propagated", snapshot)
	}
	if snapshot.Project.PracticePlans[0].Cues.RestSeconds != 2 {
		t.Fatal("runtime changed saved cues")
	}
}

type noCueDevice struct{ LiveDevice }
type restOnlyDevice struct{ LiveDevice }

func (restOnlyDevice) SupportsCues() bool { return true }

type limitedCueFactory struct {
	delegate LiveDeviceFactory
	restOnly bool
}

func (f limitedCueFactory) OpenLive(ctx context.Context, source VerifiedSource) (LiveDevice, error) {
	d, err := f.delegate.OpenLive(ctx, source)
	if err != nil {
		return nil, err
	}
	if f.restOnly {
		return restOnlyDevice{d}, nil
	}
	return noCueDevice{d}, nil
}
func TestCueCapabilityFailureKeepsSessionAndDeviceUntouched(t *testing.T) {
	for _, restOnly := range []bool{false, true} {
		s, _, d, f := liveSession(t)
		s.service.LiveDevice = limitedCueFactory{delegate: f, restOnly: restOnly}
		annotations := project.Annotations{Regions: []project.Region{{ID: "phrase", Label: "Phrase", Start: .25, End: .75}}}
		dispatchLive(t, s, Command{Type: "set_annotations", Annotations: &annotations})
		plans := []project.PracticePlan{{ID: "ladder", Label: "Cued ladder", RegionID: "phrase", Ladder: ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}, Cues: &timing.CueDefinition{Version: 1, CountInBeats: 4, RestSeconds: 2, ClickLevel: .7, Metronome: true, Grid: &timing.Definition{Version: 1, SourceBPM: 120, BeatsPerBar: 4}}}}
		dispatchLive(t, s, Command{Type: "set_practice_plans", PracticePlans: &plans})
		if err := s.AttachTransport(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := s.Snapshot()
		generation := d.generation
		id := "ladder"
		_, err := s.Dispatch(context.Background(), Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "select_practice_plan", PlanID: &id})
		errorCode(t, err, UnsupportedCapability)
		if !reflect.DeepEqual(s.Snapshot(), before) || d.generation != generation || d.running {
			t.Fatal("unsupported cue plan changed session or device")
		}
	}
}
