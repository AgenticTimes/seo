package hotsearch

import (
	"math"
	"testing"
	"time"
)

func seasonalSeries(n int, base, amplitude, growth float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = base + growth*float64(i) + amplitude*math.Sin(2*math.Pi*float64(i)/52)
	}
	return out
}

func TestDecomposeSeparatesSeasonAndTrend(t *testing.T) {
	seasonal, ok := Decompose(seasonalSeries(260, 50, 25, 0), 52)
	if !ok {
		t.Fatal("expected decomposition")
	}
	if seasonal.Strength < 0.5 {
		t.Fatalf("seasonal strength = %.2f, want high", seasonal.Strength)
	}
	if math.Abs(seasonal.Growth) > 2 {
		t.Fatalf("seasonal growth = %.2f, want near zero", seasonal.Growth)
	}

	trend, ok := Decompose(seasonalSeries(260, 20, 0, 0.2), 52)
	if !ok {
		t.Fatal("expected decomposition")
	}
	if trend.Strength > 0.2 {
		t.Fatalf("trend strength = %.2f, want low", trend.Strength)
	}
	if trend.Growth <= 0 {
		t.Fatalf("trend growth = %.2f, want positive", trend.Growth)
	}

	both, ok := Decompose(seasonalSeries(260, 50, 25, 0.15), 52)
	if !ok {
		t.Fatal("expected decomposition")
	}
	if both.Strength < 0.3 || both.Growth <= 0 {
		t.Fatalf("combined = strength %.2f growth %.2f, want both", both.Strength, both.Growth)
	}

	if _, ok := Decompose(seasonalSeries(60, 50, 25, 0), 52); ok {
		t.Fatal("short series must not decompose")
	}
}

// TestSpikeRatioMedianBalanced pins the median: for an even count the two
// middle samples are averaged, so the base does not lean toward the upper half.
func TestSpikeRatioMedianBalanced(t *testing.T) {
	values := []int{1, 2, 3, 9}
	samples := make([]Index, len(values))
	for i, v := range values {
		idx, err := NewIndex(v)
		if err != nil {
			t.Fatal(err)
		}
		samples[i] = idx
	}
	// median(1,2,3,9) = 2.5, so the ratio is 9/2.5 = 3.6, not 9/3 = 3.
	if got := spikeRatio(samples); math.Abs(got-3.6) > 1e-9 {
		t.Fatalf("spikeRatio = %.4f, want 3.6", got)
	}
	if got := medianEven([]int{1, 2, 3, 4}); got != 2.5 {
		t.Fatalf("medianEven = %.1f, want 2.5", got)
	}
	if got := medianEven([]int{1, 2, 5}); got != 2 {
		t.Fatalf("medianEven odd = %.1f, want 2", got)
	}
}

func TestIntentRules(t *testing.T) {
	cases := map[string]Intent{
		"claude ai login":       IntentNavigational,
		"chatgpt":               IntentNavigational,
		"ai agent price":        IntentTransactional,
		"best coding agent":     IntentCommercial,
		"how to build an agent": IntentInformational,
		"ai agent memory":       IntentUnknown,
		"agent 教程":              IntentInformational,
		"claude ai 官网":          IntentNavigational,
	}
	for term, want := range cases {
		if got := ClassifyIntent(term); got != want {
			t.Fatalf("ClassifyIntent(%q) = %s, want %s", term, got, want)
		}
	}
	if !IsBrandMention("google gemini ai") {
		t.Fatal("brand mention missed")
	}
	if IsBrandMention("ai agent memory") {
		t.Fatal("false brand mention")
	}
	if got := NonLatinRatio("ai モード"); got <= 0 {
		t.Fatalf("non-latin ratio = %v, want > 0", got)
	}
}

func TestSpikeRatio(t *testing.T) {
	samples := []Index{10, 10, 10, 10, 10, 100, 10, 10}
	if got := spikeRatio(samples); got < 9 {
		t.Fatalf("spike ratio = %.1f, want >= 9", got)
	}
	if got := spikeRatio([]Index{10, 10, 10}); got != 1 {
		t.Fatalf("flat spike ratio = %.1f, want 1", got)
	}
}

func TestClassifySeasonalGrowth(t *testing.T) {
	policy := DefaultTrendPolicy()
	obs := map[Window]Observation{
		Window30D: mustObs(t, Window30D, 10, 20),
		Window90D: mustObs(t, Window90D, 10, 20),
		Window12M: mustObs(t, Window12M, 50, 50),
	}
	long := seasonalSeries(260, 50, 25, 0.15)
	obs[Window5Y] = mustObsFloat(t, Window5Y, long)

	sig := ComputeSignals("ai agent", obs, policy)
	if !sig.HasSeasonality(policy) || sig.StructuralGrowth <= 0 {
		t.Fatalf("signals = %+v, want seasonal growth", sig)
	}
	if got := classifyTrend(obs, sig, policy); got != VerdictSeasonalGrowth {
		t.Fatalf("verdict = %s, want SEASONAL_GROWTH", got)
	}
	if !VerdictSeasonalGrowth.AllowsSeed() {
		t.Fatal("seasonal growth must allow the next round")
	}
}

func TestClassifySeasonalWithoutStructure(t *testing.T) {
	policy := DefaultTrendPolicy()
	obs := map[Window]Observation{
		Window30D: mustObs(t, Window30D, 10, 20),
		Window90D: mustObs(t, Window90D, 10, 20),
		Window12M: mustObs(t, Window12M, 50, 50),
		Window5Y:  mustObsFloat(t, Window5Y, seasonalSeries(260, 50, 25, 0)),
	}
	sig := ComputeSignals("ai agent", obs, policy)
	if got := classifyTrend(obs, sig, policy); got != VerdictSeasonal {
		t.Fatalf("verdict = %s, want SEASONAL", got)
	}
	if VerdictSeasonal.AllowsSeed() {
		t.Fatal("seasonal without structure must not advance")
	}
}

