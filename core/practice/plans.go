package practice

import (
	"context"
	"errors"
	"math"

	"github.com/NLstn/riffine/core/engine"
	"github.com/NLstn/riffine/core/project"
)

// Session owns saved-plan lookup and source-region selection. Hosts send an ID,
// never reconstruct a ladder or choose its next speed from UI timers.
func (s *Session) dispatchPracticePlan(ctx context.Context, command Command, p project.Project) error {
	attachment := s.live
	before := attachment.transport.Snapshot()
	if before.State == "error" || attachment.dirty {
		return failure(HostFailure, errors.New("transport requires recovery"))
	}
	reject := func(err error) error {
		if errors.Is(err, engine.ErrUnsupportedCues) || errors.Is(err, engine.ErrUnsupportedMetronome) {
			return failure(UnsupportedCapability, err)
		}
		attachment.dirty = attachment.transport.Snapshot().State == "error"
		s.collectTransportEvents()
		if attachment.dirty {
			return failure(HostFailure, err)
		}
		return failure(InvalidCommand, err)
	}
	switch command.Type {
	case "select_practice_plan":
		saved, compiled, err := p.CompilePracticePlan(*command.PlanID)
		if err != nil {
			return failure(InvalidCommand, err)
		}
		config := s.transportConfig(p, before.Config.OutputRate)
		config.Cues, err = p.ResolvePlanCues(saved)
		if err != nil {
			return failure(InvalidCommand, err)
		}
		for _, region := range p.Annotations.Regions {
			if region.ID == saved.RegionID {
				config.LoopEnabled = true
				config.LoopStart = int64(math.Round(region.Start * float64(config.SampleRate)))
				config.LoopEnd = int64(math.Round(region.End * float64(config.SampleRate)))
				break
			}
		}
		s.collectTransportEvents()
		if err := attachment.transport.LoadPlan(ctx, config, compiled.Definition()); err != nil {
			return reject(err)
		}
		attachment.activePlan = &saved
	case "resume_practice_plan":
		if attachment.activePlan == nil {
			return failure(InvalidCommand, errors.New("no practice plan is selected"))
		}
		if _, err := attachment.transport.Dispatch(ctx, engine.TransportCommand{Version: engine.TransportVersion, Sequence: before.Sequence + 1, Type: "resume_plan"}); err != nil {
			return reject(err)
		}
	case "clear_practice_plan":
		if attachment.activePlan == nil {
			return failure(InvalidCommand, errors.New("no practice plan is selected"))
		}
		// Suspend first so acknowledgements arriving at the boundary are drained
		// before Load clears its resource-scoped event queue.
		if before.State == "playing" {
			if _, err := attachment.transport.Dispatch(ctx, engine.TransportCommand{Version: engine.TransportVersion, Sequence: before.Sequence + 1, Type: "pause"}); err != nil {
				return reject(err)
			}
		}
		s.collectTransportEvents()
		if err := attachment.transport.Load(ctx, s.transportConfig(p, before.Config.OutputRate)); err != nil {
			return reject(err)
		}
		attachment.activePlan = nil
	}
	attachment.dirty = false
	return nil
}
