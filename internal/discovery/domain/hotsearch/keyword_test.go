package hotsearch

import (
	"errors"
	"testing"
	"time"

	"seo/internal/shared/kernel"
)

func mustTerm(t *testing.T, s string) kernel.Term {
	t.Helper()
	term, err := kernel.NewTerm(s)
	if err != nil {
		t.Fatalf("NewTerm(%q): %v", s, err)
	}
	return term
}

func mustKey(t *testing.T, term string) Key {
	t.Helper()
	key, err := NewKey(mustTerm(t, term), kernel.CountryWorldwide, kernel.EntityTopic)
	if err != nil {
		t.Fatalf("NewKey(%q): %v", term, err)
	}
	return key
}

func mustObs(t *testing.T, w Window, samples ...int) Observation {
	t.Helper()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	// The 30D noise gate (BR-16) needs at least MinPoints30 samples. Expand
	// short test anchors linearly; the first and last values stay the same.
	o, err := NewObservation(w, start, end, expandSamples(samples, 8))
	if err != nil {
		t.Fatalf("NewObservation(%s): %v", w, err)
	}
	return o
}

func TestClassifyBR01(t *testing.T) {
	policy := DefaultTrendPolicy()

	tests := []struct {
		name    string
		series  map[Window][]int
		verdict Verdict
	}{
		{
			name:    "no 30d data is insufficient",
			series:  map[Window][]int{Window90D: {10, 20}},
			verdict: VerdictInsufficient,
		},
		{
			name:    "30d down is declining",
			series:  map[Window][]int{Window30D: {20, 10}},
			verdict: VerdictDeclining,
		},
		{
			name:    "30d flat is stable",
			series:  map[Window][]int{Window30D: {10, 10}},
			verdict: VerdictStable,
		},
		{
			name: "30d and 90d and 12m up is sustained",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {10, 20},
			},
			verdict: VerdictSustained,
		},
		{
			name: "30d and 90d up with flat year is emerging",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {10, 10},
			},
			verdict: VerdictEmerging,
		},
		{
			name: "30d and 90d up after a down year is emerging",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {20, 10},
			},
			verdict: VerdictEmerging,
		},
		{
			name: "30d up but 90d flat is short lived",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 10}, Window12M: {10, 10},
			},
			verdict: VerdictShortLived,
		},
		{
			name: "30d up but 90d flat after a growing year is stable",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 10}, Window12M: {10, 20},
			},
			verdict: VerdictStable,
		},
		{
			name: "five year growth takes precedence over sustained",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {10, 20}, Window5Y: {10, 20},
			},
			verdict: VerdictLongTerm,
		},
		{
			name:    "30d up without 90d is insufficient",
			series:  map[Window][]int{Window30D: {10, 20}},
			verdict: VerdictInsufficient,
		},
		{
			name: "30d and 90d up without 12m is insufficient",
			series: map[Window][]int{
				Window30D: {10, 20}, Window90D: {10, 20},
			},
			verdict: VerdictInsufficient,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			obs := make(map[Window]Observation, len(tc.series))
			for w, samples := range tc.series {
				obs[w] = mustObs(t, w, samples...)
			}
			if got := classify(obs, policy); got != tc.verdict {
				t.Fatalf("classify = %s, want %s", got, tc.verdict)
			}
		})
	}
}

// expandSamples linearly interpolates a short series up to min points.
func expandSamples(in []int, min int) []int {
	if len(in) >= min || len(in) < 2 {
		return in
	}
	out := make([]int, min)
	span := len(in) - 1
	for i := 0; i < min; i++ {
		pos := float64(i) * float64(span) / float64(min-1)
		lo := int(pos)
		hi := lo + 1
		if hi > span {
			hi = span
		}
		frac := pos - float64(lo)
		out[i] = int(float64(in[lo]) + (float64(in[hi])-float64(in[lo]))*frac)
	}
	return out
}

func TestIndexBounds(t *testing.T) {
	if _, err := NewIndex(-1); !errors.Is(err, ErrIndexOutOfRange) {
		t.Fatalf("NewIndex(-1) err = %v", err)
	}
	if _, err := NewIndex(101); !errors.Is(err, ErrIndexOutOfRange) {
		t.Fatalf("NewIndex(101) err = %v", err)
	}
	if idx, err := NewIndex(100); err != nil || idx.Int() != 100 {
		t.Fatalf("NewIndex(100) = %v, %v", idx, err)
	}
}

func TestObservationDirectionUsesOwnSamples(t *testing.T) {
	policy := DefaultTrendPolicy()
	up := mustObs(t, Window30D, 10, 20)
	flat := mustObs(t, Window12M, 50, 50)

	if got := up.Direction(policy); got != DirectionUp {
		t.Fatalf("30d direction = %s, want up", got)
	}
	if got := flat.Direction(policy); got != DirectionFlat {
		t.Fatalf("12m direction = %s, want flat", got)
	}
	if up.Window() != Window30D || flat.Window() != Window12M {
		t.Fatal("observation must stay bound to its window")
	}
}

func TestRecordObservationReplacesSameWindow(t *testing.T) {
	k := NewSeed(mustKey(t, "ai agent"), time.Now())
	if err := k.RecordObservation(mustObs(t, Window30D, 10, 20)); err != nil {
		t.Fatal(err)
	}
	if err := k.RecordObservation(mustObs(t, Window30D, 10, 30)); err != nil {
		t.Fatal(err)
	}
	if got := len(k.Observations()); got != 1 {
		t.Fatalf("observations = %d, want 1 (replace per window)", got)
	}
	if _, ok := k.Observation(Window30D); !ok {
		t.Fatal("30d observation missing")
	}
}

