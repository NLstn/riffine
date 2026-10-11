package project

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/NLstn/riffine/core/ladder"
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
		Track       Track       `json:"track"`
		Settings    Settings    `json:"settings"`
		Annotations Annotations `json:"annotations"`
	}{2, p.Name, p.Track, p.Settings, p.Annotations}
	data, _ := json.Marshal(old)
	migrated, e := Decode(bytes.NewReader(data))
	if e != nil || migrated.Version != 3 || !reflect.DeepEqual(migrated.Annotations, p.Annotations) || migrated.Track != p.Track || migrated.Settings != p.Settings {
		t.Fatal(migrated, e)
	}
	if len(migrated.PracticePlans) != 0 {
		t.Fatal("invented plan during migration")
	}
}
