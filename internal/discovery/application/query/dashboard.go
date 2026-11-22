// Package query serves read models for the UI. Queries never mutate.
package query

import (
	"context"
	"fmt"
	"sort"

	"seo/internal/discovery/domain/hotsearch"
)

// Node is one tree node for rendering.
type Node struct {
	Keyword  *hotsearch.Keyword
	Children []*Node
}

// Stats aggregates the pipeline for the header cards.
type Stats struct {
	Total          int
	Discovered     int
	Validated      int
	SeedCandidates int
	TooBroad       int
	Discarded      int
}

// Dashboard is the single read model the web page needs.
type Dashboard struct {
	Roots      []*Node
	All        []*hotsearch.Keyword
	Queue      []*hotsearch.Keyword
	Candidates []*hotsearch.Keyword
	Stats      Stats
}

// DashboardHandler builds the dashboard read model.
type DashboardHandler struct {
	repo  hotsearch.Repository
	limit int
}

// NewDashboard builds the query.
func NewDashboard(repo hotsearch.Repository) *DashboardHandler {
	return &DashboardHandler{repo: repo, limit: 500}
}

// Execute loads everything, orders it deterministically and builds the tree.
func (h *DashboardHandler) Execute(ctx context.Context) (Dashboard, error) {
	all, err := h.repo.ListAll(ctx, h.limit)
	if err != nil {
		return Dashboard{}, fmt.Errorf("list keywords: %w", err)
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Depth() != all[j].Depth() {
			return all[i].Depth() < all[j].Depth()
		}
		return all[i].Key().String() < all[j].Key().String()
	})

	nodes := make(map[string]*Node, len(all))
	var out Dashboard
	out.All = all
	for _, k := range all {
		node := &Node{Keyword: k}
		nodes[k.Key().String()] = node
		switch k.State() {
		case hotsearch.StateDiscovered:
			out.Stats.Discovered++
			out.Queue = append(out.Queue, k)
		case hotsearch.StateValidated:
			out.Stats.Validated++
			out.Queue = append(out.Queue, k)
		case hotsearch.StateSeedCandidate:
			out.Stats.SeedCandidates++
			out.Candidates = append(out.Candidates, k)
		case hotsearch.StateTooBroad:
			out.Stats.TooBroad++
		case hotsearch.StateDiscarded:
			out.Stats.Discarded++
		}
	}
	out.Stats.Total = len(all)

	// Attach children after all nodes exist so a missing parent never drops a node.
	for _, k := range all {
		node := nodes[k.Key().String()]
		parent := k.Parent()
		if parent == nil {
			out.Roots = append(out.Roots, node)
			continue
		}
		if parentNode, ok := nodes[parent.String()]; ok {
			parentNode.Children = append(parentNode.Children, node)
		} else {
			out.Roots = append(out.Roots, node)
		}
	}

	sort.SliceStable(out.Queue, func(i, j int) bool {
		return out.Queue[i].Key().String() < out.Queue[j].Key().String()
	})
	sort.SliceStable(out.Candidates, func(i, j int) bool {
		return out.Candidates[i].Key().String() < out.Candidates[j].Key().String()
	})
	return out, nil
}
