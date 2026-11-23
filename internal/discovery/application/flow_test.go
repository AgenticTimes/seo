package application_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"seo/internal/discovery/application/command/discardkeyword"
	"seo/internal/discovery/application/command/expandkeyword"
	"seo/internal/discovery/application/command/judgetrend"
	"seo/internal/discovery/application/command/marktoobroad"
	"seo/internal/discovery/application/command/promote"
	"seo/internal/discovery/application/command/recordobservation"
	"seo/internal/discovery/application/command/reopenkeyword"
	"seo/internal/discovery/application/command/startexploration"
	"seo/internal/discovery/application/query"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/infrastructure/memory"
	"seo/internal/infrastructure/trends"
	"seo/internal/shared/kernel"
)

type flow struct {
	ctx     context.Context
	repo    *memory.KeywordRepository
	start   *startexploration.Handler
	expand  *expandkeyword.Handler
	observe *recordobservation.Handler
	assess  *judgetrend.Handler
	advance *promote.Handler
	broad   *marktoobroad.Handler
	discard *discardkeyword.Handler
	reopen  *reopenkeyword.Handler
	dash    *query.DashboardHandler
}

func newFlow() *flow {
	repo := memory.NewKeywordRepository()
	gateway := trends.NewFake()
	exp := hotsearch.DefaultExpansionPolicy()
	trend := hotsearch.DefaultTrendPolicy()
	return &flow{
		ctx:     context.Background(),
		repo:    repo,
		start:   startexploration.New(repo, nil),
		expand:  expandkeyword.New(repo, gateway, exp, nil),
		observe: recordobservation.New(repo, gateway, nil),
		assess:  judgetrend.New(repo, trend),
		advance: promote.New(repo, exp),
		broad:   marktoobroad.New(repo),
		discard: discardkeyword.New(repo),
		reopen:  reopenkeyword.New(repo),
		dash:    query.NewDashboard(repo),
	}
}

func (f *flow) seed(t *testing.T, term string) hotsearch.Key {
	t.Helper()
	res, err := f.start.Execute(f.ctx, startexploration.Command{Terms: []string{term}})
	if err != nil {
		t.Fatalf("start %q: %v", term, err)
	}
	if len(res.Created) != 1 {
		t.Fatalf("start created %d keys, want 1", len(res.Created))
	}
	return res.Created[0]
}

func TestStartExplorationIsIdempotent(t *testing.T) {
	f := newFlow()
	key := f.seed(t, "AI Agent")

	res, err := f.start.Execute(f.ctx, startexploration.Command{Terms: []string{" ai   agent "}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 0 || len(res.Skipped) != 1 || !res.Skipped[0].Equal(key) {
		t.Fatalf("second start = %+v, want skipped", res)
	}
}

func TestExpandIsIdempotentAndBounded(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")

	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatalf("first expand: %v", err)
	}
	if len(first.Created) == 0 {
		t.Fatal("first expand created nothing")
	}

	second, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatalf("second expand: %v", err)
	}
	if len(second.Created) != 0 {
		t.Fatalf("second expand created %d duplicates", len(second.Created))
	}

	// A freshly discovered child cannot drive a round until promoted.
	childTerm := first.Created[0].Term.String()
	if _, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: childTerm}); !errors.Is(err, hotsearch.ErrInvalidTransition) {
		t.Fatalf("expand unpromoted child err = %v, want ErrInvalidTransition", err)
	}
}

func TestDepthLimitStopsThirdRound(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")
	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatal(err)
	}

	var promoted bool
	for _, key := range first.Created {
		if f.promoteIfEligible(t, key) {
			promoted = true
			second, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: key.Term.String()})
			if err != nil {
				t.Fatalf("expand depth 1: %v", err)
			}
			for _, grandchild := range second.Created {
				if f.promoteIfEligible(t, grandchild) {
					third, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: grandchild.Term.String()})
					if err != nil {
						t.Fatalf("expand depth 2: %v", err)
					}
					if len(third.Created) != 0 {
						t.Fatalf("depth 3 created %d nodes, want 0", len(third.Created))
					}
					return
				}
			}
		}
	}
	if !promoted {
		t.Fatal("expected at least one child to be promotable")
	}
}

