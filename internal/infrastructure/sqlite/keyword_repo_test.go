package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/infrastructure/sqlite"
	"seo/internal/shared/kernel"
)

func newRepo(t *testing.T) *sqlite.KeywordRepository {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewKeywordRepository(db)
}

func key(t *testing.T, term string) hotsearch.Key {
	t.Helper()
	normalized, err := kernel.NewTerm(term)
	if err != nil {
		t.Fatal(err)
	}
	k, err := hotsearch.NewKey(normalized, kernel.CountryWorldwide, kernel.EntityTopic)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestSaveGetRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	root := hotsearch.NewSeed(key(t, "ai agent"), time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	obs, err := hotsearch.NewObservation(
		hotsearch.Window30D,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC),
		[]int{10, 20, 30},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.RecordObservation(obs); err != nil {
		t.Fatal(err)
	}
	root.AssessTrend(hotsearch.DefaultTrendPolicy())
	if err := repo.Save(ctx, root); err != nil {
		t.Fatalf("save root: %v", err)
	}

	child, err := hotsearch.NewChild(root, mustTerm(t, "browser agent"), hotsearch.SourceBreakout, time.Now(), hotsearch.DefaultExpansionPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, child); err != nil {
		t.Fatalf("save child: %v", err)
	}

	loaded, err := repo.Get(ctx, root.Key())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.Verdict() != root.Verdict() {
		t.Fatalf("verdict = %s, want %s", loaded.Verdict(), root.Verdict())
	}
	if !reflect.DeepEqual(loaded.Signals(), root.Signals()) {
		t.Fatalf("signals = %+v, want %+v", loaded.Signals(), root.Signals())
	}
	if got, ok := loaded.Observation(hotsearch.Window30D); !ok || len(got.Samples()) != 3 {
		t.Fatalf("observation round-trip failed: %v %v", got, ok)
	}

	children, err := repo.FindChildren(ctx, root.Key())
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].Source() != hotsearch.SourceBreakout {
		t.Fatalf("children = %+v", children)
	}

	loadedChild, err := repo.Get(ctx, child.Key())
	if err != nil {
		t.Fatal(err)
	}
	if loadedChild.Parent() == nil || !loadedChild.Parent().Equal(root.Key()) {
		t.Fatal("parent not persisted")
	}
	if loadedChild.Depth() != 1 {
		t.Fatalf("depth = %d, want 1", loadedChild.Depth())
	}

	all, err := repo.ListAll(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("list all = %d, want 2", len(all))
	}

	if _, err := repo.Get(ctx, key(t, "missing")); !errors.Is(err, hotsearch.ErrNotFound) {
		t.Fatalf("missing get err = %v, want ErrNotFound", err)
	}
}

func TestSaveUpdatesSignalsOnConflict(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	root := hotsearch.NewSeed(key(t, "ai coding"), time.Now())

	// First save runs before any assessment, so signals are zero.
	if err := repo.Save(ctx, root); err != nil {
		t.Fatal(err)
	}
	// Assess later and save again: the upsert must refresh the signals column.
	recordSeries(t, root)
	if v := root.AssessTrend(hotsearch.DefaultTrendPolicy()); v == hotsearch.VerdictUnknown {
		t.Fatal("expected a verdict")
	}
	if err := repo.Save(ctx, root); err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Get(ctx, root.Key())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Signals().Points30 == 0 {
		t.Fatalf("signals not updated on conflict: %+v", loaded.Signals())
	}
	if !reflect.DeepEqual(loaded.Signals(), root.Signals()) {
		t.Fatalf("signals = %+v, want %+v", loaded.Signals(), root.Signals())
	}
}

func TestSaveReplacesObservations(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)
	root := hotsearch.NewSeed(key(t, "ai coding"), time.Now())

	for _, v := range [][]int{{10, 20}, {20, 30}} {
		obs, err := hotsearch.NewObservation(hotsearch.Window30D, time.Unix(0, 0), time.Unix(1, 0), v)
		if err != nil {
			t.Fatal(err)
		}
		if err := root.RecordObservation(obs); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Save(ctx, root); err != nil {
		t.Fatal(err)
	}
	root2 := hotsearch.NewSeed(key(t, "ai coding"), time.Now())
	obs, _ := hotsearch.NewObservation(hotsearch.Window30D, time.Unix(0, 0), time.Unix(2, 0), []int{40, 50})
	if err := root2.RecordObservation(obs); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, root2); err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.Get(ctx, root.Key())
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Observations(); len(got) != 1 {
		t.Fatalf("observations = %d, want 1", len(got))
	}
	if got, _ := loaded.Observation(hotsearch.Window30D); got.Samples()[0].Int() != 40 {
		t.Fatalf("observation not replaced: %v", got.Samples())
	}
}

func mustTerm(t *testing.T, s string) kernel.Term {
	t.Helper()
	term, err := kernel.NewTerm(s)
	if err != nil {
		t.Fatal(err)
	}
	return term
}

func recordSeries(t *testing.T, k *hotsearch.Keyword) {
	t.Helper()
	start := time.Unix(0, 0)
	end := start.Add(time.Hour)
	for _, window := range []hotsearch.Window{hotsearch.Window30D, hotsearch.Window90D, hotsearch.Window12M} {
		obs, err := hotsearch.NewObservation(window, start, end, []int{10, 12, 14, 16, 18, 20, 22, 25})
		if err != nil {
			t.Fatal(err)
		}
		if err := k.RecordObservation(obs); err != nil {
			t.Fatal(err)
		}
	}
}
