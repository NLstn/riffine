package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var ErrRevisionConflict = errors.New("document revision changed")
var ErrDocumentBusy = errors.New("document has an active or interrupted save lock")

func Revision(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

type UpdateResult struct {
	Revision string `json:"revision"`
	Recovery string `json:"recovery,omitempty"` // opaque host handle to previous valid bytes
}

// UpdatingStore is an optional document port. Update compares the exact byte
// revision and publishes all new bytes atomically among cooperating writers.
// Failure must retain the last valid document or a separately readable recovery
// copy. External resource mutation is outside this cooperating-writer contract.
// Hosts may reject unsupported filesystems explicitly; no truncate fallback.
type UpdatingStore interface {
	Store
	Update(ctx context.Context, document, expectedRevision string, data []byte) (UpdateResult, error)
}
