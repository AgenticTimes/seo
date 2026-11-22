// Package promote promotes a validated keyword to the next round.
package promote

import (
	"context"
	"fmt"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
)

// Command is the intent to promote one keyword (BR-09/BR-10).
type Command struct {
	Term    string
	Country string
	Entity  string
}

// Result reports the new state.
type Result struct {
	Keyword hotsearch.Key
	State   hotsearch.State
}

// Handler promotes a keyword to seed candidate.
type Handler struct {
	repo   hotsearch.Repository
	policy hotsearch.ExpansionPolicy
}

// New builds the handler.
func New(repo hotsearch.Repository, policy hotsearch.ExpansionPolicy) *Handler {
	return &Handler{repo: repo, policy: policy}
}

// Execute applies the gate and stores the transition.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Keyword = loaded.Key
	if err := loaded.Keyword.ConfirmAsSeedCandidate(h.policy); err != nil {
		return out, fmt.Errorf("advance %s: %w", loaded.Key, err)
	}
	if err := loaded.Save(ctx, h.repo); err != nil {
		return out, err
	}
	out.State = loaded.Keyword.State()
	return out, nil
}
