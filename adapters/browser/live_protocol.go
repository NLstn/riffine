package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/NLstn/riffine/core/engine"
)

type liveAck struct {
	Generation string `json:"generation"`
	Sequence   string `json:"sequence"`
	Position   *int64 `json:"consumedOutputFrame"`
}

func decodeLiveAcks(text string, limit int) ([]engine.OutputAcknowledgement, error) {
	if len(text) > 16384 {
		return nil, errors.New("live acknowledgements exceed bound")
	}
	var input []liveAck
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("trailing live acknowledgement data")
	}
	if limit < 1 || limit > 128 {
		return nil, errors.New("invalid live acknowledgement limit")
	}
	if len(input) > limit {
		return nil, errors.New("live acknowledgement batch exceeds bound")
	}
	result := make([]engine.OutputAcknowledgement, 0, len(input))
	for _, a := range input {
		generation, err := strconv.ParseUint(a.Generation, 10, 64)
		if err != nil || generation == 0 || strconv.FormatUint(generation, 10) != a.Generation {
			return nil, errors.New("invalid live acknowledgement generation")
		}
		sequence, err := strconv.ParseUint(a.Sequence, 10, 64)
		if err != nil || sequence == 0 || strconv.FormatUint(sequence, 10) != a.Sequence {
			return nil, errors.New("invalid live acknowledgement sequence")
		}
		if a.Position == nil || *a.Position < 0 || *a.Position > 1<<53-1 {
			return nil, errors.New("invalid live acknowledgement position")
		}
		result = append(result, engine.OutputAcknowledgement{Generation: generation, Sequence: sequence, ConsumedOutputFrame: *a.Position})
	}
	return result, nil
}
