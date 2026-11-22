// Package createopportunity opens a candidate from an eligible keyword.
package createopportunity

import (
	"context"
	"errors"
	"fmt"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/shared/kernel"
)

// Command is the intent to evaluate one keyword.
type Command struct {
	Term    string
	Country string
	Entity  string
}

// Result reports whether a new candidate was created.
type Result struct {
	Key     hotsearch.Key
	State   opportunity.State
	Created bool
}

// Handler opens an opportunity.
type Handler struct {
	keywords hotsearch.Repository
	opps     opportunity.Repository
	now      func() time.Time
}

// New builds the handler.
func New(keywords hotsearch.Repository, opps opportunity.Repository, now func() time.Time) *Handler {
	return &Handler{keywords: keywords, opps: opps, now: now}
}

// Execute is idempotent: an existing decision is returned unchanged.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	key, err := keyFrom(cmd)
	if err != nil {
		return out, err
	}
	out.Key = key

	existing, err := h.opps.Get(ctx, key)
	if err == nil {
		out.State = existing.State()
		return out, nil
	}
	if !errors.Is(err, opportunity.ErrNotFound) {
		return out, fmt.Errorf("load opportunity %s: %w", key, err)
	}

	keyword, err := h.keywords.Get(ctx, key)
	if err != nil {
		return out, fmt.Errorf("load keyword %s: %w", key, err)
	}
	if keyword.TooBroad() {
		return out, fmt.Errorf("%w: keyword is too broad", opportunity.ErrNotEligible)
	}
	candidate, err := opportunity.New(key, keyword.Signals().Intent, keyword.Verdict(), now(h.now))
	if err != nil {
		return out, err
	}
	if err := h.opps.Save(ctx, candidate); err != nil {
		return out, fmt.Errorf("save opportunity %s: %w", key, err)
	}
	out.State = candidate.State()
	out.Created = true
	return out, nil
}

func keyFrom(cmd Command) (hotsearch.Key, error) {
	term, err := kernel.NewTerm(cmd.Term)
	if err != nil {
		return hotsearch.Key{}, err
	}
	country := kernel.CountryWorldwide
	if cmd.Country != "" {
		country, err = kernel.NewCountry(cmd.Country)
		if err != nil {
			return hotsearch.Key{}, err
		}
	}
	entity := kernel.EntityTopic
	if cmd.Entity != "" {
		entity, err = kernel.ParseEntityType(cmd.Entity)
		if err != nil {
			return hotsearch.Key{}, err
		}
	}
	return hotsearch.NewKey(term, country, entity)
}

func now(clock func() time.Time) time.Time {
	if clock == nil {
		return time.Now().UTC()
	}
	return clock().UTC()
}
