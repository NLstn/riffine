package project

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

func annotated(t *testing.T) Project {
	p := fixture(t)
	p.Annotations = Annotations{Notes: "Practice the pickup slowly 🎸", Regions: []Region{{ID: "verse", Label: "Verse", Start: .1, End: .8, Notes: "Observe the fingering"}}, Markers: []Marker{{ID: "section", Kind: "section", Label: "Verse", Position: .1}, {ID: "bar", Kind: "measure", Position: .1, Measure: 1}, {ID: "beat", Kind: "beat", Position: .3, Measure: 1, Beat: 2}}, Loops: []NamedLoop{{ID: "phrase", Label: "Phrase", RegionID: "verse"}}, Bookmarks: []Bookmark{{ID: "resume", Label: "Tomorrow", Position: .4}}}
	return p
}
func TestAnnotationsRoundTripAndOwnership(t *testing.T) {
	p := annotated(t)
	b, e := Encode(p)
	if e != nil {
		t.Fatal(e)
	}
	got, e := Decode(bytes.NewReader(b))
	if e != nil || !reflect.DeepEqual(p, got) {
		t.Fatal(got, e)
	}
	clone := p.Clone()
	clone.Annotations.Regions[0].Label = "changed"
	clone.Annotations.Markers[0].Position = .9
	clone.Annotations.Loops[0].RegionID = "missing"
	clone.Annotations.Bookmarks[0].Position = .9
	if !reflect.DeepEqual(p, annotated(t)) {
		t.Fatal("clone borrowed annotations")
	}
}
func TestVersionOneMigration(t *testing.T) {
	p := fixture(t)
	p.Settings.Speed = .6
	p.Settings.Pitch = 4
	p.Settings.Loop = Loop{Enabled: true, Start: .25, End: .75}
	old := struct {
		Version  int      `json:"version"`
		Name     string   `json:"name"`
		Track    Track    `json:"track"`
		Settings Settings `json:"settings"`
	}{1, p.Name, p.Track, p.Settings}
	b, _ := json.Marshal(old)
	before := append([]byte(nil), b...)
	migrated, e := Decode(bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	if migrated.Version != Version || migrated.Settings != p.Settings || migrated.Track != p.Track || len(migrated.Annotations.Regions) != 1 || migrated.Annotations.Regions[0].Start != .25 || migrated.Annotations.Loops[0].RegionID != "region-v1-loop" || !bytes.Equal(b, before) {
		t.Fatal(migrated)
	}
	again, e := Decode(bytes.NewReader(b))
	if e != nil || !reflect.DeepEqual(again, migrated) {
		t.Fatal("migration changed stable IDs", e)
	}
	old.Settings.Loop = Loop{Enabled: false, Start: 0, End: .000001}
	b, _ = json.Marshal(old)
	migrated, e = Decode(bytes.NewReader(b))
	if e != nil || migrated.Settings.Loop != old.Settings.Loop || len(migrated.Annotations.Regions) != 0 {
		t.Fatal("lost valid disabled sub-frame v1 bounds", migrated, e)
	}
	b = bytes.Replace(b, []byte(`"version":1`), []byte(`"version":99`), 1)
	if _, e = Decode(bytes.NewReader(b)); e == nil {
		t.Fatal("accepted future schema")
	}
}
func TestAnnotationsRejectInvalid(t *testing.T) {
	cases := []func(*Annotations){func(a *Annotations) { a.Regions[0].End = math.NaN() }, func(a *Annotations) { a.Markers[0].ID = "verse" }, func(a *Annotations) { a.Loops[0].RegionID = "gone" }, func(a *Annotations) { a.Bookmarks[0].Position = 2 }, func(a *Annotations) { a.Markers[1].Measure = 0 }, func(a *Annotations) { a.Markers[2].Beat = 0 }, func(a *Annotations) { a.Markers[0].Kind = "tempo" }, func(a *Annotations) { a.Regions[0].ID = "../../bad" }, func(a *Annotations) { a.Notes = strings.Repeat("x", MaxNoteBytes+1) }, func(a *Annotations) { a.Notes = string([]byte{255}) }, func(a *Annotations) { a.Regions[0].Start = .8 }, func(a *Annotations) { a.Bookmarks = make([]Bookmark, MaxAnnotations+1) }}
	for i, mutate := range cases {
		p := annotated(t)
		mutate(&p.Annotations)
		if p.Validate() == nil {
			t.Errorf("accepted invalid case %d", i)
		}
	}
}
func TestStrictAnnotationInput(t *testing.T) {
	for _, b := range []string{`{"unexpected":1}`, `{} {}`, strings.Repeat(" ", MaxDocumentBytes+1)} {
		if _, e := DecodeAnnotations(strings.NewReader(b)); e == nil {
			t.Fatal("accepted bad input")
		}
	}
}

func TestMigrationGoldenFixture(t *testing.T) {
	old, e := os.ReadFile("testdata/v1.json")
	if e != nil {
		t.Fatal(e)
	}
	expected, e := os.ReadFile("testdata/v2.json")
	if e != nil {
		t.Fatal(e)
	}
	migrated, e := Decode(bytes.NewReader(old))
	if e != nil {
		t.Fatal(e)
	}
	want, e := Decode(bytes.NewReader(expected))
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(migrated, want) {
		t.Fatal("migration differs from v2 fixture", migrated, want)
	}
}
