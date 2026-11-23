// Package discardkeyword exits a keyword from the discovery path with a reason.
package discardkeyword

import (
	"context"
	"fmt"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
)

// Command requires a reason.
type Command struct {
	Term    string
	Country string
	Entity  string
	Reason  string
}

// Result reports the new state.
type Result struct {
	Keyword hotsearch.Key
	State   hotsearch.State
}

// Handler applies the discard.
type Handler struct {
	repo hotsearch.Repository
}

// New builds the handler.
func New(repo hotsearch.Repository) *Handler { return &Handler{repo: repo} }

// Execute loads, discards and saves.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Keyword = loaded.Key
	if err := loaded.Keyword.Discard(cmd.Reason); err != nil {
		return out, fmt.Errorf("discard %s: %w", loaded.Key, err)
	}
	if err := loaded.Save(ctx, h.repo); err != nil {
		return out, err
	}
	out.State = loaded.Keyword.State()
	return out, nil
}
