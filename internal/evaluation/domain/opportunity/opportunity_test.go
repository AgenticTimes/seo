package opportunity_test

import (
	"errors"
	"testing"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
	"seo/internal/shared/kernel"
)

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

func newOpportunity(t *testing.T) *opportunity.Opportunity {
	t.Helper()
	o, err := opportunity.New(key(t, "agent memory"), hotsearch.IntentInformational, hotsearch.VerdictEmerging, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestNewRejectsIneligible(t *testing.T) {
	if _, err := opportunity.New(key(t, "claude login"), hotsearch.IntentNavigational, hotsearch.VerdictEmerging, time.Now()); !errors.Is(err, opportunity.ErrNotEligible) {
		t.Fatalf("navigational err = %v", err)
	}
	if _, err := opportunity.New(key(t, "agent memory"), hotsearch.IntentInformational, hotsearch.VerdictDeclining, time.Now()); !errors.Is(err, opportunity.ErrNotEligible) {
		t.Fatalf("declining err = %v", err)
	}
	if _, err := opportunity.New(key(t, "agent memory"), hotsearch.IntentInformational, hotsearch.VerdictSeasonalGrowth, time.Now()); err != nil {
		t.Fatalf("seasonal growth should be eligible: %v", err)
	}
}

func TestAttachEvidenceBR11(t *testing.T) {
	o := newOpportunity(t)
	now := time.Now()
	e, err := opportunity.NewEvidence(opportunity.SourceReddit, "https://reddit.com/r/x", "users ask for memory", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := o.AttachEvidence(e); err != nil {
		t.Fatal(err)
	}
	if o.State() != opportunity.StateEvidence || len(o.Evidence()) != 1 {
		t.Fatalf("state = %s evidence = %d", o.State(), len(o.Evidence()))
	}
	if err := o.AttachEvidence(e); !errors.Is(err, opportunity.ErrDuplicateEvidence) {
		t.Fatalf("duplicate err = %v", err)
	}
	if _, err := opportunity.NewEvidence(opportunity.SourceOther, "", "", now); !errors.Is(err, opportunity.ErrEvidenceEmpty) {
		t.Fatalf("empty evidence err = %v", err)
	}
}

func TestApproveGateBR20(t *testing.T) {
	o := newOpportunity(t)
	now := time.Now()
	gate := opportunity.Gate{TrendValid: true, TooBroad: false, MinEvidence: 1}

	if err := o.Approve(gate, "", now); !errors.Is(err, opportunity.ErrGateFailed) {
		t.Fatalf("no evidence err = %v", err)
	}
	e, _ := opportunity.NewEvidence(opportunity.SourceReddit, "https://reddit.com/r/x", "asks", now)
	if err := o.AttachEvidence(e); err != nil {
		t.Fatal(err)
	}
	if err := o.Approve(opportunity.Gate{TrendValid: false, MinEvidence: 1}, "", now); !errors.Is(err, opportunity.ErrGateFailed) {
		t.Fatalf("trend gate err = %v", err)
	}
	if err := o.Approve(opportunity.Gate{TrendValid: true, TooBroad: true, MinEvidence: 1}, "", now); !errors.Is(err, opportunity.ErrGateFailed) {
		t.Fatalf("broad gate err = %v", err)
	}
	if err := o.Approve(gate, "evidence is strong", now); err != nil {
		t.Fatal(err)
	}
	if o.State() != opportunity.StateApproved {
		t.Fatalf("state = %s", o.State())
	}
	if err := o.Approve(gate, "", now); !errors.Is(err, opportunity.ErrClosed) {
		t.Fatalf("second approve err = %v", err)
	}
}

func TestRejectNeedsNote(t *testing.T) {
	o := newOpportunity(t)
	if err := o.Reject("", time.Now()); !errors.Is(err, opportunity.ErrNoteRequired) {
		t.Fatalf("empty note err = %v", err)
	}
	if err := o.Reject("competitors already cover it", time.Now()); err != nil {
		t.Fatal(err)
	}
	if o.State() != opportunity.StateRejected {
		t.Fatalf("state = %s", o.State())
	}
	e, _ := opportunity.NewEvidence(opportunity.SourceOther, "https://example.com", "x", time.Now())
	if err := o.AttachEvidence(e); !errors.Is(err, opportunity.ErrClosed) {
		t.Fatalf("attach after reject err = %v", err)
	}
}

func TestRehydrate(t *testing.T) {
	o := newOpportunity(t)
	e, _ := opportunity.NewEvidence(opportunity.SourceReview, "https://example.com/review", "good", time.Now())
	if err := o.AttachEvidence(e); err != nil {
		t.Fatal(err)
	}
	restored, err := opportunity.Rehydrate(o.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if restored.Key() != o.Key() || restored.State() != o.State() || len(restored.Evidence()) != 1 {
		t.Fatalf("rehydrate lost data: %+v", restored)
	}
	snapshot := o.Snapshot()
	snapshot.State = "BROKEN"
	if _, err := opportunity.Rehydrate(snapshot); !errors.Is(err, opportunity.ErrInvalidState) {
		t.Fatalf("invalid state err = %v", err)
	}
}
