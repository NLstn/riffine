//go:build js && wasm

package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"syscall/js"

	"github.com/NLstn/riffine/adapters/binding"
	"github.com/NLstn/riffine/adapters/browser"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/practice"
	"github.com/NLstn/riffine/core/project"
)

func main() {
	var mu sync.Mutex
	var active context.CancelFunc
	var session *binding.Session
	post := func(id int, response binding.Response) {
		data, _ := json.Marshal(response)
		js.Global().Call("postMessage", map[string]any{"type": "result", "id": id, "response": js.Global().Get("JSON").Call("parse", string(data))})
	}
	fail := func(id int, err error) {
		post(id, binding.Response{Version: 1, Error: &practice.Error{Version: 1, Code: practice.HostFailure, Message: err.Error()}})
	}
	request := js.FuncOf(func(_ js.Value, args []js.Value) any {
		id := args[0].Int()
		data := args[1].String()
		file := args[2]
		document := args[3].String()
		mu.Lock()
		if active != nil {
			mu.Unlock()
			fail(id, errors.New("another practice operation is running"))
			return nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		active = cancel
		mu.Unlock()
		go func() {
			defer func() { cancel(); mu.Lock(); active = nil; mu.Unlock() }()
			if len(data) > binding.MaxRequestBytes || len(document) > project.MaxDocumentBytes {
				fail(id, errors.New("practice request exceeds limit"))
				return
			}
			var envelope binding.Request
			if err := json.Unmarshal([]byte(data), &envelope); err != nil {
				fail(id, err)
				return
			}
			current := session
			if envelope.Type == "import" || envelope.Type == "open" {
				host, err := browser.NewFiles(file, []byte(document))
				if err != nil {
					fail(id, err)
					return
				}
				current = binding.New(practice.Service{Resources: host, Store: host, Decoder: audio.WAVDecoder{}})
			}
			if current == nil {
				fail(id, errors.New("select a source to open practice"))
				return
			}
			response := current.Handle(ctx, []byte(data))
			if current != session {
				if response.Error == nil {
					if session != nil {
						session.Close()
					}
					session = current
				} else {
					current.Close()
				}
			}
			post(id, response)
		}()
		return nil
	})
	cancel := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		mu.Lock()
		if active != nil {
			active()
		}
		mu.Unlock()
		return nil
	})
	js.Global().Set("riffinePracticeRequest", request)
	js.Global().Set("riffinePracticeCancel", cancel)
	js.Global().Call("postMessage", map[string]any{"type": "ready"})
	select {}
}
