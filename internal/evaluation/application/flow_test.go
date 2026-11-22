package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/application/attacheevidence"
	"seo/internal/evaluation/application/createopportunity"
	"seo/internal/evaluation/application/decideopportunity"
	evalquery "seo/internal/evaluation/application/query"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/infrastructure/memory"
	"seo/internal/shared/kernel"
)

type harness struct {
	ctx      context.Context
	keywords *memory.KeywordRepository
	opps     *memory.OpportunityRepository
	create   *createopportunity.Handler
	attach   *attacheevidence.Handler
	decide   *decideopportunity.Handler
	lookup   *evalquery.Lookup
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	keywords := memory.NewKeywordRepository()
	opps := memory.NewOpportunityRepository()
	return &harness{
		ctx:      context.Background(),
		keywords: keywords,
		opps:     opps,
		create:   createopportunity.New(keywords, opps, nil),
		attach:   attacheevidence.New(opps, nil),
		decide:   decideopportunity.New(keywords, opps, nil),
		lookup:   evalquery.NewLookup(opps),
	}
}

func (h *harness) seedValidated(t *testing.T, term string, verdict hotsearch.Verdict) hotsearch.Key {
	t.Helper()
	up := []int{10, 14, 18, 22, 26, 30, 34, 40}
	flat := []int{10, 10, 10, 10, 10, 10, 10, 10}
	return h.seedSeries(t, term, [][]int{up, up, flat}, verdict)
}

func (h *harness) seedSeries(t *testing.T, term string, series [][]int, verdict hotsearch.Verdict) hotsearch.Key {
	t.Helper()
	key := testKey(t, term)
	k := hotsearch.NewSeed(key, time.Now())
	start := time.Unix(0, 0)
	for i, w := range []hotsearch.Window{hotsearch.Window30D, hotsearch.Window90D, hotsearch.Window12M} {
		obs, err := hotsearch.NewObservation(w, start, start.Add(time.Hour), series[i])
		if err != nil {
			t.Fatal(err)
		}
		if err := k.RecordObservation(obs); err != nil {
			t.Fatal(err)
		}
	}
	if got := k.AssessTrend(hotsearch.DefaultTrendPolicy()); got != verdict {
		t.Fatalf("verdict = %s, want %s", got, verdict)
	}
	if err := h.keywords.Save(h.ctx, k); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestFlowApproveWithEvidence(t *testing.T) {
	h := newHarness(t)
	key := h.seedValidated(t, "agent memory", hotsearch.VerdictEmerging)

	created, err := h.create.Execute(h.ctx, createopportunity.Command{Term: key.Term.String()})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || created.State != opportunity.StateCandidate {
		t.Fatalf("create = %+v", created)
	}
	again, err := h.create.Execute(h.ctx, createopportunity.Command{Term: key.Term.String()})
	if err != nil || again.Created {
		t.Fatalf("second create = %+v, %v", again, err)
	}

	// BR-20: approval without evidence must fail.
	if _, err := h.decide.Execute(h.ctx, decideopportunity.Command{Term: key.Term.String(), Approve: true}); !errors.Is(err, opportunity.ErrGateFailed) {
		t.Fatalf("approve without evidence err = %v", err)
	}

	attached, err := h.attach.Execute(h.ctx, attacheevidence.Command{
		Term: key.Term.String(), Source: "REDDIT", URL: "https://reddit.com/r/x", Summary: "users ask for memory",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attached.State != opportunity.StateEvidence || attached.Count != 1 {
		t.Fatalf("attach = %+v", attached)
	}

	decided, err := h.decide.Execute(h.ctx, decideopportunity.Command{Term: key.Term.String(), Approve: true, Note: "strong signal"})
	if err != nil {
		t.Fatal(err)
	}
	if decided.State != opportunity.StateApproved {
		t.Fatalf("state = %s", decided.State)
	}

	o, err := h.lookup.Execute(h.ctx, key)
	if err != nil || o == nil {
		t.Fatalf("lookup = %v, %v", o, err)
	}
	if o.State() != opportunity.StateApproved || len(o.Evidence()) != 1 {
		t.Fatalf("persisted = %+v", o.Snapshot())
	}
}

func TestFlowIneligibleKeywords(t *testing.T) {
	h := newHarness(t)
	navigational := h.seedValidated(t, "claude login", hotsearch.VerdictEmerging)
	if _, err := h.create.Execute(h.ctx, createopportunity.Command{Term: navigational.Term.String()}); !errors.Is(err, opportunity.ErrNotEligible) {
		t.Fatalf("navigational err = %v", err)
	}

	down := []int{40, 34, 30, 26, 22, 18, 14, 10}
	declining := h.seedSeries(t, "ai wrapper", [][]int{down, down, down}, hotsearch.VerdictDeclining)
	if _, err := h.create.Execute(h.ctx, createopportunity.Command{Term: declining.Term.String()}); !errors.Is(err, opportunity.ErrNotEligible) {
		t.Fatalf("declining err = %v", err)
	}
}

func TestFlowReject(t *testing.T) {
	h := newHarness(t)
	key := h.seedValidated(t, "ai scheduler", hotsearch.VerdictEmerging)
	if _, err := h.create.Execute(h.ctx, createopportunity.Command{Term: key.Term.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.decide.Execute(h.ctx, decideopportunity.Command{Term: key.Term.String()}); !errors.Is(err, opportunity.ErrNoteRequired) {
		t.Fatalf("reject without note err = %v", err)
	}
	rejected, err := h.decide.Execute(h.ctx, decideopportunity.Command{Term: key.Term.String(), Note: "too many competitors"})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.State != opportunity.StateRejected {
		t.Fatalf("state = %s", rejected.State)
	}
}

func testKey(t *testing.T, term string) hotsearch.Key {
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
