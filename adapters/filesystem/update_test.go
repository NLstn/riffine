package filesystem

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/project"
	"github.com/NLstn/riffine/core/resource"
)

func savedFixture(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	p, e := project.New("tone.wav", "tone.wav", audio.Info{SampleRate: 48000, Channels: 1, Bits: 16, Frames: 48000, SHA256: strings.Repeat("ab", 32)})
	if e != nil {
		t.Fatal(e)
	}
	old, e := project.Encode(p)
	if e != nil {
		t.Fatal(e)
	}
	p.Annotations.Notes = "Next practice"
	next, e := project.Encode(p)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "歌曲 🎸.json")
	if e = (Host{}).Create(context.Background(), path, old); e != nil {
		t.Fatal(e)
	}
	return path, old, next
}
func TestAtomicUpdateAndRecovery(t *testing.T) {
	path, old, next := savedFixture(t)
	result, e := (Host{}).Update(context.Background(), path, resource.Revision(old), next)
	if e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	backup, _ := os.ReadFile(result.Recovery)
	if !bytes.Equal(got, next) || !bytes.Equal(backup, old) || result.Revision != resource.Revision(next) {
		t.Fatal("update/recovery mismatch", result)
	}
	if _, e = (Host{}).Update(context.Background(), path, resource.Revision(old), old); !errors.Is(e, resource.ErrRevisionConflict) {
		t.Fatal("stale revision accepted", e)
	}
	unchanged, _ := os.ReadFile(path)
	if !bytes.Equal(unchanged, next) {
		t.Fatal("conflict destroyed latest document")
	}
	if _, e = (Host{}).Update(context.Background(), path, result.Revision, next); e != nil {
		t.Fatal("same-content save failed", e)
	}
}
func TestInterruptedAndCancelledUpdatePreservesDocument(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "cancellation"}[cancelled], func(t *testing.T) {
			path, old, next := savedFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			injected := errors.New("interrupted before commit")
			result, e := (Host{}).update(ctx, path, resource.Revision(old), next, func() error {
				if cancelled {
					cancel()
					return nil
				}
				return injected
			})
			if e == nil {
				t.Fatal("ignored interruption")
			}
			if cancelled && !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
			got, _ := os.ReadFile(path)
			backup, _ := os.ReadFile(result.Recovery)
			if !bytes.Equal(got, old) || !bytes.Equal(backup, old) {
				t.Fatal("last valid document lost")
			}
			if _, e = os.Stat(path + ".riffine-lock"); !errors.Is(e, os.ErrNotExist) {
				t.Fatal("lock leaked on return", e)
			}
			if _, e = (Host{}).Update(context.Background(), path, resource.Revision(old), next); e != nil {
				t.Fatal("retry after interruption failed", e)
			}
		})
	}
}
func TestCooperatingWritersCannotOverwriteStaleRevision(t *testing.T) {
	path, old, next := savedFixture(t)
	locked := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, e := (Host{}).update(context.Background(), path, resource.Revision(old), next, func() error { close(locked); <-release; return nil })
		finished <- e
	}()
	<-locked
	if _, e := (Host{}).Update(context.Background(), path, resource.Revision(old), old); !errors.Is(e, resource.ErrDocumentBusy) {
		t.Error("did not protect active writer", e)
	}
	close(release)
	if e := <-finished; e != nil {
		t.Fatal(e)
	}
	if _, e := (Host{}).Update(context.Background(), path, resource.Revision(old), old); !errors.Is(e, resource.ErrRevisionConflict) {
		t.Fatal("lost update", e)
	}
}
func TestInterruptedLockAndCorruptRecoveryFailSafely(t *testing.T) {
	path, old, next := savedFixture(t)
	if e := os.Mkdir(path+".riffine-lock", 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := (Host{}).Update(context.Background(), path, resource.Revision(old), next); !errors.Is(e, resource.ErrDocumentBusy) {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, old) {
		t.Fatal("lock recovery modified original")
	}
	if e := os.Remove(path + ".riffine-lock"); e != nil {
		t.Fatal(e)
	}
	backup := path + ".recovery-" + resource.Revision(old) + ".json"
	if e := os.WriteFile(backup, []byte("unrelated file"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := (Host{}).Update(context.Background(), path, resource.Revision(old), next); e == nil {
		t.Fatal("accepted conflicting recovery file")
	}
	got, _ = os.ReadFile(path)
	if !bytes.Equal(got, old) {
		t.Fatal("corrupt backup destroyed document")
	}
}
func TestUpdateCannotReplaceSourceAudio(t *testing.T) {
	path, _, next := savedFixture(t)
	source := filepath.Join(filepath.Dir(path), "tone.wav")
	if e := WriteTone(source, .1, 440, 48000, 1); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(source)
	if _, e := (Host{}).Update(context.Background(), source, resource.Revision(before), next); e == nil {
		t.Fatal("replaced source audio")
	}
	after, _ := os.ReadFile(source)
	if !bytes.Equal(before, after) {
		t.Fatal("changed source bytes")
	}
	if _, e := (Host{}).Update(context.Background(), path, "not-a-revision", next); e == nil {
		t.Fatal("accepted invalid revision")
	}
}
