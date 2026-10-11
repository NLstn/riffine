package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"unicode/utf8"

	"github.com/NLstn/riffine/core/ladder"
)

const MaxPracticePlans = 256

// Only definitions are persisted. Runtime generation, counts and pause state
// belong to the transport and are reset when a new run is explicitly started.
type PracticePlan struct {
	ID       string            `json:"id"`
	Label    string            `json:"label"`
	RegionID string            `json:"regionId"`
	Ladder   ladder.Definition `json:"ladder"`
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
		if !annotationID.MatchString(plan.ID) || ids[plan.ID] || !regions[plan.RegionID] || len(plan.Label) > 1024 || !utf8.ValidString(plan.Label) {
			return errors.New("invalid practice plan ID, label or region reference")
		}
		ids[plan.ID] = true
		if _, err := ladder.Compile(plan.Ladder); err != nil {
			return err
		}
	}
	return nil
}
func ClonePlans(plans []PracticePlan) []PracticePlan { return slices.Clone(plans) }

func (p Project) CompilePracticePlan(id string) (PracticePlan, *ladder.Plan, error) {
	if err := p.Validate(); err != nil {
		return PracticePlan{}, nil, err
	}
	for _, plan := range p.PracticePlans {
		if plan.ID == id {
			compiled, err := ladder.Compile(plan.Ladder)
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
