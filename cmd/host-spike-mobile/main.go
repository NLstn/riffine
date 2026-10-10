//go:build cgo

// The exported ABI is a spike for mobile in-process control calls. No audio
// samples or realtime callbacks cross this boundary. Caller frees each result.
package main

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"github.com/NLstn/riffine/spikes/hosts"
	"unsafe"
)

//export RiffineRunFixture
func RiffineRunFixture(input *C.char) *C.char {
	if input == nil {
		return C.CString(`{"version":1,"ok":false,"error":"fixture required"}`)
	}
	size := C.strnlen(input, C.size_t(hosts.MaxFixtureBytes+1))
	if size > hosts.MaxFixtureBytes {
		return C.CString(`{"version":1,"ok":false,"error":"fixture exceeds 1 MiB"}`)
	}
	return C.CString(hosts.Run(C.GoStringN(input, C.int(size))))
}

//export RiffineFree
func RiffineFree(result *C.char) { C.free(unsafe.Pointer(result)) }
func main()                      {}
