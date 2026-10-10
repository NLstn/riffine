package project

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/NLstn/riffine/core/audio"
)

func fixture(t *testing.T) (Project, string, string) {
	t.Helper()
	dir := t.TempDir()
	wav := filepath.Join(dir, "guitar.wav")
	path := filepath.Join(dir, "song.riffine.json")
	if err := audio.WriteTone(wav, 1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	p, err := Import(wav, path)
	if err != nil {
		t.Fatal(err)
	}
	return p, path, wav
}

func TestRoundTripAndRebase(t *testing.T) {
	p, path, wav := fixture(t)
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = loaded.VerifyAudio(path); err != nil {
		t.Fatal(err)
	}
	if err = Save(path, p); err == nil {
		t.Fatal("overwrote project")
	}
	dir := filepath.Join(filepath.Dir(path), "elsewhere")
	os.Mkdir(dir, 0700)
	dest := filepath.Join(dir, "saved.json")
	if err = p.Rebase(path, dest); err != nil {
		t.Fatal(err)
	}
	if p.AudioPath(dest) != wav {
		t.Fatalf("lost source: %s", p.AudioPath(dest))
	}
	if err = Save(dest, p); err != nil {
		t.Fatal(err)
	}
	if err = p.VerifyAudio(dest); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidSettings(t *testing.T) {
	base, _, _ := fixture(t)
	cases := []func(*Project){func(p *Project) { p.Version = 2 }, func(p *Project) { p.Settings.Speed = 0 }, func(p *Project) { p.Settings.Pitch = 13 }, func(p *Project) { p.Settings.Speed = math.NaN() }, func(p *Project) { p.Settings.Loop = Loop{true, .8, .2} }, func(p *Project) { p.Settings.Loop = Loop{true, 0, 2} }, func(p *Project) { p.Settings.Loop = Loop{true, .5, .5} }, func(p *Project) { p.Track.Audio.SHA256 = "bad" }}
	for i, mutate := range cases {
		p := base
		mutate(&p)
		if p.Validate() == nil {
			t.Errorf("accepted case %d", i)
		}
	}
}

func TestChangedAudioAndStrictJSON(t *testing.T) {
	p, path, wav := fixture(t)
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(wav)
	b[len(b)-1] ^= 1
	os.WriteFile(wav, b, 0600)
	if p.VerifyAudio(path) == nil {
		t.Fatal("accepted modified source")
	}
	for _, data := range []string{`{"version":99}`, `{} {}`, `{"surprise":true}`} {
		bad := filepath.Join(t.TempDir(), "bad.json")
		os.WriteFile(bad, []byte(data), 0600)
		if _, err := Load(bad); err == nil {
			t.Fatal("accepted malformed project")
		}
	}
}

func TestRelinkSameContent(t *testing.T) {
	p, path, wav := fixture(t)
	renamed := filepath.Join(filepath.Dir(wav), "renamed.wav")
	if err := os.Rename(wav, renamed); err != nil {
		t.Fatal(err)
	}
	if err := p.Relink(renamed, path); err != nil {
		t.Fatal(err)
	}
	if err := p.VerifyAudio(path); err != nil {
		t.Fatal(err)
	}
}
