package query

import (
	"context"
	"fmt"

	"seo/internal/discovery/domain/hotsearch"
)

// Advice is a read-only suggestion. It never changes a keyword.
type Advice struct {
	Suggest  bool
	Reasons  []string
	Children int
}

// Advisor loads a keyword's children and applies BR-21.
type Advisor struct {
	repo hotsearch.Repository
}

// NewAdvisor builds the query.
func NewAdvisor(repo hotsearch.Repository) *Advisor {
	return &Advisor{repo: repo}
}

// Execute needs only the key; the caller already has the aggregate.
func (a *Advisor) Execute(ctx context.Context, key hotsearch.Key) (Advice, error) {
	children, err := a.repo.FindChildren(ctx, key)
	if err != nil {
		return Advice{}, fmt.Errorf("load children of %s: %w", key, err)
	}
	terms := make([]string, 0, len(children))
	for _, child := range children {
		terms = append(terms, child.Key().Term.String())
	}
	suggest, reasons := hotsearch.SuggestTooBroad(key.Term.String(), terms)
	return Advice{Suggest: suggest, Reasons: reasons, Children: len(children)}, nil
}
