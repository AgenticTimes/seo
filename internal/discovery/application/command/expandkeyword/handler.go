// Package expandkeyword discovers and records the children of one keyword.
package expandkeyword

import (
	"context"
	"errors"
	"fmt"
	"time"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
)

// Command is the intent to expand exactly one node.
type Command struct {
	Term    string
	Country string
	Entity  string
}

// Result reports the four outcomes of a round.
type Result struct {
	Parent       hotsearch.Key
	Created      []hotsearch.Key
	Skipped      []hotsearch.Key
	DepthLimited []hotsearch.Key
}

// Handler expands a keyword into its next-round children.
type Handler struct {
	repo   hotsearch.Repository
	trends ports.TrendsGateway
	policy hotsearch.ExpansionPolicy
	now    func() time.Time
}

// New builds the handler.
func New(repo hotsearch.Repository, trends ports.TrendsGateway, policy hotsearch.ExpansionPolicy, now func() time.Time) *Handler {
	return &Handler{repo: repo, trends: trends, policy: policy, now: now}
}

// Execute fetches related queries, applies dedupe and the BR-09 depth bound, and
// stores only new nodes. It never mutates the parent.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	parent := loaded.Keyword
	out.Parent = loaded.Key
	if !parent.CanBeExpanded() {
		return out, fmt.Errorf("expand %s: %w", loaded.Key, hotsearch.ErrInvalidTransition)
	}

	related, err := h.trends.RelatedQueries(ctx, ports.RelatedRequest{
		Term:    loaded.Key.Term,
		Country: loaded.Key.Country,
		Entity:  loaded.Key.Entity,
	})
	if err != nil {
		return out, fmt.Errorf("fetch related queries for %s: %w", loaded.Key, err)
	}

	seen := map[string]bool{}
	for _, discovered := range related.Queries {
		if discovered.Term == loaded.Key.Term {
			continue
		}
		childKey, err := hotsearch.NewKey(discovered.Term, loaded.Key.Country, loaded.Key.Entity)
		if err != nil {
			continue
		}
		if seen[childKey.String()] {
			continue
		}
		seen[childKey.String()] = true

		exists, err := h.repo.Exists(ctx, childKey)
		if err != nil {
			return out, fmt.Errorf("check child %s: %w", childKey, err)
		}
		if exists {
			out.Skipped = append(out.Skipped, childKey)
			continue
		}

		child, err := hotsearch.NewChild(parent, discovered.Term, discovered.Source, support.Clock(h.now), h.policy)
		if errors.Is(err, hotsearch.ErrDepthExceeded) {
			out.DepthLimited = append(out.DepthLimited, childKey)
			continue
		}
		if err != nil {
			return out, fmt.Errorf("build child %s: %w", childKey, err)
		}
		if err := h.repo.Save(ctx, child); err != nil {
			return out, fmt.Errorf("save child %s: %w", childKey, err)
		}
		out.Created = append(out.Created, childKey)
	}
	return out, nil
}
