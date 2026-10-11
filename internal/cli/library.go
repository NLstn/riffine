package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"unicode/utf8"

	"github.com/NLstn/riffine/adapters/filesystem"
	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/library"
)

// The explicit locator list is a bounded CLI host adapter. Other clients can
// supply lazy directory/document enumeration through library.Source.
func scanSources(ctx context.Context, source string, out io.Writer) error {
	if source == "" {
		return errors.New("--sources is required")
	}
	data, err := (filesystem.Host{}).Read(ctx, source, 1<<20)
	if err != nil {
		return err
	}
	if !utf8.Valid(data) {
		return errors.New("source list must be UTF-8")
	}
	var locators []string
	dec := json.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(&locators); err != nil {
		return err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return errors.New("unexpected trailing source list data")
	}
	if len(locators) == 0 || len(locators) > library.MaxEntries {
		return errors.New("source list requires 1–100000 locators within 1 MiB")
	}
	decoder := audio.WAVDecoder{}
	idx, err := library.New(filesystem.Host{}, decoder, &locatorSource{document: source, locators: locators})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	for {
		batch, err := idx.Next(ctx, library.MaxBatch)
		// Completed rows remain useful even when a later candidate is cancelled.
		if len(batch.Results) > 0 || batch.Done {
			row := struct {
				Version int      `json:"version"`
				Formats []string `json:"formats"`
				library.Batch
			}{1, decoder.Formats(), batch}
			if e := enc.Encode(row); e != nil {
				return e
			}
		}
		if err != nil {
			return err
		}
		if batch.Done {
			return nil
		}
	}
}

type locatorSource struct {
	document string
	locators []string
	next     int
}

func (s *locatorSource) Next(ctx context.Context) (library.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return library.Candidate{}, err
	}
	if s.next == len(s.locators) {
		return library.Candidate{}, io.EOF
	}
	locator := s.locators[s.next]
	if locator == "" {
		s.next++
		return library.Candidate{}, nil
	}
	handle, err := (filesystem.Host{}).Resolve(ctx, s.document, locator)
	if err != nil {
		return library.Candidate{}, err
	}
	s.next++
	return library.Candidate{Handle: handle, Name: filepath.Base(filepath.FromSlash(locator))}, nil
}
