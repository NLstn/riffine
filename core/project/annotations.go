package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"unicode/utf8"
)

const MaxAnnotations = 4096
const MaxNoteBytes = 16384

var annotationID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// All annotation positions remain source seconds, independent of playback speed.
// IDs are client-supplied opaque stable keys; relinking never regenerates them.
type Region struct {
	ID    string  `json:"id"`
	Label string  `json:"label"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Notes string  `json:"notes,omitempty"`
}
type Marker struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"` // section, measure or beat
	Label    string  `json:"label"`
	Position float64 `json:"position"`
	Measure  int     `json:"measure,omitempty"` // one-based ordinal, not an inferred tempo map
	Beat     int     `json:"beat,omitempty"`
	Notes    string  `json:"notes,omitempty"`
}
type NamedLoop struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	RegionID string `json:"regionId"`
}
type Bookmark struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Position float64 `json:"position"`
}
type Annotations struct {
	Notes     string      `json:"notes,omitempty"`
	Regions   []Region    `json:"regions,omitempty"`
	Markers   []Marker    `json:"markers,omitempty"`
	Loops     []NamedLoop `json:"loops,omitempty"`
	Bookmarks []Bookmark  `json:"bookmarks,omitempty"`
}

func (a Annotations) Clone() Annotations {
	a.Regions = slices.Clone(a.Regions)
	a.Markers = slices.Clone(a.Markers)
	a.Loops = slices.Clone(a.Loops)
	a.Bookmarks = slices.Clone(a.Bookmarks)
	return a
}
func (p Project) Clone() Project { p.Annotations = p.Annotations.Clone(); return p }
func (a Annotations) validate(duration float64, rate int) error {
	if !utf8.ValidString(a.Notes) || len(a.Notes) > MaxNoteBytes || len(a.Regions)+len(a.Markers)+len(a.Loops)+len(a.Bookmarks) > MaxAnnotations {
		return errors.New("annotation count or note size exceeds limit")
	}
	ids := map[string]bool{}
	regions := map[string]bool{}
	key := func(id, label, notes string) error {
		if !annotationID.MatchString(id) || ids[id] || len(label) > 1024 || len(notes) > MaxNoteBytes || !utf8.ValidString(label) || !utf8.ValidString(notes) {
			return fmt.Errorf("invalid or duplicate annotation ID/label/notes: %q", id)
		}
		ids[id] = true
		return nil
	}
	position := func(value float64) bool { return finite(value) && value >= 0 && value <= duration }
	for _, r := range a.Regions {
		if err := key(r.ID, r.Label, r.Notes); err != nil {
			return err
		}
		if !position(r.Start) || !position(r.End) || (r.End-r.Start)*float64(rate) < 1 {
			return errors.New("region must contain at least one source frame within the track")
		}
		regions[r.ID] = true
	}
	for _, m := range a.Markers {
		if err := key(m.ID, m.Label, m.Notes); err != nil {
			return err
		}
		if !position(m.Position) {
			return errors.New("marker is outside the track")
		}
		switch m.Kind {
		case "section":
			if m.Measure != 0 || m.Beat != 0 {
				return errors.New("section marker cannot specify measure/beat")
			}
		case "measure":
			if m.Measure < 1 || m.Beat != 0 {
				return errors.New("measure marker requires a positive measure only")
			}
		case "beat":
			if m.Measure < 1 || m.Beat < 1 {
				return errors.New("beat marker requires positive measure and beat")
			}
		default:
			return errors.New("unknown marker kind")
		}
	}
	for _, l := range a.Loops {
		if err := key(l.ID, l.Label, ""); err != nil {
			return err
		}
		if !regions[l.RegionID] {
			return errors.New("named loop references a missing region")
		}
	}
	for _, b := range a.Bookmarks {
		if err := key(b.ID, b.Label, ""); err != nil {
			return err
		}
		if !position(b.Position) {
			return errors.New("bookmark is outside the track")
		}
	}
	return nil
}

// DecodeAnnotations is the bounded storage-neutral CLI/binding entry point.
// Timing and reference validation happens when applied to a specific project.
func DecodeAnnotations(r io.Reader) (Annotations, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return Annotations{}, err
	}
	if len(b) > MaxDocumentBytes {
		return Annotations{}, errors.New("annotations exceed 1 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var a Annotations
	if err = dec.Decode(&a); err != nil {
		return a, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return a, errors.New("unexpected trailing annotation data")
	}
	return a, nil
}
