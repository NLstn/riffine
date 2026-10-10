// Package practice provides application operations shared by all future hosts.
package practice

import (
	"context"
	"math"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
)

func Render(ctx context.Context, renderer engine.Renderer, projectPath, destination string) error {
	p, err := project.Load(projectPath)
	if err != nil {
		return err
	}
	if err = p.VerifyAudio(projectPath); err != nil {
		return err
	}
	start, end := int64(0), p.Track.Audio.Frames
	if p.Settings.Loop.Enabled {
		start = int64(math.Round(p.Settings.Loop.Start * float64(p.Track.Audio.SampleRate)))
		end = int64(math.Round(p.Settings.Loop.End * float64(p.Track.Audio.SampleRate)))
	}
	return renderer.Render(ctx, engine.RenderRequest{Source: p.AudioPath(projectPath), Destination: destination, StartFrame: start, EndFrame: end, Speed: p.Settings.Speed, Pitch: p.Settings.Pitch})
}
