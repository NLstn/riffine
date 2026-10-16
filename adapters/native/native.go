// Package native implements the engine ports using an isolated native helper.
package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/resource"
)

type Adapter struct {
	Executable    string
	Stderr        io.Writer
	SilentPreview bool
}

// Check verifies that the configured helper can actually execute.
func (a Adapter) Check(ctx context.Context) error { return a.run(ctx, "version") }

func Find() (string, error) {
	if p := os.Getenv("RIFFINE_AUDIO_HELPER"); p != "" {
		return p, nil
	}
	self, err := os.Executable()
	if err == nil {
		p := filepath.Join(filepath.Dir(self), "riffine-audio")
		if filepath.Ext(self) == ".exe" {
			p += ".exe"
		}
		if st, e := os.Stat(p); e == nil && !st.IsDir() {
			return p, nil
		}
	}
	p, err := exec.LookPath("riffine-audio")
	if err != nil {
		return "", errors.New("native helper missing: build native/ and put riffine-audio beside the CLI, on PATH, or set RIFFINE_AUDIO_HELPER")
	}
	return p, nil
}
func (a Adapter) run(ctx context.Context, args ...string) error {
	if a.Executable == "" {
		return errors.New("native helper path is empty")
	}
	cmd := exec.CommandContext(ctx, a.Executable, args...)
	configureHelper(cmd)
	cmd.Stderr = a.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("audio helper: %w", err)
	}
	return nil
}
func (a Adapter) Render(ctx context.Context, r engine.RenderRequest) error {
	info, err := a.inspectSource(ctx, r.Source)
	if err != nil {
		return err
	}
	return a.renderOutput(ctx, info, r, func(tmp string) error {
		return a.run(ctx, "render", r.Source, tmp, strconv.FormatInt(r.StartFrame, 10), strconv.FormatInt(r.EndFrame, 10), strconv.FormatFloat(r.Speed, 'g', -1, 64), strconv.FormatFloat(r.Pitch, 'g', -1, 64))
	})
}

func (a Adapter) CheckSourceRender(ctx context.Context) error {
	d, err := NewDecoder(ctx, a)
	if err != nil {
		return err
	}
	if !d.borrowedRender {
		return errors.New("native source render is unsupported; rebuild the helper with borrowed-render support")
	}
	return nil
}

// RenderSource retains source identity through native teardown. Only native PCM
// reaches the encoder; output is inspected, synced and published exclusively.
func (a Adapter) RenderSource(ctx context.Context, reader resource.Reader, info audio.Info, r engine.RenderRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := audio.ValidateInfo(info); err != nil {
		return err
	}
	if err := a.CheckSourceRender(ctx); err != nil {
		return err
	}
	return a.renderOutput(ctx, info, r, func(tmp string) error {
		cmd, release, err := a.sourceCommand(ctx, reader, "render-handle", tmp, strconv.FormatInt(r.StartFrame, 10), strconv.FormatInt(r.EndFrame, 10), strconv.FormatFloat(r.Speed, 'g', -1, 64), strconv.FormatFloat(r.Pitch, 'g', -1, 64))
		if err != nil {
			return err
		}
		defer release()
		cmd.Stderr = a.Stderr
		if err = cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("audio render helper: %w", err)
		}
		return ctx.Err()
	})
}

func (a Adapter) renderOutput(ctx context.Context, info audio.Info, r engine.RenderRequest, process func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.StartFrame < 0 || r.EndFrame <= r.StartFrame || r.EndFrame > info.Frames || math.IsNaN(r.Speed) || math.IsInf(r.Speed, 0) || r.Speed < .25 || r.Speed > 2 || math.IsNaN(r.Pitch) || math.IsInf(r.Pitch, 0) || r.Pitch < -12 || r.Pitch > 12 || r.Destination == "" {
		return errors.New("invalid render request")
	}
	if _, err := os.Lstat(r.Destination); err == nil {
		return errors.New("output already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(r.Destination), ".riffine-render-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	tmp := filepath.Join(dir, "render.wav")
	if err = process(tmp); err != nil {
		return err
	}
	out, err := filesystem.Inspect(ctx, tmp)
	if err != nil {
		return fmt.Errorf("invalid rendered audio: %w", err)
	}
	want := int64(math.Round(float64(r.EndFrame-r.StartFrame) / r.Speed))
	if out.Frames != want || out.Channels != info.Channels || out.SampleRate != info.SampleRate {
		return errors.New("helper returned unexpected audio dimensions")
	}
	file, err := os.OpenFile(tmp, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Link publishes a complete file without replacing a destination created
	// during processing. Unsupported filesystems fail without a copy fallback.
	return os.Link(tmp, r.Destination)
}
func (a Adapter) Play(ctx context.Context, path string, loop bool) error {
	if _, err := filesystem.Inspect(ctx, path); err != nil {
		return err
	}
	repeat := "0"
	if loop {
		repeat = "1"
	}
	return a.run(ctx, "play", path, repeat)
}
