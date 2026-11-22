// Package attacheevidence records external proof for one opportunity (BR-11).
package attacheevidence

import (
	"context"
	"fmt"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/shared/kernel"
)

// Command records one evidence item.
type Command struct {
	Term    string
	Country string
	Entity  string
	Source  string
	URL     string
	Summary string
}

// Result reports the new state.
type Result struct {
	Key   hotsearch.Key
	State opportunity.State
	Count int
}

// Handler attaches evidence.
type Handler struct {
	opps opportunity.Repository
	now  func() time.Time
}

// New builds the handler.
func New(opps opportunity.Repository, now func() time.Time) *Handler {
	return &Handler{opps: opps, now: now}
}

// Execute loads, attaches and saves.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	key, err := keyFrom(cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Key = key

	source, err := opportunity.ParseSourceType(cmd.Source)
	if err != nil {
		return out, err
	}
	when := time.Now().UTC()
	if h.now != nil {
		when = h.now().UTC()
	}
	evidence, err := opportunity.NewEvidence(source, cmd.URL, cmd.Summary, when)
	if err != nil {
		return out, err
	}

	o, err := h.opps.Get(ctx, key)
	if err != nil {
		return out, fmt.Errorf("load opportunity %s: %w", key, err)
	}
	if err := o.AttachEvidence(evidence); err != nil {
		return out, err
	}
	if err := h.opps.Save(ctx, o); err != nil {
		return out, fmt.Errorf("save opportunity %s: %w", key, err)
	}
	out.State = o.State()
	out.Count = len(o.Evidence())
	return out, nil
}

func keyFrom(term, country, entity string) (hotsearch.Key, error) {
	t, err := kernel.NewTerm(term)
	if err != nil {
		return hotsearch.Key{}, err
	}
	c := kernel.CountryWorldwide
	if country != "" {
		c, err = kernel.NewCountry(country)
		if err != nil {
			return hotsearch.Key{}, err
		}
	}
	e := kernel.EntityTopic
	if entity != "" {
		e, err = kernel.ParseEntityType(entity)
		if err != nil {
			return hotsearch.Key{}, err
		}
	}
	return hotsearch.NewKey(t, c, e)
}
