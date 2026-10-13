package native

import (
	"context"
	"encoding/binary"
	"errors"
	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/resource"
	"github.com/NLstn/riffine/core/timing"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func controlFixture(t *testing.T) (resource.Reader, audio.Info) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.wav")
	if err := filesystem.WriteTone(path, 1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	reader, err := (filesystem.Host{}).Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	info, err := (audio.WAVDecoder{}).Inspect(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	return reader, info
}
func serveControlFixture() {
	mode := os.Getenv("RIFFINE_CONTROL_TEST_MODE")
	generation := uint64(0)
	for call := 1; ; call++ {
		var request [128]byte
		if _, err := io.ReadFull(os.Stdin, request[:]); err != nil {
			return
		}
		if mode == "stall_open" || (mode == "stall_info" && call == 2) {
			if err := os.WriteFile(os.Getenv("RIFFINE_DECODER_TEST_SIGNAL"), []byte("started"), 0600); err != nil {
				os.Exit(1)
			}
			time.Sleep(time.Hour)
		}
		if binary.LittleEndian.Uint64(request[24:]) == 2 {
			generation = binary.LittleEndian.Uint64(request[32:])
		}
		words := [16]uint64{controlMagic, 2, binary.LittleEndian.Uint64(request[16:]), 0, 1, 48000, 48000, generation, 0, 0, 48000}
		if mode == "bad_id" {
			words[2]++
		}
		if mode == "bad_rate" {
			words[10]++
		}
		if mode == "bad_dimensions" {
			words[6]++
		}
		if mode == "bad_reserved" {
			words[15] = 1
		}
		if mode == "bad_status" {
			words[3] = 2
		}
		var response [640]byte
		for i, word := range words {
			binary.LittleEndian.PutUint64(response[i*8:], word)
		}
		if mode == "bad_error" {
			response[128] = 255
		}
		if mode == "short" {
			os.Stdout.Write(response[:127])
			return
		}
		if _, err := os.Stdout.Write(response[:]); err != nil {
			return
		}
	}
}

func TestNativeScheduledWorker(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a := Adapter{Executable: helper}
	decoder, err := NewDecoder(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, codec := range []string{"wav", "flac", "mp3"} {
		t.Run(codec, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "音-guitär."+codec)
			if codec == "wav" {
				if err := filesystem.WriteTone(path, 1, 440, 48000, 2); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(filepath.Join("../../native/testdata", "generated-stereo."+codec))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			reader, err := (filesystem.Host{}).Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			info, err := decoder.Inspect(ctx, reader)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(path, path+".original"); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			file, ok := filesystem.BorrowedFile(reader)
			if !ok {
				t.Fatal("missing original")
			}
			if _, err = file.Seek(123, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			opening, finish := context.WithCancel(ctx)
			worker, err := a.OpenScheduledWorker(opening, reader, info)
			if err != nil {
				t.Fatal(err)
			}
			defer worker.Close()
			finish() // opening context must not become the lifetime context
			pid := worker.cmd.Process.Pid
			generation := uint64(1<<53) + 17
			sequence := uint64(1<<53) + 19
			for run := 0; run < 100; run++ {
				generation++
				sequence++
				if err = worker.Reset(ctx, generation); err != nil {
					t.Fatal(err)
				}
				s := engine.Schedule{Generation: generation, Sequence: sequence, StartFrame: 1151, EndFrame: 10001, OutputStart: 0, OutputEnd: 8850, EdgeFadeFrames: 240, Speed: 1, Pitch: 3}
				if err = worker.Submit(ctx, s); err != nil {
					t.Fatal(err)
				}
				sequence++
				s.Sequence = sequence
				s.OutputStart = 8850
				s.OutputEnd = 17700
				if err = worker.Submit(ctx, s); err != nil {
					t.Fatal(err)
				}
				state, err := worker.Info(ctx)
				if err != nil || state.Generation != generation || state.LastSequence != sequence || state.Pending != 2 || state.Frames != info.Frames {
					t.Fatal(state, err)
				}
				sequence++
				s.Sequence = sequence
				s.OutputStart = 17700
				s.OutputEnd = 26550
				if err = worker.Submit(ctx, s); err == nil {
					t.Fatal("accepted third native slot")
				}
				if err = worker.Reset(ctx, generation); err == nil {
					t.Fatal("accepted stale reset")
				}
				again, err := worker.Info(ctx)
				if err != nil || again != state || worker.cmd.Process.Pid != pid {
					t.Fatal("rejection/restart changed worker", again, err)
				}
			}
			if err = worker.Reset(ctx, math.MaxUint64); err != nil {
				t.Fatal(err)
			}
			final, err := worker.Info(ctx)
			if err != nil || final.Generation != math.MaxUint64 || final.Pending != 0 {
				t.Fatal("64-bit identity was rounded", final, err)
			}
			worker.mu.Lock()
			_, err = worker.exchangeLocked(ctx, [16]uint64{3: 99})
			worker.mu.Unlock()
			if err == nil {
				t.Fatal("accepted unknown operation")
			}
			if _, err = worker.Info(ctx); err != nil {
				t.Fatal("operation rejection lost framing", err)
			}
			if err = worker.Close(); err != nil {
				t.Fatal(err)
			}
			worker.Close()
			if worker.cmd.ProcessState == nil || !worker.waited {
				t.Fatal("Close returned before child exit")
			}
			if pos, err := file.Seek(0, io.SeekCurrent); err != nil || pos != 123 {
				t.Fatal("worker changed caller cursor", pos, err)
			}
			if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
				t.Fatal("worker closed caller reader", err)
			}
			if _, err = worker.Info(ctx); err == nil {
				t.Fatal("closed worker remained usable")
			}
		})
	}
}

func TestControlMalformedHandshakeFencesProcess(t *testing.T) {
	for _, mode := range []string{"bad_id", "bad_dimensions", "bad_rate", "bad_reserved", "bad_status", "bad_error", "short"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
			t.Setenv("RIFFINE_CONTROL_TEST_MODE", mode)
			reader, info := controlFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			worker, err := (Adapter{Executable: os.Args[0]}).OpenScheduledWorker(ctx, reader, info)
			if err == nil || worker != nil {
				if worker != nil {
					worker.Close()
				}
				t.Fatal("accepted malformed handshake", mode)
			}
			if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
				t.Fatal("failed handshake closed reader", err)
			}
		})
	}
}
func TestControlOpeningCancellationFencesProcess(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_CONTROL_TEST_MODE", "stall_open")
	signal := filepath.Join(t.TempDir(), "started")
	t.Setenv("RIFFINE_DECODER_TEST_SIGNAL", signal)
	reader, info := controlFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		worker, err := (Adapter{Executable: os.Args[0]}).OpenScheduledWorker(ctx, reader, info)
		if worker != nil {
			worker.Close()
			done <- errors.New("stalled handshake returned a worker")
			return
		}
		done <- err
	}()
	for {
		if _, err := os.Stat(signal); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatal("handshake ended before cancellation", err)
		case <-ctx.Done():
			t.Fatal("handshake did not stall")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handshake cancellation did not fence process")
	}
	if _, err := reader.ReadAt(make([]byte, 4), 0); err != nil {
		t.Fatal(err)
	}
}
func TestControlCloseInterruptsPendingCall(t *testing.T)       { testControlStall(t, false) }
func TestControlRequestCancellationFencesProcess(t *testing.T) { testControlStall(t, true) }
func testControlStall(t *testing.T, cancelRequest bool) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_CONTROL_TEST_MODE", "stall_info")
	signal := filepath.Join(t.TempDir(), "started")
	t.Setenv("RIFFINE_DECODER_TEST_SIGNAL", signal)
	reader, info := controlFixture(t)
	lifetime, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	worker, err := (Adapter{Executable: os.Args[0]}).OpenScheduledWorker(lifetime, reader, info)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	ctx, cancel := context.WithCancel(lifetime)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := worker.Info(ctx); done <- err }()
	for {
		if _, err = os.Stat(signal); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatal("no stalled request", err)
		case <-lifetime.Done():
			t.Fatal("worker never stalled")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if cancelRequest {
		cancel()
	} else {
		worker.Close()
	}
	select {
	case err := <-done:
		if err == nil || (cancelRequest && !errors.Is(err, context.Canceled)) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stalled control ignored teardown")
	}
	if worker.cmd.ProcessState == nil || !worker.waited {
		t.Fatal("teardown did not wait for process")
	}
	if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
		t.Fatal("teardown closed borrowed reader", err)
	}
}

