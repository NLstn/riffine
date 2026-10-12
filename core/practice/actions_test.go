package practice

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
)

func actionSession(t *testing.T) (*Session, *sessionDevice) {
	t.Helper()
	s, _, d, _ := liveSession(t)
	if err := s.AttachTransport(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s, d
}
func acknowledgePosition(t *testing.T, s *Session, d *sessionDevice, source int64) {
	t.Helper()
	schedule := d.schedules[len(d.schedules)-1]
	for _, candidate := range d.schedules {
		if candidate.Generation == d.generation && candidate.StartFrame <= source {
			schedule = candidate
			break
		}
	}
	output := schedule.OutputStart + int64(math.Round(float64(source-schedule.StartFrame)*float64(schedule.OutputEnd-schedule.OutputStart)/float64(schedule.EndFrame-schedule.StartFrame)))
	d.pending = []engine.OutputAcknowledgement{{Generation: schedule.Generation, Sequence: schedule.Sequence, ConsumedOutputFrame: output}}
}
func TestPracticeActionsCaptureAcknowledgedBoundsAndKeepRewindPaused(t *testing.T) {
	s, d := actionSession(t)
	dispatchLive(t, s, Command{Type: "play"})
	acknowledgePosition(t, s, d, 12000)
	snapshot := dispatchLive(t, s, Command{Type: "set_a"})
	if snapshot.Project.Settings.Loop.Start != .25 || snapshot.Project.Settings.Loop.End != 1 || snapshot.Transport.SourceFrame != 12000 || snapshot.State != "playing" {
		t.Fatal(snapshot)
	}
	acknowledgePosition(t, s, d, 36000)
	snapshot = dispatchLive(t, s, Command{Type: "set_b"})
	if snapshot.Project.Settings.Loop.Start != .25 || snapshot.Project.Settings.Loop.End != .75 {
		t.Fatal(snapshot)
	}
	dispatchLive(t, s, Command{Type: "pause"})
	position := int64(20000)
	dispatchLive(t, s, Command{Type: "seek", Position: &position})
	snapshot = dispatchLive(t, s, Command{Type: "rewind_phrase"})
	if snapshot.State != "paused" || snapshot.Transport.SourceFrame != 12000 || snapshot.Transport.CompletedLoops != 0 || d.running {
		t.Fatal(snapshot)
	}
	snapshot = dispatchLive(t, s, Command{Type: "seek_forward"})
	if snapshot.Transport.SourceFrame != 35999 {
		t.Fatal("forward escaped active selection", snapshot)
	}
	snapshot = dispatchLive(t, s, Command{Type: "seek_backward"})
	if snapshot.Transport.SourceFrame != 12000 {
		t.Fatal("backward escaped active selection", snapshot)
	}
}
func TestPracticeActionsRotateSavedRegionsAndResetOnlyPracticeSettings(t *testing.T) {
	s, d := actionSession(t)
	annotations := project.Annotations{Notes: "Keep", Regions: []project.Region{{ID: "one", Label: "One", Start: .1, End: .3}, {ID: "two", Label: "Two", Start: .5, End: .9}}}
	dispatchLive(t, s, Command{Type: "set_annotations", Annotations: &annotations})
	speed, pitch := .8, 3.0
	dispatchLive(t, s, Command{Type: "set_parameters", Speed: &speed, Pitch: &pitch})
	snapshot := dispatchLive(t, s, Command{Type: "next_region"})
	if snapshot.Project.Settings.Loop.Start != .1 || snapshot.State != "stopped" || d.running {
		t.Fatal(snapshot)
	}
	dispatchLive(t, s, Command{Type: "play"})
	snapshot = dispatchLive(t, s, Command{Type: "next_region"})
	if snapshot.Project.Settings.Loop.Start != .5 || snapshot.State != "playing" || !d.running || snapshot.Project.Settings.Speed != .8 || snapshot.Project.Settings.Pitch != 3 {
		t.Fatal(snapshot)
	}
	snapshot = dispatchLive(t, s, Command{Type: "next_region"})
	if snapshot.Project.Settings.Loop.Start != .1 {
		t.Fatal("region cycle did not wrap", snapshot)
	}
	snapshot = dispatchLive(t, s, Command{Type: "practice_reset"})
	if snapshot.State != "stopped" || d.running || snapshot.Transport.SourceFrame != 0 || snapshot.Project.Settings.Speed != 1 || snapshot.Project.Settings.Pitch != 0 || snapshot.Project.Settings.Loop.Enabled || !reflect.DeepEqual(snapshot.Project.Annotations, annotations) {
		t.Fatal(snapshot)
	}
}
func TestPracticeActionsStepActualLadderSpeedAndPreserveDefinitionsOnReset(t *testing.T) {
	s, _ := savedPlanSession(t)
	id := "ladder"
	dispatchLive(t, s, Command{Type: "select_practice_plan", PlanID: &id})
	snapshot := dispatchLive(t, s, Command{Type: "speed_up"})
	if snapshot.Project.Settings.Speed != .65 || snapshot.Transport.Plan.State != "paused" || snapshot.Transport.Config.Speed != .65 {
		t.Fatal(snapshot)
	}
	snapshot = dispatchLive(t, s, Command{Type: "speed_down"})
	if snapshot.Project.Settings.Speed != .6 {
		t.Fatal(snapshot)
	}
	plans := snapshot.Project.PracticePlans
	snapshot = dispatchLive(t, s, Command{Type: "practice_reset"})
	if snapshot.ActivePracticePlan != nil || snapshot.Transport.Plan != nil || !reflect.DeepEqual(snapshot.Project.PracticePlans, plans) {
		t.Fatal(snapshot)
	}
}
func TestPracticeActionValidationAndFailedDeviceDoNotCommit(t *testing.T) {
	s, d := actionSession(t)
	before := s.Snapshot()
	speed := .9
	for _, command := range []Command{{Type: "speed_up", Speed: &speed}, {Type: "next_region"}, {Type: "set_b"}} {
		command.Version, command.Sequence = APIVersion, before.Sequence+1
		_, err := s.Dispatch(context.Background(), command)
		errorCode(t, err, InvalidCommand)
		if !reflect.DeepEqual(s.Snapshot(), before) {
			t.Fatal("invalid action changed state")
		}
	}
	for _, bound := range []float64{.25, 2} {
		dispatchLive(t, s, Command{Type: "set_parameters", Speed: &bound})
		kind := "speed_down"
		if bound == 2 {
			kind = "speed_up"
		}
		before = s.Snapshot()
		_, err := s.Dispatch(context.Background(), Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: kind})
		errorCode(t, err, InvalidCommand)
		if !reflect.DeepEqual(s.Snapshot(), before) {
			t.Fatal("speed-limit action changed state")
		}
	}
	before = s.Snapshot()
	d.failReset = true
	_, err := s.Dispatch(context.Background(), Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "practice_reset"})
	errorCode(t, err, HostFailure)
	if s.Snapshot().Sequence != before.Sequence || !reflect.DeepEqual(s.Snapshot().Project, before.Project) || s.Snapshot().State != "error" {
		t.Fatal("failed reset committed", s.Snapshot())
	}
}