func TestAdvanceRequiresValidatedTrend(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")
	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	target := first.Created[0]

	if _, err := f.advance.Execute(f.ctx, promote.Command{Term: target.Term.String()}); !errors.Is(err, hotsearch.ErrNotValidated) {
		t.Fatalf("advance before validation err = %v, want ErrNotValidated", err)
	}

	f.observeAll(t, target)
	assessed, err := f.assess.Execute(f.ctx, judgetrend.Command{Term: target.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(assessed.Missing) != 0 {
		t.Fatalf("missing windows = %v, want none", assessed.Missing)
	}

	if assessed.AllowsSeed {
		if _, err := f.advance.Execute(f.ctx, promote.Command{Term: target.Term.String()}); err != nil {
			t.Fatalf("advance validated: %v", err)
		}
	} else if _, err := f.advance.Execute(f.ctx, promote.Command{Term: target.Term.String()}); !errors.Is(err, hotsearch.ErrNotValidated) {
		t.Fatalf("advance non-qualifying verdict err = %v, want ErrNotValidated", err)
	}
}

func TestTooBroadCannotAdvance(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")
	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	target := first.Created[0]
	f.observeAll(t, target)
	if _, err := f.assess.Execute(f.ctx, judgetrend.Command{Term: target.Term.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.broad.Execute(f.ctx, marktoobroad.Command{Term: target.Term.String(), Reason: "one word"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.advance.Execute(f.ctx, promote.Command{Term: target.Term.String()}); !errors.Is(err, hotsearch.ErrTooBroad) {
		t.Fatalf("advance too broad err = %v, want ErrTooBroad", err)
	}
}

func TestDashboardBuildsTreeAndQueue(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")
	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatal(err)
	}

	view, err := f.dash.Execute(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Roots) != 1 {
		t.Fatalf("roots = %d, want 1", len(view.Roots))
	}
	if len(view.Roots[0].Children) != len(first.Created) {
		t.Fatalf("children = %d, want %d", len(view.Roots[0].Children), len(first.Created))
	}
	if view.Stats.Total != 1+len(first.Created) {
		t.Fatalf("total = %d, want %d", view.Stats.Total, 1+len(first.Created))
	}
	if len(view.Queue) != 1+len(first.Created) {
		t.Fatalf("queue = %d, want %d (root seed is also unverified)", len(view.Queue), 1+len(first.Created))
	}
}

func (f *flow) observeAll(t *testing.T, key hotsearch.Key) {
	t.Helper()
	if _, err := f.observe.Execute(f.ctx, recordobservation.Command{Term: key.Term.String()}); err != nil {
		t.Fatalf("observe %s: %v", key, err)
	}
}

func (f *flow) promoteIfEligible(t *testing.T, key hotsearch.Key) bool {
	t.Helper()
	f.observeAll(t, key)
	res, err := f.assess.Execute(f.ctx, judgetrend.Command{Term: key.Term.String()})
	if err != nil {
		t.Fatalf("assess %s: %v", key, err)
	}
	if !res.AllowsSeed {
		return false
	}
	if _, err := f.advance.Execute(f.ctx, promote.Command{Term: key.Term.String()}); err != nil {
		t.Fatalf("advance %s: %v", key, err)
	}
	return true
}

func TestJudgeTrendComputesBrandShare(t *testing.T) {
	f := newFlow()
	rootKey := f.seed(t, "ai agent")
	root, err := f.repo.Get(f.ctx, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{"chatgpt", "best ai agent", "ai agent memory"} {
		normalized, err := kernel.NewTerm(term)
		if err != nil {
			t.Fatal(err)
		}
		child, err := hotsearch.NewChild(root, normalized, hotsearch.SourceRising, time.Now(), hotsearch.DefaultExpansionPolicy())
		if err != nil {
			t.Fatal(err)
		}
		if err := f.repo.Save(f.ctx, child); err != nil {
			t.Fatal(err)
		}
	}

	res, err := f.assess.Execute(f.ctx, judgetrend.Command{Term: rootKey.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	if res.ChildCount != 3 {
		t.Fatalf("child count = %d, want 3", res.ChildCount)
	}
	if math.Abs(res.BrandShare-1.0/3.0) > 1e-9 {
		t.Fatalf("brand share = %v, want 1/3", res.BrandShare)
	}
	if res.Crowded {
		t.Fatal("one brand of three is not crowded")
	}
	loaded, err := f.repo.Get(f.ctx, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(loaded.Signals().BrandShare-1.0/3.0) > 1e-9 {
		t.Fatalf("persisted brand share = %v", loaded.Signals().BrandShare)
	}
}

func TestDiscardAndReopen(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")
	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	target := first.Created[0]
	f.observeAll(t, target)
	if _, err := f.assess.Execute(f.ctx, judgetrend.Command{Term: target.Term.String()}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.discard.Execute(f.ctx, discardkeyword.Command{Term: target.Term.String()}); !errors.Is(err, hotsearch.ErrReasonRequired) {
		t.Fatalf("discard without reason err = %v", err)
	}
	discarded, err := f.discard.Execute(f.ctx, discardkeyword.Command{Term: target.Term.String(), Reason: "not relevant"})
	if err != nil {
		t.Fatal(err)
	}
	if discarded.State != hotsearch.StateDiscarded {
		t.Fatalf("state = %s", discarded.State)
	}
	if _, err := f.advance.Execute(f.ctx, promote.Command{Term: target.Term.String()}); err == nil {
		t.Fatal("discarded keyword must not promote")
	}

	reopened, err := f.reopen.Execute(f.ctx, reopenkeyword.Command{Term: target.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	if reopened.State != hotsearch.StateDiscovered {
		t.Fatalf("state = %s", reopened.State)
	}
	loaded, err := f.repo.Get(f.ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verdict() != hotsearch.VerdictUnknown {
		t.Fatalf("verdict = %s, want UNKNOWN after reopen", loaded.Verdict())
	}
	if !loaded.Signals().IsZero() {
		t.Fatalf("signals must reset on reopen: %+v", loaded.Signals())
	}
	if _, err := f.reopen.Execute(f.ctx, reopenkeyword.Command{Term: target.Term.String()}); !errors.Is(err, hotsearch.ErrInvalidTransition) {
		t.Fatalf("reopen twice err = %v", err)
	}
}

func TestDashboardListsSeedCandidates(t *testing.T) {
	f := newFlow()
	root := f.seed(t, "ai agent")
	first, err := f.expand.Execute(f.ctx, expandkeyword.Command{Term: root.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	promoted := 0
	for _, key := range first.Created {
		if f.promoteIfEligible(t, key) {
			promoted++
		}
	}
	view, err := f.dash.Execute(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Candidates) != promoted {
		t.Fatalf("candidates = %d, want %d", len(view.Candidates), promoted)
	}
	if promoted > 0 && view.Stats.SeedCandidates != promoted {
		t.Fatalf("stats candidates = %d, want %d", view.Stats.SeedCandidates, promoted)
	}
}
