// Package judgetrend classifies a keyword's trend (BR-01).
package judgetrend

import (
	"context"
	"fmt"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
)

// Command is the intent to classify one keyword.
type Command struct {
	Term    string
	Country string
	Entity  string
}

// Result surfaces the verdict and what still blocks a confident answer.
type Result struct {
	Keyword    hotsearch.Key
	Verdict    hotsearch.Verdict
	State      hotsearch.State
	Missing    []hotsearch.Window
	AllowsSeed bool
	BrandShare float64
	ChildCount int
	Crowded    bool
}

// Handler runs the trend cross-check.
type Handler struct {
	repo   hotsearch.Repository
	policy hotsearch.TrendPolicy
}

// New builds the handler.
func New(repo hotsearch.Repository, policy hotsearch.TrendPolicy) *Handler {
	return &Handler{repo: repo, policy: policy}
}

// Execute computes and stores the verdict. It never upgrades a terminal keyword.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	k := loaded.Keyword
	out.Keyword = loaded.Key
	out.Verdict = k.AssessTrend(h.policy)
	out.State = k.State()
	out.AllowsSeed = out.Verdict.AllowsSeed()
	out.Missing = missingWindows(k)

	// BR-18 needs the neighborhood, which only the repository can load.
	children, err := h.repo.FindChildren(ctx, loaded.Key)
	if err != nil {
		return out, fmt.Errorf("load children of %s: %w", loaded.Key, err)
	}
	if len(children) > 0 {
		brands := 0
		for _, child := range children {
			if hotsearch.IsBrandMention(child.Key().Term.String()) {
				brands++
			}
		}
		k.AnalyzeChildren(len(children), brands)
		out.BrandShare = k.Signals().BrandShare
		out.ChildCount = k.Signals().ChildCount
		out.Crowded = k.Signals().IsCrowded(h.policy)
	}
	if err := loaded.Save(ctx, h.repo); err != nil {
		return out, err
	}
	return out, nil
}

// missingWindows lists the BR-01 required windows that have no data yet.
func missingWindows(k *hotsearch.Keyword) []hotsearch.Window {
	required := []hotsearch.Window{hotsearch.Window30D, hotsearch.Window90D, hotsearch.Window12M}
	var missing []hotsearch.Window
	for _, w := range required {
		if _, ok := k.Observation(w); !ok {
			missing = append(missing, w)
		}
	}
	return missing
}
