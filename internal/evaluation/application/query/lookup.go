// Package query holds read models for the evaluation context.
package query

import (
	"context"
	"errors"
	"fmt"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
)

// Lookup returns the opportunity for one keyword, or nil when none exists.
type Lookup struct {
	opps opportunity.Repository
}

// NewLookup builds the query.
func NewLookup(opps opportunity.Repository) *Lookup {
	return &Lookup{opps: opps}
}

// Execute never returns ErrNotFound to the caller.
func (l *Lookup) Execute(ctx context.Context, key hotsearch.Key) (*opportunity.Opportunity, error) {
	o, err := l.opps.Get(ctx, key)
	if errors.Is(err, opportunity.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load opportunity %s: %w", key, err)
	}
	return o, nil
}
