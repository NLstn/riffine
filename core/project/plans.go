package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/NLstn/riffine/core/ladder"
	"github.com/NLstn/riffine/core/timing"
)

const MaxPracticePlans = 256

// Only definitions are persisted. Runtime generation, counts and pause state
// belong to the transport and are reset when a new run is explicitly started.
type PracticePlan struct {
	ID       string                `json:"id"`
	Label    string                `json:"label"`
	RegionID string                `json:"regionId"`
	Ladder   ladder.Definition     `json:"ladder"`
	Cues     *timing.CueDefinition `json:"cues,omitempty"`
}

func (p Project) validatePlans() error {
	if len(p.PracticePlans) > MaxPracticePlans {
		return errors.New("too many practice plans")
	}
	ids := map[string]bool{}
	regions := map[string]bool{}
	for _, r := range p.Annotations.Regions {
		ids[r.ID] = true
		regions[r.ID] = true
	}
	for _, m := range p.Annotations.Markers {
		ids[m.ID] = true
	}
	for _, l := range p.Annotations.Loops {
		ids[l.ID] = true
	}
	for _, b := range p.Annotations.Bookmarks {
		ids[b.ID] = true
	}
	for _, plan := range p.PracticePlans {
		if !annotationID.MatchString(plan.ID) || ids[plan.ID] || len(plan.Label) > 1024 || !utf8.ValidString(plan.Label) {
			return errors.New("invalid practice plan ID or label")
		}
		if !regions[plan.RegionID] {
			return fmt.Errorf("practice plan %q references missing phrase %q; remove or change that plan before removing its phrase", plan.ID, plan.RegionID)
		}
		ids[plan.ID] = true
		cues, err := p.ResolvePlanCues(plan)
		if err != nil {
			return err
		}
		grid, err := cues.Compile(p.Track.Audio.SampleRate, p.Track.Audio.Frames)
		if err != nil {
			return err
		}
		if plan.Cues != nil && plan.Cues.CountInBeats > 0 {
			for _, region := range p.Annotations.Regions {
				if region.ID == plan.RegionID {
					_, _, err = grid.CountIn(context.Background(), timing.Selection{StartFrame: int64(math.Round(region.Start * float64(p.Track.Audio.SampleRate))), EndFrame: int64(math.Round(region.End * float64(p.Track.Audio.SampleRate))), OutputRate: p.Track.Audio.SampleRate, Speed: plan.Ladder.StartSpeed}, plan.Cues.CountInBeats)
					if err != nil {
						return err
					}
					break
				}
			}
		}
		if _, err := ladder.Compile(plan.Ladder); err != nil {
			return err
		}
	}
	return nil
}
func ClonePlans(plans []PracticePlan) []PracticePlan {
	result := slices.Clone(plans)
	for i := range result {
		result[i].Cues = result[i].Cues.Clone()
	}
	return result
}

func (p Project) CompilePracticePlan(id string) (PracticePlan, *ladder.Plan, error) {
	if err := p.Validate(); err != nil {
		return PracticePlan{}, nil, err
	}
	for _, plan := range p.PracticePlans {
		if plan.ID == id {
			compiled, err := ladder.Compile(plan.Ladder)
			plan.Cues = plan.Cues.Clone()
			return plan, compiled, err
		}
	}
	return PracticePlan{}, nil, errors.New("practice plan ID not found")
}
func DecodePracticePlans(r io.Reader) ([]PracticePlan, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxDocumentBytes {
		return nil, errors.New("practice plans exceed 1 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var plans []PracticePlan
	if err = dec.Decode(&plans); err != nil {
		return nil, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, errors.New("unexpected trailing practice plans")
	}
	return plans, nil
}

// Older document schemas deliberately do not accept new cue fields.
type legacyPracticePlan struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	RegionID string            `json:"regionId"`
	Ladder   ladder.Definition `json:"ladder"`
}

func migratePlans(old []legacyPracticePlan) []PracticePlan {
	if old == nil {
		return nil
	}
	result := make([]PracticePlan, len(old))
	for i, p := range old {
		result[i] = PracticePlan{ID: p.ID, Label: p.Label, RegionID: p.RegionID, Ladder: p.Ladder}
	}
	return result
}

// ResolvePlanCues derives source frames/ordinals from saved beat annotations in
// the core. Clients choose this mode; they never reconstruct meter/beat policy.
func (p Project) ResolvePlanCues(plan PracticePlan) (*timing.CueDefinition, error) {
	cues := plan.Cues.Clone()
	if cues == nil || !cues.UseBeatMarkers {
		return cues, nil
	}
	if cues.Grid == nil || cues.Grid.Version != timing.Version || cues.Grid.BeatsPerBar < 1 || cues.Grid.BeatsPerBar > 16 || cues.Grid.SourceBPM != 0 || cues.Grid.OriginFrame != 0 || len(cues.Grid.Markers) != 0 {
		return nil, errors.New("saved-beat timing needs a version-1 meter grid without manual BPM or inline beats")
	}
	meter := int64(cues.Grid.BeatsPerBar)
	for _, marker := range p.Annotations.Markers {
		if marker.Kind == "beat" {
			if marker.Measure < 1 || marker.Beat < 1 || int64(marker.Measure) > (1<<48)/meter || int64(marker.Beat) > meter {
				return nil, fmt.Errorf("beat marker %q needs a positive measure and beat within the plan's %d-beat bar", marker.ID, meter)
			}
			cues.Grid.Markers = append(cues.Grid.Markers, timing.Beat{SourceFrame: int64(math.Round(marker.Position * float64(p.Track.Audio.SampleRate))), Ordinal: (int64(marker.Measure)-1)*meter + int64(marker.Beat) - 1})
		}
	}
	if len(cues.Grid.Markers) == 0 {
		return nil, fmt.Errorf("plan %q needs saved beat markers; add beat markers or choose original-recording BPM", plan.ID)
	}
	sort.Slice(cues.Grid.Markers, func(i, j int) bool { return cues.Grid.Markers[i].SourceFrame < cues.Grid.Markers[j].SourceFrame })
	cues.UseBeatMarkers = false
	return cues, nil
}
