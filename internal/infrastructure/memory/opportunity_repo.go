package memory

import (
	"context"
	"sync"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
)

// OpportunityRepository is an in-memory opportunity.Repository.
type OpportunityRepository struct {
	mu    sync.RWMutex
	items map[string]opportunity.Snapshot
}

// NewOpportunityRepository builds an empty store.
func NewOpportunityRepository() *OpportunityRepository {
	return &OpportunityRepository{items: map[string]opportunity.Snapshot{}}
}

// Get returns a rehydrated copy.
func (r *OpportunityRepository) Get(_ context.Context, key hotsearch.Key) (*opportunity.Opportunity, error) {
	r.mu.RLock()
	snapshot, ok := r.items[key.String()]
	r.mu.RUnlock()
	if !ok {
		return nil, opportunity.ErrNotFound
	}
	return opportunity.Rehydrate(snapshot)
}

// Save stores a copy.
func (r *OpportunityRepository) Save(_ context.Context, o *opportunity.Opportunity) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[o.Key().String()] = o.Snapshot()
	return nil
}

// Exists reports presence.
func (r *OpportunityRepository) Exists(_ context.Context, key hotsearch.Key) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[key.String()]
	return ok, nil
}
