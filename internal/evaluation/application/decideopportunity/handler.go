// Package decideopportunity approves or rejects one opportunity (BR-20).
package decideopportunity

import (
	"context"
	"fmt"
	"time"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
)

// Command is a decision.
type Command struct {
	Term    string
	Country string
	Entity  string
	Approve bool
	Note    string
}

// Result reports the final state.
type Result struct {
	Key   hotsearch.Key
	State opportunity.State
}

// Handler makes the decision. The gate is computed here from the discovery
// aggregate, so the evaluation context never imports a use case.
type Handler struct {
	keywords hotsearch.Repository
	opps     opportunity.Repository
	now      func() time.Time
}

// New builds the handler.
func New(keywords hotsearch.Repository, opps opportunity.Repository, now func() time.Time) *Handler {
	return &Handler{keywords: keywords, opps: opps, now: now}
}

// Execute applies the decision.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	key, err := support.KeyFromStrings(cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Key = key

	o, err := h.opps.Get(ctx, key)
	if err != nil {
		return out, fmt.Errorf("load opportunity %s: %w", key, err)
	}
	when := support.Clock(h.now)

	if cmd.Approve {
		keyword, err := h.keywords.Get(ctx, key)
		if err != nil {
			return out, fmt.Errorf("load keyword %s: %w", key, err)
		}
		gate := opportunity.Gate{
			TrendValid:  keyword.Verdict().AllowsSeed(),
			TooBroad:    keyword.TooBroad(),
			MinEvidence: 1,
		}
		if err := o.Approve(gate, cmd.Note, when); err != nil {
			return out, err
		}
	} else if err := o.Reject(cmd.Note, when); err != nil {
		return out, err
	}

	if err := h.opps.Save(ctx, o); err != nil {
		return out, fmt.Errorf("save opportunity %s: %w", key, err)
	}
	out.State = o.State()
	return out, nil
}
