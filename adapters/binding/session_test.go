package binding

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/practice"
)

func TestBindingUsesCoreCommandsAndPortableReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "音楽.wav")
	document := filepath.Join(dir, "session.json")
	var pcm bytes.Buffer
	if err := audio.WriteTone(&pcm, .2, 440, 48000, 2); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, pcm.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	host := filesystem.Host{}
	service := practice.Service{Resources: host, Store: host, Decoder: audio.WAVDecoder{}}
	bridge := New(service)
	defer bridge.Close()
	call := func(request Request) Response {
		t.Helper()
		data, _ := json.Marshal(request)
		response := bridge.Handle(ctx, data)
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		return response
	}
	opened := call(Request{Version: 1, Type: "import", Source: path, Document: document})
	if opened.Snapshot.Sequence != 0 || opened.Snapshot.Capabilities.LiveTransport {
		t.Fatal(opened)
	}
	command := json.RawMessage(`{"version":1,"sequence":1,"type":"set_parameters","speed":0.75,"pitch":3}`)
	edited := call(Request{Version: 1, Type: "command", Command: command})
	if edited.Snapshot.Sequence != 1 || edited.Snapshot.Project.Settings.Speed != .75 || edited.Snapshot.Project.Settings.Pitch != 3 {
		t.Fatal(edited)
	}
	bad := bridge.Handle(ctx, []byte(`{"version":1,"type":"command","command":{"version":1,"sequence":1,"type":"set_parameters","speed":1}}`))
	if bad.Error == nil || bad.Error.Code != practice.OutOfOrder || bad.Snapshot.Sequence != 1 {
		t.Fatal("duplicate command changed state", bad)
	}
	detail := call(Request{Version: 1, Type: "detail", StartFrame: 255, EndFrame: 258, Bins: 3})
	if detail.Window.FramesPerCell != 1 || len(detail.Window.Cells) != 3 {
		t.Fatal(detail)
	}
	window := call(Request{Version: 1, Type: "waveform", StartFrame: 0, EndFrame: 9600, Bins: 7})
	window2 := call(Request{Version: 1, Type: "waveform", StartFrame: 0, EndFrame: 9600, Bins: 7})
	if !reflect.DeepEqual(window.Window, window2.Window) {
		t.Fatal("retained overview changed")
	}
	exported := call(Request{Version: 1, Type: "export"})
	if strings.Contains(exported.Document, "data:audio") || exported.Document == "" {
		t.Fatal(exported)
	}
	if err := os.WriteFile(document, []byte(exported.Document), 0600); err != nil {
		t.Fatal(err)
	}
	reopened := New(service)
	defer reopened.Close()
	data, _ := json.Marshal(Request{Version: 1, Type: "open", Document: document})
	response := reopened.Handle(ctx, data)
	if response.Error != nil || !reflect.DeepEqual(response.Snapshot.Project, edited.Snapshot.Project) {
		t.Fatal(response)
	}
	// A source change must be rejected before presenting an opened project.
	pcm.Reset()
	audio.WriteTone(&pcm, .2, 660, 48000, 2)
	os.WriteFile(path, pcm.Bytes(), 0600)
	changed := New(service)
	defer changed.Close()
	if response := changed.Handle(ctx, data); response.Error == nil || response.Error.Code != practice.IdentityMismatch {
		t.Fatal("changed original accepted", response)
	}
}
func TestBindingRejectsMalformedAndCancelledRequests(t *testing.T) {
	bridge := New(practice.Service{})
	defer bridge.Close()
	for _, data := range []string{`{}`, `{"version":2,"type":"snapshot"}`, `{"version":1,"type":"snapshot","extra":1}`, `{"version":1,"type":"snapshot"} {}`, `{"version":1,"type":"snapshot","command":{}}`, `{"version":1,"type":"snapshot","startFrame":1}`, `{"version":1,"type":"snapshot","source":"unwanted"}`, strings.Repeat("x", MaxRequestBytes+1)} {
		if response := bridge.Handle(context.Background(), []byte(data)); response.Error == nil {
			t.Fatal("invalid binding request accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if response := bridge.Handle(ctx, []byte(`{"version":1,"type":"snapshot"}`)); response.Error == nil || response.Error.Code != practice.Cancelled {
		t.Fatal(response)
	}
	bridge.Close()
	if response := bridge.Handle(context.Background(), []byte(`{"version":1,"type":"snapshot"}`)); response.Error == nil {
		t.Fatal("closed bridge accepted request")
	}
}

func TestBindingRevisionProtectedSaveAndRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "source.wav"), filepath.Join(dir, "saved.json")
	var pcm bytes.Buffer
	if err := audio.WriteTone(&pcm, .2, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, pcm.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	host := filesystem.Host{}
	service := practice.Service{Resources: host, Store: host, Decoder: audio.WAVDecoder{}}
	call := func(bridge *Session, request Request) Response {
		t.Helper()
		data, _ := json.Marshal(request)
		return bridge.Handle(ctx, data)
	}
	imported := New(service)
	defer imported.Close()
	if response := call(imported, Request{Version: 1, Type: "import", Source: source, Document: destination}); response.Error != nil {
		t.Fatal(response)
	}
	if response := call(imported, Request{Version: 1, Type: "save_as"}); response.Error == nil {
		t.Fatal("missing destination accepted")
	}
	if response := call(imported, Request{Version: 1, Type: "save"}); response.Error == nil || response.Error.Code != practice.UnsupportedCapability {
		t.Fatal(response)
	}
	if response := call(imported, Request{Version: 1, Type: "save_as", Document: destination}); response.Error != nil {
		t.Fatal(response)
	}
	previous, _ := os.ReadFile(destination)
	if response := call(imported, Request{Version: 1, Type: "save_as", Document: destination}); response.Error == nil {
		t.Fatal("save-as overwrote a project")
	}
	first, stale := New(service), New(service)
	defer first.Close()
	defer stale.Close()
	for _, bridge := range []*Session{first, stale} {
		if response := call(bridge, Request{Version: 1, Type: "open", Document: destination}); response.Error != nil {
			t.Fatal(response)
		}
		if response := call(bridge, Request{Version: 1, Type: "command", Command: json.RawMessage(`{"version":1,"sequence":1,"type":"set_parameters","speed":0.6}`)}); response.Error != nil {
			t.Fatal(response)
		}
	}
	saved := call(first, Request{Version: 1, Type: "save"})
	if saved.Error != nil || saved.Saved == nil || saved.Saved.Revision != saved.Snapshot.Revision || saved.Snapshot.Sequence != 1 {
		t.Fatal(saved)
	}
	recovery, err := os.ReadFile(saved.Saved.Recovery)
	if err != nil || !bytes.Equal(recovery, previous) {
		t.Fatal("previous bytes were not retained", err)
	}
	conflict := call(stale, Request{Version: 1, Type: "save"})
	if conflict.Error == nil || conflict.Error.Code != practice.RevisionConflict {
		t.Fatal(conflict)
	}
	if response := call(stale, Request{Version: 1, Type: "export"}); response.Error != nil || !strings.Contains(response.Document, `"speed": 0.6`) {
		t.Fatal("conflict discarded edits", response)
	}
}
