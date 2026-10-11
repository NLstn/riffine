package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/resource"
)

type list struct {
	rows []Candidate
	next int
}

func (l *list) Next(ctx context.Context) (Candidate, error) {
	if err := ctx.Err(); err != nil {
		return Candidate{}, err
	}
	if l.next == len(l.rows) {
		return Candidate{}, io.EOF
	}
	c := l.rows[l.next]
	l.next++
	return c, nil
}

type host struct {
	data           map[string][]byte
	opened, closed int
}
type reader struct {
	*bytes.Reader
	h *host
}

func (r reader) Name() string { return "host.wav" }
func (r reader) Close() error { r.h.closed++; return nil }
func (h *host) Open(ctx context.Context, s string) (resource.Reader, error) {
	b, ok := h.data[s]
	if !ok {
		return nil, errors.New("missing")
	}
	h.opened++
	return reader{bytes.NewReader(b), h}, nil
}
func (h *host) Resolve(ctx context.Context, a, b string) (string, error)   { return b, ctx.Err() }
func (h *host) Reference(ctx context.Context, a, b string) (string, error) { return b, ctx.Err() }
func tone(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := audio.WriteTone(&b, .01, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestIndexIdentityFallbackAndMalformed(t *testing.T) {
	b := tone(t)
	h := &host{data: map[string][]byte{"a": b, "alias": b, "bad": []byte("not audio")}}
	src := &list{rows: []Candidate{{Handle: "a", Name: "音楽.wav"}, {Handle: "alias", Name: "copy.mp3", Title: " Tagged ", Artist: " A "}, {Handle: "bad", Name: "bad.wav"}, {Handle: "missing", Name: "missing.wav"}, {Handle: "huge", Name: strings.Repeat("x", MaxTextBytes+1)}}}
	idx, err := New(h, audio.WAVDecoder{}, src)
	if err != nil {
		t.Fatal(err)
	}
	first, err := idx.Next(context.Background(), 1)
	if err != nil || first.Done || len(first.Results) != 1 {
		t.Fatal(first, err)
	}
	track := first.Results[0].Track
	if first.Results[0].Status != "indexed" || track.Title != "音楽" || track.Audio.DataOffset != 0 || track.Audio.DataBytes != 0 {
		t.Fatal(first)
	}
	rest, err := idx.Next(context.Background(), MaxBatch)
	if err != nil || !rest.Done || len(rest.Results) != 4 {
		t.Fatal(rest, err)
	}
	if rest.Results[0].Status != "duplicate" || rest.Results[0].Track.ID != track.ID || rest.Results[0].Track.Title != "Tagged" || rest.Results[0].Track.Artist != "A" {
		t.Fatal(rest.Results[0])
	}
	// The extension is deliberately false: actual decoder bytes establish support.
	if rest.Results[1].Status != "unavailable" || rest.Results[2].Status != "unavailable" || rest.Results[3].Status != "invalid" || rest.Results[3].Candidate.Name != "" {
		t.Fatal(rest)
	}
	if h.opened != h.closed || len(idx.seen) != 1 {
		t.Fatalf("ownership/index: %+v %d", h, len(idx.seen))
	}
	empty, err := idx.Next(context.Background(), 1)
	if err != nil || !empty.Done || len(empty.Results) != 0 {
		t.Fatal(empty, err)
	}
}

type interrupted struct {
	audio.WAVDecoder
	cancel context.CancelFunc
	once   bool
}

func (d *interrupted) Inspect(ctx context.Context, r resource.Reader) (audio.Info, error) {
	if !d.once {
		d.once = true
		d.cancel()
		return audio.Info{}, ctx.Err()
	}
	return d.WAVDecoder.Inspect(ctx, r)
}
func TestIndexCancellationRetriesPending(t *testing.T) {
	h := &host{data: map[string][]byte{"a": tone(t)}}
	src := &list{rows: []Candidate{{Handle: "a", Name: "a.wav"}}}
	ctx, cancel := context.WithCancel(context.Background())
	d := &interrupted{cancel: cancel}
	idx, _ := New(h, d, src)
	batch, err := idx.Next(ctx, 2)
	if !errors.Is(err, context.Canceled) || len(batch.Results) != 0 || idx.pending == nil || len(idx.seen) != 0 {
		t.Fatal(batch, err)
	}
	batch, err = idx.Next(context.Background(), 2)
	if err != nil || !batch.Done || batch.Results[0].Status != "indexed" || src.next != 1 || h.opened != h.closed {
		t.Fatal(batch, err)
	}
	for _, n := range []int{0, MaxBatch + 1} {
		if _, err = idx.Next(context.Background(), n); err == nil {
			t.Fatal("bad batch accepted")
		}
	}
}

func TestIndexCapacityRejectsNewIdentity(t *testing.T) {
	h := &host{data: map[string][]byte{"a": tone(t)}}
	src := &list{rows: []Candidate{{Handle: "a", Name: "a.wav"}}}
	idx, _ := New(h, audio.WAVDecoder{}, src)
	for n := 0; n < MaxEntries; n++ {
		idx.seen[fmt.Sprint(n)] = audio.Info{}
	}
	batch, err := idx.Next(context.Background(), 1)
	if err == nil || len(batch.Results) != 0 || idx.pending == nil || h.opened != h.closed || len(idx.seen) != MaxEntries {
		t.Fatal("capacity failure mutated index or leaked resource", err)
	}
}