func TestConfirmAsSeedCandidateGates(t *testing.T) {
	policy := DefaultExpansionPolicy()

	t.Run("unvalidated is rejected", func(t *testing.T) {
		parent := NewSeed(mustKey(t, "ai agent"), time.Now())
		child, err := NewChild(parent, mustTerm(t, "browser agent"), SourceRising, time.Now(), policy)
		if err != nil {
			t.Fatal(err)
		}
		if err := child.ConfirmAsSeedCandidate(policy); !errors.Is(err, ErrNotValidated) {
			t.Fatalf("err = %v, want ErrNotValidated", err)
		}
	})

	t.Run("emerging passes", func(t *testing.T) {
		parent := NewSeed(mustKey(t, "ai agent"), time.Now())
		child, err := NewChild(parent, mustTerm(t, "browser agent"), SourceRising, time.Now(), policy)
		if err != nil {
			t.Fatal(err)
		}
		child.recordDefaultsForTest(t)
		if v := child.AssessTrend(DefaultTrendPolicy()); v != VerdictEmerging {
			t.Fatalf("verdict = %s, want emerging", v)
		}
		if err := child.ConfirmAsSeedCandidate(policy); err != nil {
			t.Fatalf("confirm: %v", err)
		}
		if child.State() != StateSeedCandidate {
			t.Fatalf("state = %s, want seed candidate", child.State())
		}
	})

	t.Run("too broad is rejected", func(t *testing.T) {
		parent := NewSeed(mustKey(t, "ai agent"), time.Now())
		child, err := NewChild(parent, mustTerm(t, "ai"), SourceRising, time.Now(), policy)
		if err != nil {
			t.Fatal(err)
		}
		child.recordDefaultsForTest(t)
		child.AssessTrend(DefaultTrendPolicy())
		if err := child.MarkTooBroad("one word, no intent"); err != nil {
			t.Fatal(err)
		}
		if err := child.ConfirmAsSeedCandidate(policy); !errors.Is(err, ErrTooBroad) {
			t.Fatalf("err = %v, want ErrTooBroad", err)
		}
	})

	t.Run("root seed cannot advance", func(t *testing.T) {
		root := NewSeed(mustKey(t, "ai agent"), time.Now())
		root.recordDefaultsForTest(t)
		root.AssessTrend(DefaultTrendPolicy())
		if err := root.ConfirmAsSeedCandidate(policy); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("err = %v, want ErrInvalidTransition", err)
		}
	})
}

func TestExpansionDepthPolicyBR09(t *testing.T) {
	policy := DefaultExpansionPolicy()
	root := NewSeed(mustKey(t, "ai agent"), time.Now())

	d1, err := NewChild(root, mustTerm(t, "browser agent"), SourceRising, time.Now(), policy)
	if err != nil {
		t.Fatalf("depth 1: %v", err)
	}
	d1.recordDefaultsForTest(t)
	d1.AssessTrend(DefaultTrendPolicy())
	if err := d1.ConfirmAsSeedCandidate(policy); err != nil {
		t.Fatalf("confirm depth 1: %v", err)
	}

	d2, err := NewChild(d1, mustTerm(t, "browser automation"), SourceRising, time.Now(), policy)
	if err != nil {
		t.Fatalf("depth 2: %v", err)
	}
	d2.recordDefaultsForTest(t)
	d2.AssessTrend(DefaultTrendPolicy())
	if err := d2.ConfirmAsSeedCandidate(policy); err != nil {
		t.Fatalf("confirm depth 2: %v", err)
	}

	if _, err := NewChild(d2, mustTerm(t, "web agent"), SourceRising, time.Now(), policy); !errors.Is(err, ErrDepthExceeded) {
		t.Fatalf("depth 3 err = %v, want ErrDepthExceeded", err)
	}
}

func TestMarkTooBroadRequiresReason(t *testing.T) {
	k := NewSeed(mustKey(t, "ai"), time.Now())
	if err := k.MarkTooBroad(""); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("err = %v, want ErrReasonRequired", err)
	}
}

func TestRehydrateRejectsDuplicateWindow(t *testing.T) {
	s := NewSeed(mustKey(t, "ai agent"), time.Now()).Snapshot()
	s.Observations = []Observation{
		mustObs(t, Window30D, 10, 20),
		mustObs(t, Window30D, 20, 30),
	}
	if _, err := Rehydrate(s); err == nil {
		t.Fatal("expected duplicate window error")
	}
}

func TestBreakoutIsNotRankedAsHeat(t *testing.T) {
	if !SourceBreakout.IsBreakout() {
		t.Fatal("breakout flag lost")
	}
	if SourceBreakout.Valid() == false || SourceTop.IsRising() {
		t.Fatal("source classification broken")
	}
	if got := ParseSource("TOP", true); got != SourceBreakout {
		t.Fatalf("ParseSource(TOP, true) = %s, want BREAKOUT", got)
	}
}

// recordDefaultsForTest records an emerging-shaped series (30d up, 90d up,
// flat year). Tests that need another verdict record their own series.
func (k *Keyword) recordDefaultsForTest(t *testing.T) {
	t.Helper()
	if err := k.RecordObservation(mustObs(t, Window30D, 10, 20)); err != nil {
		t.Fatal(err)
	}
	if err := k.RecordObservation(mustObs(t, Window90D, 10, 20)); err != nil {
		t.Fatal(err)
	}
	if err := k.RecordObservation(mustObs(t, Window12M, 10, 10)); err != nil {
		t.Fatal(err)
	}
}
