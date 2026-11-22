package hotsearch

import (
	"math"
	"sort"
)

// Signals are the computed properties of one keyword. They are derived from
// observations and the term string; the aggregate stores the latest snapshot so
// the UI and an operator can audit why a verdict was produced.
type Signals struct {
	Points30         int     `json:"points30"`
	Points90         int     `json:"points90"`
	Momentum30       float64 `json:"momentum30"`
	Momentum90       float64 `json:"momentum90"`
	Acceleration     float64 `json:"acceleration"`
	SpikeRatio       float64 `json:"spike_ratio"`
	Seasonality      float64 `json:"seasonality"`
	StructuralGrowth float64 `json:"structural_growth"`
	Intent           Intent  `json:"intent"`
	BrandMention     bool    `json:"brand_mention"`
	NonLatinRatio    float64 `json:"non_latin_ratio"`
	// SevenDayChange is the 7D relative change. It catches news-driven spikes
	// that the longer windows smooth out.
	SevenDayChange float64 `json:"seven_day_change"`
	// BrandShare is the share of brand-named children. It is computed by the
	// application layer, which can load the neighborhood.
	BrandShare float64 `json:"brand_share"`
	// ChildCount is how many children the share was computed over.
	ChildCount int `json:"child_count"`
	// Reasons explains the verdict. Codes stay language-neutral.
	Reasons []string `json:"reasons"`
}

// IsZero reports whether no signals were computed yet.
func (s Signals) IsZero() bool {
	return s.Points30 == 0 && s.Points90 == 0 && s.Intent == "" &&
		s.SevenDayChange == 0 && len(s.Reasons) == 0 &&
		s.BrandShare == 0 && s.ChildCount == 0
}

// Noisy applies BR-16: too few points make a shape unreliable.
func (s Signals) Noisy(p TrendPolicy) bool { return s.Points30 < p.MinPoints30 }

// HasSeasonality applies BR-12: a strong annual pattern changes the comparison.
func (s Signals) HasSeasonality(p TrendPolicy) bool { return s.Seasonality >= p.MinSeasonality }

// IsSpike applies BR-15: a one-off peak is not growth.
func (s Signals) IsSpike(p TrendPolicy) bool { return s.SpikeRatio >= p.SpikeRatio }

// IsCrowded applies BR-18: most children are brands, so this is a competitive
// entry point, not an unmet need.
func (s Signals) IsCrowded(p TrendPolicy) bool { return s.BrandShare >= p.BrandShareThreshold }

// ComputeSignals derives all signals. It never decides a verdict.
func ComputeSignals(term string, obs map[Window]Observation, p TrendPolicy) Signals {
	s := Signals{
		Intent:        ClassifyIntent(term),
		BrandMention:  IsBrandMention(term),
		NonLatinRatio: NonLatinRatio(term),
	}

	if o, ok := obs[Window30D]; ok {
		s.Points30 = len(o.Samples())
		s.Momentum30 = logSlope(o.Samples())
	}
	if o, ok := obs[Window90D]; ok {
		s.Points90 = len(o.Samples())
		s.Momentum90 = logSlope(o.Samples())
	}
	if o, ok := obs[Window7D]; ok {
		s.SevenDayChange = o.ChangePct()
	}
	s.Acceleration = s.Momentum30 - s.Momentum90

	if longest, ok := longestObservation(obs); ok && len(longest.Samples()) > 0 {
		s.SpikeRatio = spikeRatio(longest.Samples())
	}

	if o, ok := obs[Window5Y]; ok {
		values := floatSamples(o.Samples())
		if d, ok := Decompose(values, 52); ok {
			s.Seasonality = d.Strength
			s.StructuralGrowth = d.Growth
		}
	}
	return s
}

// longestObservation picks the series with the most points for spike detection.
func longestObservation(obs map[Window]Observation) (Observation, bool) {
	var best Observation
	found := false
	for _, w := range []Window{Window5Y, Window12M, Window90D, Window30D, Window7D} {
		o, ok := obs[w]
		if !ok {
			continue
		}
		if !found || len(o.Samples()) > len(best.Samples()) {
			best = o
			found = true
		}
	}
	return best, found
}

// logSlope is the least-squares slope of log(1+x) against the sample index.
// Log space keeps a 10→100 rise comparable to a 10→20 rise.
func logSlope(samples []Index) float64 {
	n := len(samples)
	if n < 2 {
		return 0
	}
	var sx, sy, sxx, sxy float64
	for i, v := range samples {
		x := float64(i)
		y := math.Log1p(float64(v.Int()))
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	denom := float64(n)*sxx - sx*sx
	if denom == 0 {
		return 0
	}
	return (float64(n)*sxy - sx*sy) / denom
}

func spikeRatio(samples []Index) float64 {
	if len(samples) == 0 {
		return 0
	}
	values := make([]int, len(samples))
	for i, v := range samples {
		values[i] = v.Int()
	}
	sort.Ints(values)
	max := values[len(values)-1]
	median := values[len(values)/2]
	base := float64(median)
	if base < 1 {
		base = 1
	}
	return float64(max) / base
}

func floatSamples(samples []Index) []float64 {
	out := make([]float64, len(samples))
	for i, v := range samples {
		out[i] = float64(v.Int())
	}
	return out
}
