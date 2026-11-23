// Package reopenkeyword returns a decided keyword to the validation queue.
package reopenkeyword

import (
	"context"
	"fmt"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
)

// Command is the intent to reopen one keyword.
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

// Handler reopens a keyword.
type Handler struct {
	repo hotsearch.Repository
}

// New builds the handler.
func New(repo hotsearch.Repository) *Handler { return &Handler{repo: repo} }

// Execute loads, reopens and saves.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Keyword = loaded.Key
	if err := loaded.Keyword.Reopen(); err != nil {
		return out, fmt.Errorf("reopen %s: %w", loaded.Key, err)
	}
	if err := loaded.Save(ctx, h.repo); err != nil {
		return out, err
	}
	out.State = loaded.Keyword.State()
	return out, nil
}
