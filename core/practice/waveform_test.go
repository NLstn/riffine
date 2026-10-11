package practice

import (
	"bytes"
	"context"
	"testing"

	"github.com/NLstn/riffine/core/audio"
)

func TestOpenWaveformIdentityAndOwnedHandle(t *testing.T) {
	var data bytes.Buffer
	if err := audio.WriteTone(&data, .2, 440, 48000, 1); err != nil {
		t.Fatal(err)
	}
	h := &memoryHost{data: map[string][]byte{"source": data.Bytes()}}
	service := Service{Resources: h, Store: h, Decoder: audio.WAVDecoder{}}
	ctx := context.Background()
	session, err := service.Import(ctx, "source", "project")
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.OpenWaveform(ctx, nil, nil)
	if err != nil || h.opened != h.closed {
		t.Fatal(err)
	}
	opened := h.opened
	for _, rangeFrames := range [][2]int64{{0, 9600}, {255, 257}, {9000, 9600}} {
		if _, err = result.Pyramid.Query(ctx, rangeFrames[0], rangeFrames[1], 3); err != nil {
			t.Fatal(err)
		}
	}
	if h.opened != opened || session.Snapshot().Sequence != 0 {
		t.Fatal("query touched resources or edit sequence")
	}
	data.Reset()
	if err = audio.WriteTone(&data, .2, 660, 48000, 1); err != nil {
		t.Fatal(err)
	}
	h.data["source"] = data.Bytes()
	result, err = session.OpenWaveform(ctx, nil, nil)
	if err == nil || result.Pyramid != nil || h.opened != h.closed {
		t.Fatal("changed source accepted", err)
	}
}

func TestWaveformDetailOwnsSourceAndChecksIdentity(t *testing.T) {
	ctx := context.Background()
	var data bytes.Buffer
	if err := audio.WriteTone(&data, .2, 440, 48000, 2); err != nil {
		t.Fatal(err)
	}
	h := &memoryHost{data: map[string][]byte{"source": data.Bytes()}}
	session, err := (Service{Resources: h, Store: h, Decoder: audio.WAVDecoder{}}).Import(ctx, "source", "project")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := session.OpenWaveformDetail(ctx)
	if err != nil || h.opened != h.closed+1 {
		t.Fatal("detail resource ownership", err)
	}
	opened := h.opened
	for _, bounds := range [][2]int64{{255, 258}, {8191, 8193}, {9599, 9600}} {
		view, err := detail.Query(ctx, bounds[0], bounds[1], 4096)
		if err != nil || view.Cells[0].StartFrame != bounds[0] || view.Cells[len(view.Cells)-1].EndFrame != bounds[1] {
			t.Fatal(view, err)
		}
	}
	if h.opened != opened || session.Snapshot().Sequence != 0 {
		t.Fatal("query reopened source or edited project")
	}
	if err := detail.Close(); err != nil || h.opened != h.closed {
		t.Fatal("resource leaked", err)
	}
	detail.Close()
	if h.opened != h.closed {
		t.Fatal("closed source twice")
	}
	data.Reset()
	audio.WriteTone(&data, .2, 660, 48000, 2)
	h.data["source"] = data.Bytes()
	if detail, err := session.OpenWaveformDetail(ctx); err == nil || detail != nil || h.opened != h.closed {
		t.Fatal("changed source accepted", err)
	}
}
