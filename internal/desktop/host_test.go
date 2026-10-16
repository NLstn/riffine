package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/NLstn/riffine/adapters/binding"
	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/practice"
)

func TestHostImportEditSaveAndFailedReplacement(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source, document := filepath.Join(dir, "original.wav"), filepath.Join(dir, "practice.json")
	if err := filesystem.WriteTone(source, .1, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	host := Host{Service: practice.Service{Resources: filesystem.Host{}, Store: filesystem.Host{}, Decoder: audio.WAVDecoder{}}}
	defer host.Close()
	call := func(request any) binding.Response {
		t.Helper()
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		return host.Handle(ctx, data).(binding.Response)
	}
	response := call(binding.Request{Version: 1, Type: "import", Source: source, Document: document})
	if response.Error != nil || response.Snapshot == nil {
		t.Fatalf("import: %+v", response)
	}
	response = call(binding.Request{Version: 1, Type: "open", Document: filepath.Join(dir, "missing.json")})
	if response.Error == nil {
		t.Fatal("opened missing project")
	}
	response = call(binding.Request{Version: 1, Type: "command", Command: json.RawMessage(`{"version":1,"sequence":1,"type":"set_parameters","speed":0.75,"pitch":-2}`)})
	if response.Error != nil || response.Snapshot.Project.Settings.Speed != .75 {
		t.Fatalf("failed replacement lost original session: %+v", response)
	}
	response = call(binding.Request{Version: 1, Type: "waveform", StartFrame: 0, EndFrame: 4800, Bins: 50})
	if response.Error != nil || response.Window == nil || len(response.Window.Cells) == 0 {
		t.Fatalf("waveform: %+v", response)
	}
	response = call(binding.Request{Version: 1, Type: "save_as", Document: document})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	before, _ := os.ReadFile(document)
	response = call(binding.Request{Version: 1, Type: "save_as", Document: document})
	if response.Error == nil {
		t.Fatal("replaced an existing project")
	}
	response = call(binding.Request{Version: 1, Type: "open", Document: document})
	if response.Error != nil || response.Snapshot.Sequence != 0 || response.Snapshot.Project.Settings.Pitch != -2 {
		t.Fatalf("reopen: %+v", response)
	}
	after, _ := os.ReadFile(document)
	if string(before) != string(after) {
		t.Fatal("opening changed the project file")
	}
}
