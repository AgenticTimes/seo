package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/infrastructure/sqlite"
	"seo/internal/shared/kernel"
)

func newOpportunityRepo(t *testing.T) *sqlite.OpportunityRepository {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "opps.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewOpportunityRepository(db)
}

func oppKey(t *testing.T, term string) hotsearch.Key {
	t.Helper()
	normalized, err := kernel.NewTerm(term)
	if err != nil {
		t.Fatal(err)
	}
	key, err := hotsearch.NewKey(normalized, kernel.CountryWorldwide, kernel.EntityTopic)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestOpportunityRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newOpportunityRepo(t)
	key := oppKey(t, "agent memory")

	o, err := opportunity.New(key, hotsearch.IntentInformational, hotsearch.VerdictEmerging, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	e, _ := opportunity.NewEvidence(opportunity.SourceReddit, "https://reddit.com/r/x", "asks", time.Now())
	if err := o.AttachEvidence(e); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, o); err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State() != opportunity.StateEvidence || len(loaded.Evidence()) != 1 {
		t.Fatalf("loaded = %+v", loaded.Snapshot())
	}
	if loaded.Evidence()[0].Source != opportunity.SourceReddit {
		t.Fatalf("evidence = %+v", loaded.Evidence())
	}

	// Upsert must replace evidence, not append or drop it.
	e2, _ := opportunity.NewEvidence(opportunity.SourceCompetitor, "https://example.com/comp", "covers it", time.Now())
	if err := loaded.AttachEvidence(e2); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	again, err := repo.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Evidence()) != 2 || again.State() != opportunity.StateEvidence {
		t.Fatalf("after upsert = %+v", again.Snapshot())
	}

	exists, err := repo.Exists(ctx, key)
	if err != nil || !exists {
		t.Fatalf("exists = %v, %v", exists, err)
	}
	if _, err := repo.Get(ctx, oppKey(t, "missing")); !errors.Is(err, opportunity.ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}