func TestNativeControlProtocolClose(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	reader, info := controlFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker, err := (Adapter{Executable: helper}).OpenScheduledWorker(ctx, reader, info)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	worker.mu.Lock()
	_, err = worker.exchangeLocked(ctx, [16]uint64{3: 4})
	if err == nil {
		err = worker.cmd.Wait()
		worker.waited = true
	}
	worker.mu.Unlock()
	if err != nil {
		t.Fatal("explicit protocol close did not exit cleanly", err)
	}
	if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
		t.Fatal(err)
	}
}

func TestNativeControlRejectsTruncatedEnvelope(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	reader, info := controlFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	worker, err := (Adapter{Executable: helper}).OpenScheduledWorker(ctx, reader, info)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	worker.mu.Lock()
	_, err = worker.input.Write([]byte{1})
	worker.input.Close()
	if err == nil {
		err = worker.cmd.Wait()
		worker.waited = true
	}
	worker.mu.Unlock()
	if err == nil {
		t.Fatal("native control accepted a truncated envelope")
	}
	if _, err = reader.ReadAt(make([]byte, 4), 0); err != nil {
		t.Fatal("protocol failure closed caller", err)
	}
}

func TestControlRejectsOlderHelper(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_DECODER_TEST_LEGACY_PREVIEW", "1")
	reader, info := controlFixture(t)
	worker, err := (Adapter{Executable: os.Args[0]}).OpenScheduledWorker(context.Background(), reader, info)
	if err == nil || worker != nil {
		if worker != nil {
			worker.Close()
		}
		t.Fatal("advertised unsupported persistent control")
	}
}

