// Package project defines the portable, versioned practice document.
package project

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"github.com/NLstn/riffine/core/audio"
)

const Version = 1

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
	Version  int      `json:"version"`
	Name     string   `json:"name"`
	Track    Track    `json:"track"`
	Settings Settings `json:"settings"`
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
	return nil
}

func Import(audioPath, projectPath string) (Project, error) {
	i, err := audio.Inspect(audioPath)
	if err != nil {
		return Project{}, err
	}
	abs, err := filepath.Abs(audioPath)
	if err != nil {
		return Project{}, err
	}
	dir, err := filepath.Abs(filepath.Dir(projectPath))
	if err != nil {
		return Project{}, err
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil {
		rel = abs
	}
	p := Project{Version: Version, Name: filepath.Base(audioPath), Track: Track{Path: filepath.ToSlash(rel), Audio: i}, Settings: Settings{Speed: 1, Loop: Loop{End: i.Duration()}}}
	return p, p.Validate()
}

func Load(path string) (Project, error) {
	f, err := os.Open(path)
	if err != nil {
		return Project{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Project{}, err
	}
	if st.Size() > 1<<20 {
		return Project{}, errors.New("project exceeds 1 MiB")
	}
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var p Project
	if err = dec.Decode(&p); err != nil {
		return Project{}, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return Project{}, errors.New("unexpected trailing project data")
	}
	return p, p.Validate()
}

// Save creates a new project without overwriting an existing file. Edits use a
// new destination so a failed operation never destroys the previous document.
func Save(path string, p Project) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(path)
		}
	}()
	if _, err = f.Write(append(b, '\n')); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func (p Project) AudioPath(projectPath string) string {
	if filepath.IsAbs(p.Track.Path) {
		return filepath.FromSlash(p.Track.Path)
	}
	return filepath.Join(filepath.Dir(projectPath), filepath.FromSlash(p.Track.Path))
}

func (p Project) VerifyAudio(projectPath string) error {
	i, err := audio.Inspect(p.AudioPath(projectPath))
	if err != nil {
		return fmt.Errorf("relink the source audio: %w", err)
	}
	old := p.Track.Audio
	if i.SHA256 != old.SHA256 || i.Frames != old.Frames || i.SampleRate != old.SampleRate || i.Channels != old.Channels || i.Bits != old.Bits {
		return errors.New("audio does not match project fingerprint/metadata; relink the original file")
	}
	return nil
}

// Rebase preserves the source reference when saving a project elsewhere.
func (p *Project) Rebase(from, to string) error {
	source, err := filepath.Abs(p.AudioPath(from))
	if err != nil {
		return err
	}
	dir, err := filepath.Abs(filepath.Dir(to))
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, source)
	if err != nil {
		rel = source
	}
	p.Track.Path = filepath.ToSlash(rel)
	return nil
}

func (p *Project) Relink(source, projectPath string) error {
	i, err := audio.Inspect(source)
	if err != nil {
		return err
	}
	if i.SHA256 != p.Track.Audio.SHA256 {
		return errors.New("replacement audio fingerprint differs")
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	p.Track.Path = abs
	return p.Rebase(projectPath, projectPath)
}
