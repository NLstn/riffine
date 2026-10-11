// Package project defines the portable, versioned practice document.
package project

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/NLstn/riffine/core/audio"
)

const Version = 3

type Loop struct {
	Enabled bool    `json:"enabled"`
	Start   float64 `json:"start"`
	End     float64 `json:"end"`
}
type Settings struct {
	Speed float64 `json:"speed"`
	Pitch float64 `json:"pitch"`
	Loop  Loop    `json:"loop"`
}
type Track struct {
	Path  string     `json:"path"`
	Audio audio.Info `json:"audio"`
}
type Project struct {
	Version       int            `json:"version"`
	Name          string         `json:"name"`
	Track         Track          `json:"track"`
	Settings      Settings       `json:"settings"`
	Annotations   Annotations    `json:"annotations"`
	PracticePlans []PracticePlan `json:"practicePlans,omitempty"`
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func (p Project) Validate() error {
	if p.Version != Version {
		return fmt.Errorf("unsupported project version %d (supported: %d)", p.Version, Version)
	}
	if p.Name == "" || p.Track.Path == "" {
		return errors.New("name and track path are required")
	}
	i := p.Track.Audio
	if i.SampleRate < 8000 || i.SampleRate > 192000 || i.Channels < 1 || i.Channels > 2 || i.Frames <= 0 || i.Frames > int64(^uint32(0))/int64(i.Channels*2) || (i.Bits != 16 && i.Bits != 24 && i.Bits != 32) {
		return errors.New("invalid track metadata")
	}
	h, err := hex.DecodeString(i.SHA256)
	if err != nil || len(h) != 32 {
		return errors.New("invalid audio fingerprint")
	}
	s := p.Settings
	if !finite(s.Speed) || s.Speed < 0.25 || s.Speed > 2 {
		return errors.New("speed must be 0.25–2")
	}
	if !finite(s.Pitch) || s.Pitch < -12 || s.Pitch > 12 {
		return errors.New("pitch must be -12–12 semitones")
	}
	l := s.Loop
	if !finite(l.Start) || !finite(l.End) || l.Start < 0 || l.End < l.Start || l.End > i.Duration() {
		return errors.New("loop must be within track duration")
	}
	if l.Enabled && (l.End-l.Start)*float64(i.SampleRate) < 1 {
		return errors.New("enabled loop must contain at least one frame")
	}
	if err := p.Annotations.validate(i.Duration(), i.SampleRate); err != nil {
		return err
	}
	return p.validatePlans()
}

// New builds a version-3 document. Reference is a host-produced locator;
// content identity is the fingerprint and metadata, never the locator.
func New(name, reference string, info audio.Info) (Project, error) {
	info.DataOffset, info.DataBytes = 0, 0 // Decoder cursor details are not document metadata.
	p := Project{Version: Version, Name: name, Track: Track{Path: reference, Audio: info}, Settings: Settings{Speed: 1, Loop: Loop{End: info.Duration()}}}
	return p, p.Validate()
}

const MaxDocumentBytes = 1 << 20

// Decode is storage-neutral, strict and bounded. It does not close the reader.
func Decode(r io.Reader) (Project, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return Project{}, err
	}
	if len(b) > MaxDocumentBytes {
		return Project{}, errors.New("project exceeds 1 MiB")
	}
	var header struct {
		Version int `json:"version"`
	}
	if err = json.Unmarshal(b, &header); err != nil {
		return Project{}, err
	}
	if header.Version != 1 && header.Version != 2 && header.Version != Version {
		return Project{}, fmt.Errorf("unsupported project version %d (supported: 1, 2 and %d)", header.Version, Version)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var p Project
	if header.Version == 1 {
		// A distinct schema prevents v2 fields from being accepted under a v1 label.
		var old struct {
			Version  int      `json:"version"`
			Name     string   `json:"name"`
			Track    Track    `json:"track"`
			Settings Settings `json:"settings"`
		}
		if err = dec.Decode(&old); err != nil {
			return Project{}, err
		}
		p = Project{Version: Version, Name: old.Name, Track: old.Track, Settings: old.Settings}
		if (old.Settings.Loop.End-old.Settings.Loop.Start)*float64(old.Track.Audio.SampleRate) >= 1 {
			p.Annotations.Regions = []Region{{ID: "region-v1-loop", Label: "Saved loop", Start: old.Settings.Loop.Start, End: old.Settings.Loop.End}}
			p.Annotations.Loops = []NamedLoop{{ID: "loop-v1", Label: "Saved loop", RegionID: "region-v1-loop"}}
		}
	} else if header.Version == 2 {
		var old struct {
			Version     int         `json:"version"`
			Name        string      `json:"name"`
			Track       Track       `json:"track"`
			Settings    Settings    `json:"settings"`
			Annotations Annotations `json:"annotations"`
		}
		if err = dec.Decode(&old); err != nil {
			return Project{}, err
		}
		p = Project{Version: Version, Name: old.Name, Track: old.Track, Settings: old.Settings, Annotations: old.Annotations}
	} else {
		if err = dec.Decode(&p); err != nil {
			return Project{}, err
		}
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return Project{}, errors.New("unexpected trailing project data")
	}
	return p, p.Validate()
}

func Encode(p Project) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b)+1 > MaxDocumentBytes {
		return nil, errors.New("project exceeds 1 MiB")
	}
	return append(b, '\n'), nil
}

func (p Project) VerifyIdentity(i audio.Info) error {
	old := p.Track.Audio
	if i.SHA256 != old.SHA256 || i.Frames != old.Frames || i.SampleRate != old.SampleRate || i.Channels != old.Channels || i.Bits != old.Bits {
		return errors.New("audio does not match project fingerprint/metadata; relink the original file")
	}
	return nil
}