func TestClassifyEvent(t *testing.T) {
	policy := DefaultTrendPolicy()
	spike := make([]int, 260)
	for i := range spike {
		spike[i] = 10
	}
	spike[130] = 100
	obs := map[Window]Observation{
		Window30D: mustObs(t, Window30D, 10, 20),
		Window90D: mustObs(t, Window90D, 10, 10),
		Window12M: mustObs(t, Window12M, 10, 10),
		Window5Y:  mustObsInt(t, Window5Y, spike),
	}
	sig := ComputeSignals("ai agent", obs, policy)
	if !sig.IsSpike(policy) {
		t.Fatalf("spike ratio = %.1f, want spike", sig.SpikeRatio)
	}
	if got := classifyTrend(obs, sig, policy); got != VerdictEvent {
		t.Fatalf("verdict = %s, want EVENT", got)
	}
}

func TestClassifyNoisy(t *testing.T) {
	policy := DefaultTrendPolicy()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	short, err := NewObservation(Window30D, start, start.Add(24*time.Hour), []int{10, 20})
	if err != nil {
		t.Fatal(err)
	}
	obs := map[Window]Observation{Window30D: short}
	if got := classify(obs, policy); got != VerdictInsufficient {
		t.Fatalf("verdict = %s, want INSUFFICIENT for 2 points", got)
	}
}

func mustObsInt(t *testing.T, w Window, values []int) Observation {
	t.Helper()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	o, err := NewObservation(w, start, end, values)
	if err != nil {
		t.Fatalf("NewObservation(%s): %v", w, err)
	}
	return o
}

func mustObsFloat(t *testing.T, w Window, values []float64) Observation {
	t.Helper()
	ints := make([]int, len(values))
	for i, v := range values {
		n := int(math.Round(v))
		if n < 0 {
			n = 0
		}
		if n > 100 {
			n = 100
		}
		ints[i] = n
	}
	return mustObsInt(t, w, ints)
}

func TestAnalyzeReasons(t *testing.T) {
	policy := DefaultTrendPolicy()
	cases := []struct {
		name    string
		series  map[Window][]int
		verdict Verdict
		reasons []string
	}{
		{"no 30d", map[Window][]int{Window90D: {10, 20}}, VerdictInsufficient, []string{ReasonNo30D}},
		{"declining", map[Window][]int{Window30D: {20, 10}}, VerdictDeclining, []string{ReasonDeclining30D}},
		{"flat", map[Window][]int{Window30D: {10, 10}}, VerdictStable, []string{ReasonFlat30D}},
		{"missing 90d", map[Window][]int{Window30D: {10, 20}}, VerdictInsufficient, []string{ReasonMissing90D}},
		{"missing 12m", map[Window][]int{Window30D: {10, 20}, Window90D: {10, 20}}, VerdictInsufficient, []string{ReasonMissing12M}},
		{"sustained", map[Window][]int{Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {10, 20}}, VerdictSustained, []string{ReasonSustained}},
		{"emerging", map[Window][]int{Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {10, 10}}, VerdictEmerging, []string{ReasonEmerging}},
		{"short lived", map[Window][]int{Window30D: {10, 20}, Window90D: {10, 10}, Window12M: {10, 10}}, VerdictShortLived, []string{ReasonShortLived}},
		{"stable 90d", map[Window][]int{Window30D: {10, 20}, Window90D: {10, 10}, Window12M: {10, 20}}, VerdictStable, []string{ReasonStable90D}},
		{"long term", map[Window][]int{Window30D: {10, 20}, Window90D: {10, 20}, Window12M: {10, 20}, Window5Y: {10, 20}}, VerdictLongTerm, []string{ReasonLongTerm}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := map[Window]Observation{}
			for w, samples := range tc.series {
				obs[w] = mustObs(t, w, samples...)
			}
			verdict, reasons := analyzeTrend(obs, ComputeSignals("", obs, policy), policy)
			if verdict != tc.verdict {
				t.Fatalf("verdict = %s, want %s", verdict, tc.verdict)
			}
			if len(reasons) != len(tc.reasons) {
				t.Fatalf("reasons = %v, want %v", reasons, tc.reasons)
			}
			for i := range reasons {
				if reasons[i] != tc.reasons[i] {
					t.Fatalf("reasons = %v, want %v", reasons, tc.reasons)
				}
			}
		})
	}
}

func TestAssessTrendStoresReasons(t *testing.T) {
	k := NewSeed(mustKey(t, "agent memory"), time.Now())
	if err := k.RecordObservation(mustObs(t, Window30D, 10, 20)); err != nil {
		t.Fatal(err)
	}
	if err := k.RecordObservation(mustObs(t, Window90D, 10, 20)); err != nil {
		t.Fatal(err)
	}
	if err := k.RecordObservation(mustObs(t, Window12M, 10, 10)); err != nil {
		t.Fatal(err)
	}
	if v := k.AssessTrend(DefaultTrendPolicy()); v != VerdictEmerging {
		t.Fatalf("verdict = %s", v)
	}
	if len(k.Signals().Reasons) == 0 {
		t.Fatal("reasons not stored")
	}
}

func TestSevenDayChange(t *testing.T) {
	obs := map[Window]Observation{Window7D: mustObs(t, Window7D, 10, 30)}
	sig := ComputeSignals("ai agent", obs, DefaultTrendPolicy())
	if sig.SevenDayChange < 1 {
		t.Fatalf("seven day change = %v, want large", sig.SevenDayChange)
	}
}
