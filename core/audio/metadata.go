package audio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/NLstn/riffine/core/resource"
)

// ValidateInfo checks canonical original identity and generic decoded dimensions.
// Source precision is optional for FLAC/ALAC and absent for lossy codecs; it never means
// the worker's float32 output storage. Metadata cannot advertise an untested codec.
func ValidateInfo(i Info) error {
	hash, err := hex.DecodeString(i.SHA256)
	if err != nil || len(hash) != 32 || i.SHA256 != strings.ToLower(i.SHA256) || i.SampleRate < 8000 || i.SampleRate > 192000 || i.Channels < 1 || i.Channels > 2 || i.Frames < 1 || i.Frames > 1<<48 {
		return errors.New("invalid canonical audio identity or dimensions")
	}
	switch i.Encoding {
	case "wav-pcm":
		if i.Bits != 16 && i.Bits != 24 && i.Bits != 32 {
			return errors.New("unsupported WAV precision")
		}
	case "flac", "alac":
		if i.Bits != 0 && (i.Bits < 4 || i.Bits > 32) {
			return errors.New("invalid lossless source precision")
		}
	case "mp3", "aac", "opus":
		if i.Bits != 0 {
			return errors.New("lossy audio has no fixed source PCM precision")
		}
	default:
		return errors.New("unsupported source codec")
	}
	return nil
}

// Fingerprint streams all original bytes (including tags), with bounded reads.
func Fingerprint(ctx context.Context, source resource.Reader) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.NewSectionReader(contextReader{ctx, source}, 0, source.Size()))
	if err != nil {
		return "", err
	}
	if n != source.Size() {
		return "", errors.New("audio changed or truncated while fingerprinting")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
