// Package library owns bounded, host-neutral indexing and identity rules.
package library

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

const MaxBatch = 256
const MaxEntries = 100000
const MaxTextBytes = 4096

// Candidate handles are scoped to the host. Locators/names and optional tags
// come from the host; an extension never establishes decoder support.
type Candidate struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Title  string `json:"title,omitempty"`
	Artist string `json:"artist,omitempty"`
	Album  string `json:"album,omitempty"`
}

// Source supplies deterministic host enumeration order, without preloading a
// complete folder. A candidate with io.EOF is not consumed. Close is host-owned.
type Source interface {
	Next(context.Context) (Candidate, error)
}
type Track struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Title  string     `json:"title"`
	Artist string     `json:"artist,omitempty"`
	Album  string     `json:"album,omitempty"`
	Audio  audio.Info `json:"audio"`
}
type Result struct {
	Candidate Candidate `json:"candidate"`
	Status    string    `json:"status"` // indexed, duplicate, unavailable, invalid
	Track     *Track    `json:"track,omitempty"`
	Message   string    `json:"message,omitempty"`
}
type Batch struct {
	Results []Result `json:"results"`
	Done    bool     `json:"done"`
}

// Index keeps bounded canonical identities; aliases are emitted, not retained.
// Each successful Next advances exactly once. Calls must be serialized by the
// host worker. Cancellation consumes no candidate during an active inspect;
// that candidate is retried on the next call. Completed rows remain committed.
type Index struct {
	resources resource.Resources
	decoder   audio.Decoder
	source    Source
	seen      map[string]audio.Info
	pending   *Candidate
	done      bool
}

func New(resources resource.Resources, decoder audio.Decoder, source Source) (*Index, error) {
	if resources == nil || decoder == nil || source == nil {
		return nil, errors.New("library resource, decoder and source ports are required")
	}
	return &Index{resources: resources, decoder: decoder, source: source, seen: make(map[string]audio.Info)}, nil
}
func validText(s string) bool {
	return len(s) <= MaxTextBytes && utf8.ValidString(s) && !strings.ContainsRune(s, '\x00')
}
func (i *Index) Next(ctx context.Context, limit int) (Batch, error) {
	batch := Batch{Results: make([]Result, 0)}
	if limit < 1 || limit > MaxBatch {
		return batch, errors.New("library batch must be 1–256 entries")
	}
	for len(batch.Results) < limit && !i.done {
		if err := ctx.Err(); err != nil {
			return batch, err
		}
		if i.pending == nil {
			c, err := i.source.Next(ctx)
			if errors.Is(err, io.EOF) {
				i.done = true
				break
			}
			if err != nil {
				return batch, err
			}
			i.pending = &c
		}
		c := *i.pending
		result := Result{Candidate: c}
		if c.Handle == "" || c.Name == "" || !validText(c.Handle) || !validText(c.Name) || !validText(c.Title) || !validText(c.Artist) || !validText(c.Album) {
			result.Status = "invalid"
			result.Message = "candidate metadata exceeds text limits or is invalid"
			// Do not echo an unbounded or invalid host value to clients.
			result.Candidate = Candidate{}
		} else {
			info, err := i.inspect(ctx, c.Handle)
			if ctx.Err() != nil {
				return batch, ctx.Err()
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return batch, err
			}
			if err != nil {
				result.Status = "unavailable"
				result.Message = "source cannot be inspected by this host decoder"
			} else {
				title := strings.TrimSpace(c.Title)
				if title == "" {
					title = strings.TrimSuffix(c.Name, path.Ext(c.Name))
					if title == "" {
						title = c.Name
					}
				}
				track := Track{ID: info.SHA256, Name: c.Name, Title: title, Artist: strings.TrimSpace(c.Artist), Album: strings.TrimSpace(c.Album), Audio: info}
				// Decoder cursor offsets are never library metadata.
				track.Audio.DataOffset = 0
				track.Audio.DataBytes = 0
				result.Track = &track
				if old, ok := i.seen[info.SHA256]; ok {
					if old.SampleRate != info.SampleRate || old.Channels != info.Channels || old.Frames != info.Frames || old.Bits != info.Bits || old.Encoding != info.Encoding {
						return batch, errors.New("decoder returned inconsistent metadata for the same identity")
					}
					result.Status = "duplicate"
				} else {
					if len(i.seen) == MaxEntries {
						return batch, errors.New("library index capacity exceeded")
					}
					i.seen[info.SHA256] = track.Audio
					result.Status = "indexed"
				}
			}
		}
		batch.Results = append(batch.Results, result)
		i.pending = nil
	}
	batch.Done = i.done
	return batch, nil
}
func (i *Index) inspect(ctx context.Context, handle string) (audio.Info, error) {
	r, err := i.resources.Open(ctx, handle)
	if err != nil {
		return audio.Info{}, err
	}
	info, err := i.decoder.Inspect(ctx, r)
	if err == nil {
		err = audio.ValidateInfo(info)
	}
	closeErr := r.Close()
	if err != nil {
		return audio.Info{}, err
	}
	if closeErr != nil {
		return audio.Info{}, closeErr
	}
	return info, nil
}
