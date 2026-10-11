package hosts

import (
	"context"
	"errors"
	"io"
	"reflect"

	"github.com/NLstn/riffine/core/audio"
	"github.com/NLstn/riffine/core/library"
)

func pcmLibraryConformance(ctx context.Context, host *memory) error {
	r, err := host.Open(ctx, "source")
	if err != nil {
		return err
	}
	defer r.Close()
	info, err := (audio.WAVDecoder{}).Inspect(ctx, r)
	if err != nil {
		return err
	}
	s, err := (audio.WAVDecoder{}).OpenPCM(ctx, r)
	if err != nil {
		return err
	}
	defer s.Close()
	frames := min(int64(1024), info.Frames)
	start := frames / 2
	all := make([]float32, int(frames)*info.Channels)
	if n, err := s.ReadFrames(ctx, all); err != nil || int64(n) != frames {
		return errors.New("PCM fixture read differs")
	}
	if err = s.SeekFrame(ctx, start); err != nil {
		return err
	}
	seek := make([]float32, int(frames-start)*info.Channels)
	if n, err := s.ReadFrames(ctx, seek); err != nil || int64(n) != frames-start || !reflect.DeepEqual(seek, all[int(start)*info.Channels:]) {
		return errors.New("PCM fixture seek differs")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(s.SeekFrame(cancelled, 0), context.Canceled) {
		return errors.New("PCM seek cancellation differs")
	}
	idx, err := library.New(host, audio.WAVDecoder{}, &librarySource{})
	if err != nil {
		return err
	}
	batch, err := idx.Next(ctx, library.MaxBatch)
	if err != nil || !batch.Done || len(batch.Results) != 2 || batch.Results[0].Status != "indexed" || batch.Results[1].Status != "duplicate" || batch.Results[0].Track.ID != info.SHA256 || batch.Results[0].Track.Title != "Generated" {
		return errors.New("library identity/fallback differs")
	}
	return nil
}

type librarySource struct{ next int }

func (s *librarySource) Next(ctx context.Context) (library.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return library.Candidate{}, err
	}
	if s.next == 2 {
		return library.Candidate{}, io.EOF
	}
	s.next++
	return library.Candidate{Handle: "source", Name: "Generated.wav"}, nil
}
