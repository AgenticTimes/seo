// Package startexploration plants root seed keywords.
package startexploration

import (
	"context"
	"fmt"
	"time"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
)

// Command is the intent to plant one or more root seeds.
type Command struct {
	Terms   []string
	Country string
	Entity  string
}

// Result reports what was planted and what already existed.
type Result struct {
	Created []hotsearch.Key
	Skipped []hotsearch.Key
}

// Handler starts an exploration. One intent, one handler.
type Handler struct {
	repo hotsearch.Repository
	now  func() time.Time
}

// New builds the handler.
func New(repo hotsearch.Repository, now func() time.Time) *Handler {
	return &Handler{repo: repo, now: now}
}

// Execute plants seeds idempotently: an existing key is skipped, never rewritten.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	seen := map[string]bool{}
	for _, raw := range cmd.Terms {
		if raw == "" {
			continue
		}
		key, err := support.KeyFromStrings(raw, cmd.Country, cmd.Entity)
		if err != nil {
			return out, fmt.Errorf("seed %q: %w", raw, err)
		}
		if seen[key.String()] {
			continue
		}
		seen[key.String()] = true

		exists, err := h.repo.Exists(ctx, key)
		if err != nil {
			return out, fmt.Errorf("check seed %s: %w", key, err)
		}
		if exists {
			out.Skipped = append(out.Skipped, key)
			continue
		}
		if err := h.repo.Save(ctx, hotsearch.NewSeed(key, support.Clock(h.now))); err != nil {
			return out, fmt.Errorf("save seed %s: %w", key, err)
		}
		out.Created = append(out.Created, key)
	}
	return out, nil
}
