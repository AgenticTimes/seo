// Package memory provides in-memory adapters for tests and demos.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"seo/internal/discovery/domain/hotsearch"
)

// KeywordRepository is an in-memory hotsearch.Repository.
type KeywordRepository struct {
	mu    sync.RWMutex
	items map[string]hotsearch.Snapshot
}

// NewKeywordRepository builds an empty store.
func NewKeywordRepository() *KeywordRepository {
	return &KeywordRepository{items: map[string]hotsearch.Snapshot{}}
}

// Get returns a rehydrated copy.
func (r *KeywordRepository) Get(_ context.Context, key hotsearch.Key) (*hotsearch.Keyword, error) {
	r.mu.RLock()
	snapshot, ok := r.items[key.String()]
	r.mu.RUnlock()
	if !ok {
		return nil, hotsearch.ErrNotFound
	}
	return hotsearch.Rehydrate(snapshot)
}

// Save stores a copy of the aggregate state.
func (r *KeywordRepository) Save(_ context.Context, k *hotsearch.Keyword) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[k.Key().String()] = k.Snapshot()
	return nil
}

// Exists reports presence by natural key.
func (r *KeywordRepository) Exists(_ context.Context, key hotsearch.Key) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[key.String()]
	return ok, nil
}

// FindChildren lists direct children.
func (r *KeywordRepository) FindChildren(_ context.Context, parent hotsearch.Key) ([]*hotsearch.Keyword, error) {
	return r.filter(func(s hotsearch.Snapshot) bool {
		return s.Parent != nil && s.Parent.String() == parent.String()
	})
}

// ListByState lists one pipeline state.
func (r *KeywordRepository) ListByState(_ context.Context, state hotsearch.State, limit int) ([]*hotsearch.Keyword, error) {
	out, err := r.filter(func(s hotsearch.Snapshot) bool { return s.State == state })
	if err != nil {
		return nil, err
	}
	return truncate(out, limit), nil
}

// ListAll lists every keyword, shallowest first.
func (r *KeywordRepository) ListAll(_ context.Context, limit int) ([]*hotsearch.Keyword, error) {
	out, err := r.filter(func(hotsearch.Snapshot) bool { return true })
	if err != nil {
		return nil, err
	}
	return truncate(out, limit), nil
}

// ListStale lists keywords discovered before the cutoff.
func (r *KeywordRepository) ListStale(_ context.Context, before time.Time) ([]*hotsearch.Keyword, error) {
	return r.filter(func(s hotsearch.Snapshot) bool { return s.DiscoveredAt.Before(before) })
}

func (r *KeywordRepository) filter(keep func(hotsearch.Snapshot) bool) ([]*hotsearch.Keyword, error) {
	r.mu.RLock()
	snapshots := make([]hotsearch.Snapshot, 0, len(r.items))
	for _, s := range r.items {
		if keep(s) {
			snapshots = append(snapshots, s)
		}
	}
	r.mu.RUnlock()

	sort.SliceStable(snapshots, func(i, j int) bool {
		if snapshots[i].Depth != snapshots[j].Depth {
			return snapshots[i].Depth < snapshots[j].Depth
		}
		return snapshots[i].Key.String() < snapshots[j].Key.String()
	})
	out := make([]*hotsearch.Keyword, 0, len(snapshots))
	for _, s := range snapshots {
		k, err := hotsearch.Rehydrate(s)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

func truncate(in []*hotsearch.Keyword, limit int) []*hotsearch.Keyword {
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}
