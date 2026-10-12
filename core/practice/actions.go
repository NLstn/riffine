package practice

import (
	"context"
	"errors"
	"math"

	"github.com/NLstn/riffine/core/project"
)

func practiceAction(kind string) bool {
	switch kind {
	case "rewind_phrase", "seek_backward", "seek_forward", "set_a", "set_b", "next_region", "speed_down", "speed_up", "practice_reset":
		return true
	}
	return false
}

// These actions are shared product policy. Hosts discover inputs; they never
// choose speed increments, seek bounds, region order or reset defaults.
func (s *Session) dispatchPracticeAction(ctx context.Context, kind string, p project.Project) (project.Project, error) {
	live := s.live
	if live.dirty || live.transport.Snapshot().State == "error" {
		return p, failure(HostFailure, errors.New("transport requires recovery"))
	}
	if _, err := live.transport.Poll(ctx); err != nil {
		live.dirty = live.transport.Snapshot().State == "error"
		s.collectTransportEvents()
		return p, failure(HostFailure, err)
	}
	s.collectTransportEvents()
	before := live.transport.Snapshot()
	start, end := int64(0), before.Config.Frames
	if before.Config.LoopEnabled {
		start, end = before.Config.LoopStart, before.Config.LoopEnd
	}
	switch kind {
	case "rewind_phrase", "seek_backward", "seek_forward":
		position := start
		if kind == "seek_backward" {
			position = max(start, before.SourceFrame-int64(before.Config.SampleRate))
		}
		if kind == "seek_forward" {
			position = min(end-1, before.SourceFrame+int64(before.Config.SampleRate))
		}
		return p, s.dispatchTransport(ctx, Command{Type: "seek", Position: &position}, p)
	case "speed_down", "speed_up":
		delta := -5.0
		if kind == "speed_up" {
			delta = 5
		}
		speed := min(2.0, max(.25, math.Round((before.Config.Speed+delta/100)*1e12)/1e12))
		if speed == before.Config.Speed {
			return p, failure(InvalidCommand, errors.New("speed is already at its supported limit"))
		}
		p.Settings.Speed = speed
		return p, s.dispatchTransport(ctx, Command{Type: "set_parameters", Speed: &speed}, p)
	case "set_a", "set_b":
		if kind == "set_a" {
			start = before.SourceFrame
		} else {
			end = before.SourceFrame
		}
		p.Settings.Loop = project.Loop{Enabled: true, Start: float64(start) / float64(before.Config.SampleRate), End: float64(end) / float64(before.Config.SampleRate)}
		if _, err := project.Encode(p); err != nil {
			return p, failure(InvalidCommand, err)
		}
		return p, s.dispatchTransport(ctx, Command{Type: "set_loop", Loop: &p.Settings.Loop}, p)
	case "next_region":
		regions := p.Annotations.Regions
		if len(regions) == 0 {
			return p, failure(InvalidCommand, errors.New("save a named phrase before choosing the next region"))
		}
		next := 0
		for i, region := range regions {
			a := int64(math.Round(region.Start * float64(before.Config.SampleRate)))
			b := int64(math.Round(region.End * float64(before.Config.SampleRate)))
			if before.Config.LoopEnabled && a == start && b == end {
				next = (i + 1) % len(regions)
				break
			}
		}
		region := regions[next]
		p.Settings.Loop = project.Loop{Enabled: true, Start: region.Start, End: region.End}
	case "practice_reset":
		p.Settings = project.Settings{Speed: 1, Loop: project.Loop{End: p.Track.Audio.Duration()}}
	}
	if _, err := project.Encode(p); err != nil {
		return p, failure(InvalidCommand, err)
	}
	// Preserve completions delivered by suspension before Load clears its events.
	if before.State == "playing" {
		if err := s.dispatchTransport(ctx, Command{Type: "pause"}, p); err != nil {
			return p, err
		}
		s.collectTransportEvents()
	}
	if err := live.transport.Load(ctx, s.transportConfig(p, before.Config.OutputRate)); err != nil {
		live.dirty = live.transport.Snapshot().State == "error"
		s.collectTransportEvents()
		return p, failure(HostFailure, err)
	}
	live.activePlan = nil
	operation := "stop"
	if kind == "next_region" && before.State == "playing" {
		operation = "play"
	}
	if err := s.dispatchTransport(ctx, Command{Type: operation}, p); err != nil {
		// Load may have installed new device settings before a later cancellation.
		// Keep committed project settings authoritative until explicit recovery.
		live.dirty = true
		return p, failure(HostFailure, err)
	}
	return p, nil
}
