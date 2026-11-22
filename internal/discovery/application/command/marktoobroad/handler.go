// Package marktoobroad applies BR-04: a word too vague to carry intent is
// closed instead of becoming an opportunity.
package marktoobroad

import (
	"context"
	"fmt"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
)

// Command requires an explicit reason.
type Command struct {
	Term    string
	Country string
	Entity  string
	Reason  string
}

// Result reports the closed keyword.
type Result struct {
	Keyword hotsearch.Key
	State   hotsearch.State
}

// Handler marks a keyword too broad.
type Handler struct {
	repo hotsearch.Repository
}

// New builds the handler.
func New(repo hotsearch.Repository) *Handler { return &Handler{repo: repo} }

// Execute applies the rule and stores it.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Keyword = loaded.Key
	if err := loaded.Keyword.MarkTooBroad(cmd.Reason); err != nil {
		return out, fmt.Errorf("mark %s: %w", loaded.Key, err)
	}
	if err := loaded.Save(ctx, h.repo); err != nil {
		return out, err
	}
	out.State = loaded.Keyword.State()
	return out, nil
}
