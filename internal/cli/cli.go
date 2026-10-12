package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/adapters/native"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/waveform"
)

const Help = `Riffine — local guitar practice core (working name)

  riffine scan     --sources local-files.json
  riffine tone     --out tone.wav [--seconds 3 --hz 440]
  riffine import   --audio song.wav --out song.riffine.json
  riffine inspect  --project song.riffine.json [--verify]
  riffine set      --project song.riffine.json --out slow.riffine.json [--speed 0.75 --pitch -2]
  riffine loop     --project slow.riffine.json --out loop.riffine.json --start 1 --end 3 [--enabled=true]
  riffine relink   --project song.riffine.json --audio relocated.wav --out relinked.riffine.json
  riffine save     --project song.riffine.json [--speed 0.75 --pitch -2]
  riffine plans    --project annotated.json --out planned.json --plans ladder.json
  riffine plan     --project planned.json --id ladder
  riffine annotate --project song.riffine.json --out annotated.riffine.json --annotations notes.json
  riffine waveform-detail --project song.riffine.json --start-frame 1234 --end-frame 1240 [--bins 1000]
  riffine waveform-window --project song.riffine.json [--start-frame 0 --end-frame 48000 --bins 1000 --cache DIR]
  riffine waveform --project song.riffine.json [--bins 1000 --out song.peaks.json]
  riffine render   --project song.riffine.json --out practice.wav
  riffine play     --project song.riffine.json
  riffine preview  --project song.riffine.json [--silent]
  riffine doctor

Flags precede no positional arguments. Outputs are exclusive; save updates a verified project revision with recovery.
Render processes the enabled loop once; play repeats it until Ctrl+C.
Play renders a temporary WAV first. Speed/pitch changes are offline in this slice.
Preview streams the selected range once, with fixed speed/pitch; --silent uses a null device.
WAV PCM works in Go; FLAC/MP3 require a helper with tested decoder capabilities.
`

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := io.WriteString(out, Help)
		return err
	}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(errOut)
	var source, dest, proj, annotationFile, plansFile, planID, cacheDir string
	var startFrame, endFrame int64
	var verify, silent bool
	var speed, pitch, start, end, seconds, hz float64
	var enabled bool
	var bins int
	switch command {
	case "scan":
		f.StringVar(&source, "sources", "", "bounded JSON list of local source locators")
	case "import", "relink":
		f.StringVar(&source, "audio", "", "local source audio")
		f.StringVar(&dest, "out", "", "new project path")
		if command == "relink" {
			f.StringVar(&proj, "project", "", "existing project")
		}
	case "inspect":
		f.StringVar(&proj, "project", "", "project path")
		f.BoolVar(&verify, "verify", false, "verify source fingerprint")
	case "set", "save":
		f.StringVar(&proj, "project", "", "project path")
		if command == "set" {
			f.StringVar(&dest, "out", "", "new project path")
		}
		f.Float64Var(&speed, "speed", 1, "source/output time ratio, 0.25–2")
		f.Float64Var(&pitch, "pitch", 0, "semitones, -12–12")
	case "loop":
		f.StringVar(&proj, "project", "", "project path")
		f.StringVar(&dest, "out", "", "new project path")
		f.Float64Var(&start, "start", 0, "source seconds")
		f.Float64Var(&end, "end", 0, "source seconds")
		f.BoolVar(&enabled, "enabled", true, "repeat selected region")
	case "plans":
		f.StringVar(&proj, "project", "", "existing annotated project")
		f.StringVar(&dest, "out", "", "new project with practice plans")
		f.StringVar(&plansFile, "plans", "", "practice plan definitions JSON file")
	case "plan":
		f.StringVar(&proj, "project", "", "project with a saved practice plan")
		f.StringVar(&planID, "id", "", "practice plan ID to inspect")
	case "annotate":
		f.StringVar(&proj, "project", "", "existing project")
		f.StringVar(&dest, "out", "", "new annotated project")
		f.StringVar(&annotationFile, "annotations", "", "portable annotations JSON file")
	case "waveform-window", "waveform-detail":
		f.StringVar(&proj, "project", "", "project path")
		if command == "waveform-window" {
			f.StringVar(&cacheDir, "cache", "", "optional dedicated waveform cache directory")
		}
		f.Int64Var(&startFrame, "start-frame", 0, "first source frame")
		f.Int64Var(&endFrame, "end-frame", 0, "exclusive source end frame (default full track)")
		f.IntVar(&bins, "bins", 1000, "maximum returned peak cells, 1–4096")
	case "waveform":
		f.StringVar(&proj, "project", "", "project path")
		f.StringVar(&dest, "out", "", "optional output JSON (default stdout)")
		f.IntVar(&bins, "bins", 1000, "peak bins per channel")
	case "render":
		f.StringVar(&proj, "project", "", "project path")
		f.StringVar(&dest, "out", "", "new rendered WAV")
	case "play", "preview":
		f.StringVar(&proj, "project", "", "project path")
		if command == "preview" {
			f.BoolVar(&silent, "silent", false, "use an explicit null output device for diagnostics")
		}
	case "tone":
		f.StringVar(&dest, "out", "", "new tone WAV")
		f.Float64Var(&seconds, "seconds", 3, "duration up to 60 seconds")
		f.Float64Var(&hz, "hz", 440, "frequency")
	case "doctor":
	default:
		return fmt.Errorf("unknown command %q; use riffine help", command)
	}
	if err := f.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return errors.New("unexpected positional arguments; use named flags")
	}
	provided := map[string]bool{}
	f.Visit(func(x *flag.Flag) { provided[x.Name] = true })
	if command != "scan" && command != "doctor" && command != "tone" && command != "import" && proj == "" {
		return errors.New("--project is required")
	}
	switch command {
	case "tone", "import", "relink", "set", "loop", "render", "annotate", "plans":
		if dest == "" {
			return errors.New("--out is required")
		}
	}
	if (command == "import" || command == "relink") && source == "" {
		return errors.New("--audio is required")
	}
	writeJSON := func(v any) error { enc := json.NewEncoder(out); enc.SetIndent("", "  "); return enc.Encode(v) }
	if command == "doctor" {
		path, err := native.Find()
		if err != nil {
			return err
		}
		if err = (native.Adapter{Executable: path, Stderr: errOut}).Check(ctx); err != nil {
			return err
		}
		return writeJSON(map[string]any{"nativeHelper": path, "projectVersion": project.Version, "mode": "offline-render-then-play"})
	}
	if command == "tone" {
		return filesystem.WriteTone(dest, seconds, hz, 48000, 1)
	}
	if command == "scan" {
		return scanSources(ctx, source, out)
	}
	service := practice.Service{Resources: filesystem.Host{}, Store: filesystem.Host{}, Decoder: localDecoder(ctx, errOut)}
	if command == "import" {
		session, err := service.Import(ctx, source, dest)
		if err != nil {
			return err
		}
		return session.SaveAs(ctx, dest)
	}
	var adapter native.Adapter
	if command == "render" || command == "play" || command == "preview" {
		helper, err := native.Find()
		if err != nil {
			return err
		}
		adapter = native.Adapter{Executable: helper, Stderr: errOut, SilentPreview: silent}
		if command == "render" || command == "play" {
			if err := adapter.CheckSourceRender(ctx); err != nil {
				return err
			}
			service.SourceRenderer = adapter
		}
		if command == "preview" {
			if err := adapter.CheckSourcePreview(ctx); err != nil {
				return err
			}
			service.SourcePreviewer = adapter
		}
	}
	session, err := service.Open(ctx, proj)
	if err != nil {
		return err
	}
	p := session.Snapshot().Project
	var change *practice.Command
	switch command {
	case "inspect":
		if verify {
			if err = session.Verify(ctx); err != nil {
				return err
			}
		}
		return writeJSON(p)
	case "plan":
		if planID == "" {
			return errors.New("--id is required")
		}
		definition, compiled, e := p.CompilePracticePlan(planID)
		if e != nil {
			return e
		}
		return writeJSON(map[string]any{"mode": "schedule-preview", "plan": definition, "steps": compiled.Describe(), "totalLoops": compiled.TotalLoops()})
	case "plans":
		if plansFile == "" {
			return errors.New("--plans is required")
		}
		data, e := service.Store.Read(ctx, plansFile, project.MaxDocumentBytes)
		if e != nil {
			return e
		}
		plans, e := project.DecodePracticePlans(bytes.NewReader(data))
		if e != nil {
			return e
		}
		change = &practice.Command{Version: practice.APIVersion, Sequence: 1, Type: "set_practice_plans", PracticePlans: &plans}
	case "annotate":
		if annotationFile == "" {
			return errors.New("--annotations is required")
		}
		b, e := service.Store.Read(ctx, annotationFile, project.MaxDocumentBytes)
		if e != nil {
			return e
		}
		a, e := project.DecodeAnnotations(bytes.NewReader(b))
		if e != nil {
			return e
		}
		change = &practice.Command{Version: practice.APIVersion, Sequence: 1, Type: "set_annotations", Annotations: &a}
	case "set", "save":

		if !provided["speed"] && !provided["pitch"] {
			if command == "save" {
				break
			}
			return errors.New("supply --speed and/or --pitch")
		}
		change = &practice.Command{Version: practice.APIVersion, Sequence: 1, Type: "set_parameters"}
		if provided["speed"] {
			change.Speed = &speed
		}
		if provided["pitch"] {
			change.Pitch = &pitch
		}
	case "loop":
		if enabled && (!provided["start"] || !provided["end"]) {
			return errors.New("enabled loop requires --start and --end")
		}
		p.Settings.Loop.Enabled = enabled
		if provided["start"] {
			p.Settings.Loop.Start = start
		}
		if provided["end"] {
			p.Settings.Loop.End = end
		}
		change = &practice.Command{Version: practice.APIVersion, Sequence: 1, Type: "set_loop", Loop: &p.Settings.Loop}
	case "relink":
		if err = session.Relink(ctx, source); err != nil {
			return err
		}
	case "waveform-detail":
		if endFrame == 0 && !provided["end-frame"] {
			endFrame = p.Track.Audio.Frames
		}
		if startFrame < 0 || endFrame <= startFrame || endFrame > p.Track.Audio.Frames || endFrame-startFrame > waveform.MaxDetailFrames || bins < 1 || bins > waveform.MaxWindowBins {
			return errors.New("invalid detail range or bin limit; use waveform-window for wider views")
		}
		detail, e := session.OpenWaveformDetail(ctx)
		if e != nil {
			return e
		}
		defer detail.Close()
		window, e := detail.Query(ctx, startFrame, endFrame, bins)
		if e != nil {
			return e
		}
		if e = detail.Close(); e != nil {
			return e
		}
		return writeJSON(window)
	case "waveform-window":
		if endFrame == 0 && !provided["end-frame"] {
			endFrame = p.Track.Audio.Frames
		}
		if startFrame < 0 || endFrame <= startFrame || endFrame > p.Track.Audio.Frames || bins < 1 || bins > 4096 {
			return errors.New("invalid waveform window or bin limit")
		}
		var cache waveform.Cache
		if cacheDir != "" {
			cache = filesystem.PeakCache{Directory: cacheDir}
		}
		result, e := session.OpenWaveform(ctx, cache, nil)
		if e != nil {
			return e
		}
		if result.CacheError != nil {
			fmt.Fprintf(errOut, "Waveform cache unavailable: %v\n", result.CacheError)
		}
		window, e := result.Pyramid.Query(ctx, startFrame, endFrame, bins)
		if e != nil {
			return e
		}
		return writeJSON(struct {
			CacheStatus string `json:"cacheStatus"`
			waveform.Window
		}{result.CacheStatus, window})
	case "waveform":
		w, e := session.Waveform(ctx, bins)
		if e != nil {
			return e
		}
		if dest == "" {
			return writeJSON(w)
		}
		file, e := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		e = json.NewEncoder(file).Encode(w)
		closeErr := file.Close()
		if e != nil {
			os.Remove(dest)
			return e
		}
		return closeErr
	case "preview":
		fmt.Fprintln(errOut, "Verifying source, then streaming one selection. Press Ctrl+C to stop.")
		report, e := session.Preview(ctx)
		if e != nil {
			return e
		}
		return writeJSON(report)
	case "render", "play":
		if command == "render" {
			return session.Render(ctx, dest)
		}
		dir, e := os.MkdirTemp("", "riffine-play-")
		if e != nil {
			return e
		}
		defer os.RemoveAll(dir)
		wav := filepath.Join(dir, "practice.wav")
		fmt.Fprintln(errOut, "Rendering locally before playback…")
		if e = session.Render(ctx, wav); e != nil {
			return e
		}
		fmt.Fprintln(errOut, "Playing. Press Ctrl+C to stop.")
		return adapter.Play(ctx, wav, p.Settings.Loop.Enabled)
	}
	if change != nil {
		if _, err := session.Dispatch(ctx, *change); err != nil {
			return err
		}
	}
	if command == "save" {
		result, e := session.Save(ctx)
		if e != nil {
			if result.Recovery != "" {
				fmt.Fprintf(errOut, "Previous valid session recovery: %s\n", result.Recovery)
			}
			return e
		}
		return writeJSON(result)
	}
	return session.SaveAs(ctx, dest)
}
