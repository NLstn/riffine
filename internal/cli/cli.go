package cli

import (
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
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
)

const Help = `Riffine — local guitar practice core (working name)

  riffine tone     --out tone.wav [--seconds 3 --hz 440]
  riffine import   --audio song.wav --out song.riffine.json
  riffine inspect  --project song.riffine.json [--verify]
  riffine set      --project song.riffine.json --out slow.riffine.json [--speed 0.75 --pitch -2]
  riffine loop     --project slow.riffine.json --out loop.riffine.json --start 1 --end 3 [--enabled=true]
  riffine relink   --project song.riffine.json --audio relocated.wav --out relinked.riffine.json
  riffine waveform --project song.riffine.json [--bins 1000 --out song.peaks.json]
  riffine render   --project song.riffine.json --out practice.wav
  riffine play     --project song.riffine.json
  riffine preview  --project song.riffine.json [--silent]
  riffine doctor

Flags precede no positional arguments. Existing output files are never replaced.
Render processes the enabled loop once; play repeats it until Ctrl+C.
Play renders a temporary WAV first. Speed/pitch changes are offline in this slice.
Preview streams the selected range once, with fixed speed/pitch; --silent uses a null device.
Only PCM 16/24/32-bit mono/stereo WAV import is currently supported.
`

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := io.WriteString(out, Help)
		return err
	}
	command := args[0]
	f := flag.NewFlagSet(command, flag.ContinueOnError)
	f.SetOutput(errOut)
	var source, dest, proj string
	var verify, silent bool
	var speed, pitch, start, end, seconds, hz float64
	var enabled bool
	var bins int
	switch command {
	case "import", "relink":
		f.StringVar(&source, "audio", "", "local source WAV")
		f.StringVar(&dest, "out", "", "new project path")
		if command == "relink" {
			f.StringVar(&proj, "project", "", "existing project")
		}
	case "inspect":
		f.StringVar(&proj, "project", "", "project path")
		f.BoolVar(&verify, "verify", false, "verify source fingerprint")
	case "set":
		f.StringVar(&proj, "project", "", "project path")
		f.StringVar(&dest, "out", "", "new project path")
		f.Float64Var(&speed, "speed", 1, "source/output time ratio, 0.25–2")
		f.Float64Var(&pitch, "pitch", 0, "semitones, -12–12")
	case "loop":
		f.StringVar(&proj, "project", "", "project path")
		f.StringVar(&dest, "out", "", "new project path")
		f.Float64Var(&start, "start", 0, "source seconds")
		f.Float64Var(&end, "end", 0, "source seconds")
		f.BoolVar(&enabled, "enabled", true, "repeat selected region")
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
	if command != "doctor" && command != "tone" && command != "import" && proj == "" {
		return errors.New("--project is required")
	}
	switch command {
	case "tone", "import", "relink", "set", "loop", "render":
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
	service := practice.Service{Resources: filesystem.Host{}, Store: filesystem.Host{}, Decoder: audio.WAVDecoder{}}
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
		service.Renderer = adapter
		service.Previewer = adapter
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
	case "set":
		if !provided["speed"] && !provided["pitch"] {
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
	return session.SaveAs(ctx, dest)
}
