//go:build js && wasm

package main

import (
	"github.com/NLstn/riffine/spikes/hosts"
	"syscall/js"
)

func main() {
	run := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 || args[0].Type() != js.TypeString {
			return `{"version":1,"ok":false,"error":"fixture JSON string required"}`
		}
		return hosts.Run(args[0].String())
	})
	js.Global().Set("riffineRunFixture", run)
	select {}
}
