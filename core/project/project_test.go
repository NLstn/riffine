package project

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/NLstn/riffine/core/audio"
)

func fixture(t *testing.T) Project {
	t.Helper()
	p, err := New("guitar.wav", "opaque:original", audio.Info{SampleRate: 48000, Channels: 1, Bits: 16, Frames: 48000, SHA256: strings.Repeat("ab", 32)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func TestRoundTrip(t *testing.T) {
	p := fixture(t)
	p.Settings.Speed, p.Settings.Pitch = .75, -2
	p.Settings.Loop = Loop{true, .1, .8}
	b, err := Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(bytes.NewReader(b))
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
}
func TestInvalidSettings(t *testing.T) {
	base := fixture(t)
	cases := []func(*Project){func(p *Project) { p.Version = 99 }, func(p *Project) { p.Settings.Speed = 0 }, func(p *Project) { p.Settings.Pitch = 13 }, func(p *Project) { p.Settings.Speed = math.NaN() }, func(p *Project) { p.Settings.Loop = Loop{true, .8, .2} }, func(p *Project) { p.Settings.Loop = Loop{true, 0, 2} }, func(p *Project) { p.Settings.Loop = Loop{true, .5, .5} }, func(p *Project) { p.Track.Audio.SHA256 = "bad" }}
	for i, mutate := range cases {
		p := base
		mutate(&p)
		if p.Validate() == nil {
			t.Errorf("accepted case %d", i)
		}
	}
}
func TestStrictAndBoundedJSON(t *testing.T) {
	for _, data := range []string{`{"version":99}`, `{} {}`, `{"surprise":true}`, strings.Repeat(" ", MaxDocumentBytes+1)} {
		if _, err := Decode(strings.NewReader(data)); err == nil {
			t.Fatal("accepted malformed/oversized document")
		}
	}
	p := fixture(t)
	p.Name = strings.Repeat("x", MaxDocumentBytes)
	if _, err := Encode(p); err == nil {
		t.Fatal("encoded an unreadable oversized document")
	}
}
func TestIdentityIncludesMetadata(t *testing.T) {
	p := fixture(t)
	if err := p.VerifyIdentity(p.Track.Audio); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*audio.Info){func(i *audio.Info) { i.SHA256 = strings.Repeat("cd", 32) }, func(i *audio.Info) { i.SampleRate = 44100 }, func(i *audio.Info) { i.Frames++ }, func(i *audio.Info) { i.Channels = 2 }, func(i *audio.Info) { i.Bits = 24 }} {
		i := p.Track.Audio
		mutate(&i)
		if p.VerifyIdentity(i) == nil {
			t.Fatal("accepted changed identity")
		}
	}
}
