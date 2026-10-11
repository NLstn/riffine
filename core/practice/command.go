package practice

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

const MaxCommandBytes = 1 << 20

// DecodeCommand is the shared JSON entry point for bindings. It rejects unknown
// fields, trailing data and oversized messages before dispatch. Project schema
// version and command API version are independent.
func DecodeCommand(r io.Reader) (Command, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxCommandBytes+1))
	if err != nil {
		return Command{}, failure(InvalidCommand, err)
	}
	if len(b) > MaxCommandBytes {
		return Command{}, failure(InvalidCommand, errors.New("command exceeds 1 MiB"))
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var c Command
	if err = dec.Decode(&c); err != nil {
		return Command{}, failure(InvalidCommand, err)
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return Command{}, failure(InvalidCommand, errors.New("unexpected trailing command data"))
	}
	if c.Version != APIVersion {
		return Command{}, failure(UnsupportedVersion, errors.New("unsupported command version"))
	}
	return c, nil
}
