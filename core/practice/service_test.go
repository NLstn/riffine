package practice

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
)

type recorder struct {
	request engine.RenderRequest
	called  bool
}

func (r *recorder) Render(_ context.Context, q engine.RenderRequest) error {
	r.request = q
	r.called = true
	return nil
}
func TestSelectionInSourceFrames(t *testing.T) {
	d := t.TempDir()
	wav := filepath.Join(d, "tone.wav")
	audio.WriteTone(wav, 1, 440, 48000, 1)
	path := filepath.Join(d, "p.json")
	p, _ := project.Import(wav, path)
	p.Settings.Speed = .5
	p.Settings.Pitch = 3
	p.Settings.Loop = project.Loop{Enabled: true, Start: .25, End: .75}
	if err := project.Save(path, p); err != nil {
		t.Fatal(err)
	}
	r := &recorder{}
	if err := Render(context.Background(), r, path, "out.wav"); err != nil {
		t.Fatal(err)
	}
	if !r.called || r.request.StartFrame != 12000 || r.request.EndFrame != 36000 || r.request.Speed != .5 || r.request.Pitch != 3 {
		t.Fatalf("wrong request: %+v", r.request)
	}
}
