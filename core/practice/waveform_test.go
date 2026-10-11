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
