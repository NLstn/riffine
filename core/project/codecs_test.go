package project

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestVersionThreeCodecMigration(t *testing.T) {
	data, err := os.ReadFile("testdata/v3.json")
	if err != nil {
		t.Fatal(err)
	}
	original := append([]byte(nil), data...)
	p, err := Decode(bytes.NewReader(data))
	if err != nil || p.Version != Version || p.Track.Audio.Encoding != "wav-pcm" || p.Track.Audio.Bits != 16 || len(p.PracticePlans) != 1 {
		t.Fatal(p, err)
	}
	if !bytes.Equal(data, original) || p.Settings.Speed != .6 || p.Settings.Pitch != 3 || p.Settings.Loop.Start != .25 || p.Settings.Loop.End != .75 {
		t.Fatal("migration changed original bytes/units")
	}
	encoded, err := Encode(p)
	if err != nil || !bytes.Contains(encoded, []byte(`"encoding": "wav-pcm"`)) {
		t.Fatal(err)
	}
	forged := bytes.Replace(data, []byte(`"sampleRate": 48000`), []byte(`"encoding":"mp3","sampleRate":48000`), 1)
	if _, err = Decode(bytes.NewReader(forged)); err == nil {
		t.Fatal("accepted codec under old version label")
	}
}
func TestCodecMetadataValidation(t *testing.T) {
	for _, codec := range []string{"flac", "mp3", "aac", "alac", "opus"} {
		p := fixture(t)
		p.Track.Audio.Encoding = codec
		p.Track.Audio.Bits = 0
		data, err := Encode(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := Decode(bytes.NewReader(data))
		if err != nil || got.Track.Audio != p.Track.Audio {
			t.Fatal(got, err)
		}
		wrong := p.Track.Audio
		wrong.Encoding = "wav-pcm"
		wrong.Bits = 16
		if p.VerifyIdentity(wrong) == nil {
			t.Fatal("codec change matched saved identity")
		}
		if codec == "mp3" || codec == "aac" || codec == "opus" {
			p.Track.Audio.Bits = 16
			if p.Validate() == nil {
				t.Fatalf("%s accepted fixed PCM precision", codec)
			}
		}
	}
	p := fixture(t)
	p.Track.Audio.Encoding = "mp3"
	if p.Validate() == nil {
		t.Fatal("MP3 accepted fixed PCM precision")
	}
	p.Track.Audio.Encoding = "unknown"
	if p.Validate() == nil {
		t.Fatal("unknown codec accepted")
	}
	p.Track.Audio.Encoding = ""
	if p.Validate() == nil {
		t.Fatal("v4 omitted codec accepted")
	}
	p.Track.Audio.Encoding = "mp3"
	p.Track.Audio.Bits = 0
	p.Track.Audio.Frames = 1 << 40
	p.Settings.Loop.End = 0
	if p.Validate() != nil {
		t.Fatal("compressed frames incorrectly limited by RIFF size")
	}
	if _, err := Decode(strings.NewReader(`{"version":4}`)); err == nil {
		t.Fatal("incomplete current schema accepted")
	}
}
