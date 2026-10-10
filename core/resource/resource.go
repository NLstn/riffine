// Package resource defines host-owned local resource and persistence ports.
package resource

import (
	"context"
	"io"
)

// Handles are opaque strings scoped to a host session. Only the host interprets
// them as paths, browser handles or mobile document IDs. Never persist a handle
// where a portable content identity is required.
type Reader interface {
	io.ReaderAt
	io.Closer
	Size() int64
	Name() string
}

type Resources interface {
	// Each successful Open transfers ownership of a reader to the caller.
	Open(context.Context, string) (Reader, error)
	// Resolve maps a saved reference to a host handle, relative to its document.
	Resolve(context.Context, string, string) (string, error)
	// Reference produces a saved locator for a resource relative to a document.
	Reference(context.Context, string, string) (string, error)
}

type Store interface {
	// Read must reject oversized documents without reading them into memory.
	Read(ctx context.Context, document string, maxBytes int) ([]byte, error)
	// Create publishes a complete document exclusively. Failure must leave no
	// partial document and must never replace an existing document or audio.
	Create(context.Context, string, []byte) error
}