func TestNativeScheduledRateWorker(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native CI integration")
	}
	reader, info := controlFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := Adapter{Executable: helper}
	for _, rate := range []int{8000, 44100, 192000} {
		worker, err := a.OpenScheduledWorkerRate(ctx, reader, info, rate)
		if err != nil {
			t.Fatal(err)
		}
		if err = worker.Reset(ctx, 1); err != nil {
			worker.Close()
			t.Fatal(err)
		}
		frames := int64(math.Round(8850 * float64(rate) / float64(info.SampleRate) / .75))
		s := engine.Schedule{Generation: 1, Sequence: 1, StartFrame: 1151, EndFrame: 10001, OutputEnd: frames, Speed: .75, Pitch: 3, EdgeFadeFrames: int64(rate / 200)}
		if err = worker.Submit(ctx, s); err != nil {
			worker.Close()
			t.Fatal(err)
		}
		before, err := worker.Info(ctx)
		if err != nil || before.OutputRate != rate || before.SampleRate != info.SampleRate || before.Pending != 1 {
			worker.Close()
			t.Fatal(before, err)
		}
		worker.mu.Lock()
		_, err = worker.exchangeLocked(ctx, [16]uint64{3: 5, 4: 48000})
		worker.mu.Unlock()
		if err == nil {
			worker.Close()
			t.Fatal("changed negotiated rate after generation")
		}
		after, err := worker.Info(ctx)
		if err != nil || after != before {
			worker.Close()
			t.Fatal("rate rejection changed metadata", after, err)
		}
		if err = worker.Reset(ctx, 2); err != nil {
			worker.Close()
			t.Fatal(err)
		}
		reset, err := worker.Info(ctx)
		if err != nil || reset.OutputRate != rate || reset.Pending != 0 {
			worker.Close()
			t.Fatal("reset changed output rate", reset, err)
		}
		worker.Close()
	}
	for _, rate := range []int{0, 7999, 192001} {
		if worker, err := a.OpenScheduledWorkerRate(ctx, reader, info, rate); err == nil || worker != nil {
			if worker != nil {
				worker.Close()
			}
			t.Fatal("accepted invalid output rate")
		}
	}
}

