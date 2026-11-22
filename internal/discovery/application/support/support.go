// Package support holds the plumbing shared by the application use cases:
// turning raw form input into a domain key, injecting a clock, and the
// load → mutate → save cycle. It carries no business rules.
package support

import (
	"context"
	"fmt"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/shared/kernel"
)

// KeyFromStrings builds a keyword identity from raw form input, applying the
// defaults: worldwide market and Topic entity (BR-08).
func KeyFromStrings(term, country, entity string) (hotsearch.Key, error) {
	t, err := kernel.NewTerm(term)
	if err != nil {
		return hotsearch.Key{}, err
	}
	c := kernel.CountryWorldwide
	if country != "" {
		c, err = kernel.NewCountry(country)
		if err != nil {
			return hotsearch.Key{}, err
		}
	}
	e := kernel.EntityTopic
	if entity != "" {
		e, err = kernel.ParseEntityType(entity)
		if err != nil {
			return hotsearch.Key{}, err
		}
	}
	return hotsearch.NewKey(t, c, e)
}

// Clock returns now, or the wall clock when unset. Injecting it keeps handlers
// deterministic in tests without a Clock port.
func Clock(now func() time.Time) time.Time {
	if now == nil {
		return time.Now().UTC()
	}
	return now().UTC()
}

// Loaded pairs a command's identity with the aggregate it loaded.
type Loaded struct {
	Key     hotsearch.Key
	Keyword *hotsearch.Keyword
}

// Load resolves the command identity and loads the aggregate in one step, so
// every command shares the same error wrapping.
func Load(ctx context.Context, repo hotsearch.Repository, term, country, entity string) (Loaded, error) {
	key, err := KeyFromStrings(term, country, entity)
	if err != nil {
		return Loaded{}, err
	}
	k, err := repo.Get(ctx, key)
	if err != nil {
		return Loaded{Key: key}, fmt.Errorf("load %s: %w", key, err)
	}
	return Loaded{Key: key, Keyword: k}, nil
}

// Save persists the loaded aggregate.
func (l Loaded) Save(ctx context.Context, repo hotsearch.Repository) error {
	if err := repo.Save(ctx, l.Keyword); err != nil {
		return fmt.Errorf("save %s: %w", l.Key, err)
	}
	return nil
}
