package project

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/NLstn/riffine/core/ladder"
	"github.com/NLstn/riffine/core/timing"
)

func TestPersistDefinitionsWithoutRuntimeProgress(t *testing.T) {
	p := annotated(t)
	p.PracticePlans = []PracticePlan{{ID: "ladder", Label: "60 to 100", RegionID: "verse", Ladder: ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}}}
	data, e := Encode(p)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Decode(bytes.NewReader(data))
	if e != nil || !reflect.DeepEqual(got, p) {
		t.Fatal(got, e)
	}
	_, compiled, e := got.CompilePracticePlan("ladder")
	if e != nil || compiled.TotalLoops() != 36 {
		t.Fatal(e)
	}
	clone := got.Clone()
	clone.PracticePlans[0].Ladder.StartSpeed = .5
	if got.PracticePlans[0].Ladder.StartSpeed != .6 {
		t.Fatal("borrowed plan storage")
	}
	if bytes.Contains(data, []byte("completedLoops")) || bytes.Contains(data, []byte("generation")) {
		t.Fatal("serialized runtime progress")
	}
	for _, mutate := range []func(*Project){func(p *Project) { p.PracticePlans[0].ID = "verse" }, func(p *Project) { p.PracticePlans[0].RegionID = "gone" }, func(p *Project) { p.PracticePlans[0].Ladder.LoopsPerStep = 0 }} {
		bad := got.Clone()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("accepted invalid plan")
		}
	}
}
func TestVersionTwoMigrationPreservesAllAnnotations(t *testing.T) {
	p := annotated(t)
	old := struct {
		Version     int         `json:"version"`
		Name        string      `json:"name"`
		Track       legacyTrack `json:"track"`
		Settings    Settings    `json:"settings"`
		Annotations Annotations `json:"annotations"`
	}{2, p.Name, legacyFixture(p.Track), p.Settings, p.Annotations}
	data, _ := json.Marshal(old)
	migrated, e := Decode(bytes.NewReader(data))
	if e != nil || migrated.Version != Version || !reflect.DeepEqual(migrated.Annotations, p.Annotations) || migrated.Track != p.Track || migrated.Settings != p.Settings {
		t.Fatal(migrated, e)
	}
	if len(migrated.PracticePlans) != 0 {
		t.Fatal("invented plan during migration")
	}
}

func TestMissingPlanPhraseReportsWhichPlanMustChange(t *testing.T) {
	p := annotated(t)
	p.PracticePlans = []PracticePlan{{ID: "ladder", Label: "Verse ladder", RegionID: "gone", Ladder: ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}}}
	err := p.Validate()
	if err == nil || !strings.Contains(err.Error(), `practice plan "ladder" references missing phrase "gone"`) || !strings.Contains(err.Error(), "remove or change that plan") {
		t.Fatal(err)
	}
}

func TestCuePlansRoundTripStrictMigrationAndOwnership(t *testing.T) {
	p := annotated(t)
	p.PracticePlans = []PracticePlan{{ID: "ladder", Label: "Verse", RegionID: "verse", Ladder: ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}, Cues: &timing.CueDefinition{Version: 1, CountInBeats: 4, RestSeconds: 2, ClickLevel: .7, Metronome: true, Grid: &timing.Definition{Version: 1, SourceBPM: 120, BeatsPerBar: 4}}}}
	data, err := Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(p, got) {
		t.Fatal(got, err)
	}
	copied := p.Clone()
	copied.PracticePlans[0].Cues.Grid.SourceBPM = 150
	if p.PracticePlans[0].Cues.Grid.SourceBPM != 120 {
		t.Fatal("cloned cues alias stored plan")
	}
	forged := bytes.Replace(data, []byte(`"version": 5`), []byte(`"version": 4`), 1)
	if _, err = Decode(bytes.NewReader(forged)); err == nil {
		t.Fatal("v4 accepted new cue fields")
	}
	old := p.Clone()
	old.PracticePlans[0].Cues = nil
	data, _ = Encode(old)
	data = bytes.Replace(data, []byte(`"version": 5`), []byte(`"version": 4`), 1)
	migrated, err := Decode(bytes.NewReader(data))
	if err != nil || migrated.Version != 5 || migrated.PracticePlans[0].Cues != nil {
		t.Fatal(migrated, err)
	}
	for _, change := range []func(*timing.CueDefinition){func(c *timing.CueDefinition) { c.RestSeconds = -1 }, func(c *timing.CueDefinition) { c.RestSeconds = 121 }, func(c *timing.CueDefinition) { c.Grid = nil }, func(c *timing.CueDefinition) { c.CountInBeats = 33 }} {
		bad := p.Clone()
		change(bad.PracticePlans[0].Cues)
		if err = bad.Validate(); err == nil {
			t.Fatal("invalid cue controls accepted")
		}
	}
}

func TestSavedBeatMarkersResolveInCoreAndRemainPortable(t *testing.T) {
	p := annotated(t)
	p.Annotations.Markers = []Marker{{ID: "second", Kind: "beat", Label: "Beat 2", Position: .25, Measure: 1, Beat: 2}, {ID: "first", Kind: "beat", Label: "Beat 1", Position: 0, Measure: 1, Beat: 1}, {ID: "third", Kind: "beat", Label: "Beat 3", Position: .5, Measure: 1, Beat: 3}}
	p.PracticePlans = []PracticePlan{{ID: "beat-ladder", Label: "Beat ladder", RegionID: "verse", Ladder: ladder.Definition{StartSpeed: .6, TargetSpeed: 1, IncrementPoints: 5, LoopsPerStep: 4, EndBehavior: "stop"}, Cues: &timing.CueDefinition{Version: 1, CountInBeats: 4, RestSeconds: 2, ClickLevel: .7, Metronome: true, UseBeatMarkers: true, Grid: &timing.Definition{Version: 1, BeatsPerBar: 4}}}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	resolved, err := p.ResolvePlanCues(p.PracticePlans[0])
	if err != nil || resolved.UseBeatMarkers || len(resolved.Grid.Markers) != 3 || resolved.Grid.Markers[0].SourceFrame != 0 || resolved.Grid.Markers[1].SourceFrame != 12000 || resolved.Grid.Markers[1].Ordinal != 1 {
		t.Fatal(resolved, err)
	}
	if len(p.PracticePlans[0].Cues.Grid.Markers) != 0 || !p.PracticePlans[0].Cues.UseBeatMarkers {
		t.Fatal("resolution changed saved definitions")
	}
	data, err := Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(bytes.NewReader(data))
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatal(got, err)
	}
	bad := p.Clone()
	bad.Annotations.Markers = nil
	if err = bad.Validate(); err == nil || !strings.Contains(err.Error(), "needs saved beat markers") {
		t.Fatal(err)
	}
	bad = p.Clone()
	bad.Annotations.Markers[1].Beat = 5
	if err = bad.Validate(); err == nil || !strings.Contains(err.Error(), "4-beat bar") {
		t.Fatal(err)
	}
	bad = p.Clone()
	bad.Annotations.Markers[2].Beat = 2
	if err = bad.Validate(); err == nil {
		t.Fatal("duplicate beat ordinal accepted")
	}
}