func TestControlRejectsVersionOneHelper(t *testing.T) {
	t.Setenv("RIFFINE_DECODER_TEST_PROCESS", "1")
	t.Setenv("RIFFINE_CONTROL_TEST_LEGACY_VERSION", "1")
	reader, info := controlFixture(t)
	a := Adapter{Executable: os.Args[0]}
	if _, err := NewDecoder(context.Background(), a); err != nil {
		t.Fatal("valid older decoder was rejected", err)
	}
	worker, err := a.OpenScheduledWorkerRate(context.Background(), reader, info, 44100)
	if err == nil || worker != nil {
		if worker != nil {
			worker.Close()
		}
		t.Fatal("accepted incompatible scheduled-control protocol")
	}
}

func TestNativeControlSchedulesBoundedCuePayloads(t *testing.T) {
	helper := os.Getenv("RIFFINE_AUDIO_HELPER")
	if helper == "" {
		t.Skip("set RIFFINE_AUDIO_HELPER for mandatory native integration")
	}
	reader, info := controlFixture(t)
	worker, err := (Adapter{Executable: helper}).OpenScheduledWorker(context.Background(), reader, info)
	if err != nil {
		t.Fatal(err)
	}
	defer worker.Close()
	if err = worker.Reset(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	count := engine.Schedule{Generation: 1, Sequence: 1, Source: "source", StartFrame: 0, EndFrame: 0, OutputStart: 0, OutputEnd: 48000, Speed: .6, Pitch: 3, Phase: "count_in", CueOffsetFrames: 12000, ClickLevel: .7, Clicks: []timing.Click{{OutputFrame: 0, Accent: true}, {OutputFrame: 40000, Accent: false}}}
	if err = worker.Submit(context.Background(), count); err != nil {
		t.Fatal(err)
	}
	rest := engine.Schedule{Generation: 1, Sequence: 2, Source: "source", StartFrame: 0, EndFrame: 0, OutputStart: 48000, OutputEnd: 108000, Speed: .65, Pitch: 3, Phase: "rest"}
	if err = worker.Submit(context.Background(), rest); err != nil {
		t.Fatal(err)
	}
	current, err := worker.Info(context.Background())
	if err != nil || current.Pending != 2 || current.LastSequence != 2 {
		t.Fatal(current, err)
	}
	if err = worker.Reset(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	invalid := count
	invalid.Generation = 2
	invalid.Sequence = 3
	invalid.EndFrame = 1
	if err = worker.Submit(context.Background(), invalid); err == nil {
		t.Fatal("nonconstant cue anchor accepted")
	}
	current, err = worker.Info(context.Background())
	if err != nil || current.Pending != 0 {
		t.Fatal("invalid cue corrupted control stream", current, err)
	}
	invalid = count
	invalid.Generation = 2
	invalid.Sequence = 3
	invalid.ClickLevel = 2
	if err = worker.Submit(context.Background(), invalid); err == nil {
		t.Fatal("invalid click level accepted")
	}
	current, err = worker.Info(context.Background())
	if err != nil || current.Pending != 0 {
		t.Fatal("rejected payload lost protocol alignment", current, err)
	}
	count.Generation = 2
	count.Sequence = 3
	if err = worker.Submit(context.Background(), count); err != nil {
		t.Fatal("valid payload after rejection failed", err)
	}
}
