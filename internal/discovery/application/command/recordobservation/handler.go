// Package recordobservation stores trend series for one keyword.
package recordobservation

import (
	"context"
	"fmt"
	"time"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
)

// Command is the intent to record observations. An empty Windows list records
// every window, which is what the UI's single button does.
type Command struct {
	Term    string
	Country string
	Entity  string
	Windows []string
}

// Result reports which windows were stored.
type Result struct {
	Keyword  hotsearch.Key
	Recorded []hotsearch.Window
}

// Handler fetches windows and stores them on the aggregate.
type Handler struct {
	repo   hotsearch.Repository
	trends ports.TrendsGateway
	now    func() time.Time
}

// New builds the handler.
func New(repo hotsearch.Repository, trends ports.TrendsGateway, now func() time.Time) *Handler {
	return &Handler{repo: repo, trends: trends, now: now}
}

// Execute loads, records and saves once. A failed window aborts without saving.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	loaded, err := support.Load(ctx, h.repo, cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	k := loaded.Keyword
	out.Keyword = loaded.Key

	windows, err := resolveWindows(cmd.Windows)
	if err != nil {
		return out, err
	}
	for _, window := range windows {
		series, err := h.trends.InterestOverTime(ctx, ports.InterestRequest{
			Term:    loaded.Key.Term,
			Country: loaded.Key.Country,
			Entity:  loaded.Key.Entity,
			Window:  window,
		})
		if err != nil {
			return out, fmt.Errorf("fetch %s for %s: %w", window, loaded.Key, err)
		}
		obs, err := hotsearch.NewObservation(series.Window, series.Start, series.End, series.Values)
		if err != nil {
			return out, fmt.Errorf("build observation %s: %w", window, err)
		}
		if err := k.RecordObservation(obs); err != nil {
			return out, fmt.Errorf("record %s: %w", window, err)
		}
		out.Recorded = append(out.Recorded, window)
	}

	if err := loaded.Save(ctx, h.repo); err != nil {
		return out, err
	}
	return out, nil
}

func resolveWindows(raw []string) ([]hotsearch.Window, error) {
	if len(raw) == 0 {
		return hotsearch.AllWindows(), nil
	}
	out := make([]hotsearch.Window, 0, len(raw))
	for _, item := range raw {
		w, err := hotsearch.ParseWindow(item)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}