func TestPracticeResetCancellationAfterLoadRequiresRecovery(t *testing.T) {
	s, d := actionSession(t)
	speed := .8
	dispatchLive(t, s, Command{Type: "set_parameters", Speed: &speed})
	before := s.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.afterReset = cancel
	snapshot, err := s.Dispatch(ctx, Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "practice_reset"})
	errorCode(t, err, Cancelled)
	if snapshot.State != "error" || snapshot.Sequence != before.Sequence || snapshot.Project.Settings.Speed != .8 {
		t.Fatal("partially installed settings hid recovery", snapshot)
	}
	d.afterReset = nil
	recovered := dispatchLive(t, s, Command{Type: "recover"})
	if recovered.State != "stopped" || recovered.Transport.Config.Speed != .8 || recovered.Project.Settings.Speed != .8 {
		t.Fatal("recovery did not restore committed settings", recovered)
	}
}

func TestPracticeActionsRequireLiveCapabilityAndRespectPreCancellation(t *testing.T) {
	s, _, _, _ := liveSession(t)
	before := s.Snapshot()
	_, err := s.Dispatch(context.Background(), Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "speed_up"})
	errorCode(t, err, UnsupportedCapability)
	if !reflect.DeepEqual(s.Snapshot(), before) {
		t.Fatal("unsupported action changed settings")
	}
	s, d := actionSession(t)
	before = s.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	generation := d.generation
	_, err = s.Dispatch(ctx, Command{Version: APIVersion, Sequence: before.Sequence + 1, Type: "practice_reset"})
	errorCode(t, err, Cancelled)
	if d.generation != generation || !reflect.DeepEqual(s.Snapshot(), before) {
		t.Fatal("pre-cancelled action touched the device")
	}
}
