package query_test

import (
	"context"
	"testing"
	"time"

	"seo/internal/discovery/application/query"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/infrastructure/memory"
	"seo/internal/shared/kernel"
)

func TestAdvisorNeedsChildren(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewKeywordRepository()
	advisor := query.NewAdvisor(repo)

	root := hotsearch.NewSeed(testKey(t, "ai agent"), time.Now())
	if err := repo.Save(ctx, root); err != nil {
		t.Fatal(err)
	}
	advice, err := advisor.Execute(ctx, root.Key())
	if err != nil {
		t.Fatal(err)
	}
	if advice.Suggest {
		t.Fatalf("no children should not suggest: %+v", advice)
	}

	for _, term := range []string{
		"ai coding agent", "browser agent", "computer use",
		"personal ai assistant", "agent memory", "ai agent framework",
	} {
		child, err := hotsearch.NewChild(root, testTerm(t, term), hotsearch.SourceRising, time.Now(), hotsearch.DefaultExpansionPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.Save(ctx, child); err != nil {
			t.Fatal(err)
		}
	}
	advice, err = advisor.Execute(ctx, root.Key())
	if err != nil {
		t.Fatal(err)
	}
	if !advice.Suggest || advice.Children != 6 {
		t.Fatalf("advice = %+v, want suggestion over 6 children", advice)
	}
}

func testTerm(t *testing.T, s string) kernel.Term {
	t.Helper()
	term, err := kernel.NewTerm(s)
	if err != nil {
		t.Fatal(err)
	}
	return term
}

func testKey(t *testing.T, s string) hotsearch.Key {
	t.Helper()
	key, err := hotsearch.NewKey(testTerm(t, s), kernel.CountryWorldwide, kernel.EntityTopic)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
