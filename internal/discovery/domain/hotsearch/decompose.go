package hotsearch

import "math"

// Decomposition is a classical additive split of one series into a repeating
// seasonal component and a linear trend. It is the pure core of BR-12/BR-13;
// no IO and no policy decide here, only the arithmetic.
//
// This is not full STL. It estimates the seasonal index by phase mean, removes
// it, then fits a least-squares line to the remainder. That is enough to tell a
// recurring annual pattern from structural growth, which is what BR-12 needs.
type Decomposition struct {
	// Seasonal holds one offset per phase (week of year).
	Seasonal []float64
	// Slope is the trend change per sample on the deseasonalized series.
	Slope float64
	// Growth is the total relative trend change across the window.
	Growth float64
	// Strength is the share of total variance explained by the seasonal part.
	Strength float64
}

// MinPointsPerPhase keeps a seasonal average from resting on one sample.
const MinPointsPerPhase = 2

// Decompose splits a series whose values repeat every period samples. It
// reports false when there are fewer than two full periods.
func Decompose(samples []float64, period int) (Decomposition, bool) {
	if period < 2 || len(samples) < period*2 {
		return Decomposition{}, false
	}
	n := len(samples)

	mean := 0.0
	for _, v := range samples {
		mean += v
	}
	mean /= float64(n)

	sums := make([]float64, period)
	counts := make([]int, period)
	for i, v := range samples {
		sums[i%period] += v
		counts[i%period]++
	}
	seasonal := make([]float64, period)
	complete := true
	for p := range sums {
		if counts[p] < MinPointsPerPhase {
			complete = false
			break
		}
		seasonal[p] = sums[p]/float64(counts[p]) - mean
	}
	if !complete {
		return Decomposition{}, false
	}

	totalVar := 0.0
	for _, v := range samples {
		d := v - mean
		totalVar += d * d
	}
	seasonalVar := 0.0
	for i := range samples {
		s := seasonal[i%period]
		seasonalVar += s * s
	}

	slope := linearSlope(samples, seasonal, period)
	strength := 0.0
	if totalVar > 0 {
		strength = seasonalVar / totalVar
		if strength > 1 {
			strength = 1
		}
	}

	growth := 0.0
	if math.Abs(mean) > 0 {
		growth = slope * float64(n-1) / math.Abs(mean)
	}

	out := Decomposition{
		Seasonal: seasonal,
		Slope:    slope,
		Growth:   growth,
		Strength: strength,
	}
	return out, true
}

// linearSlope fits y = a + b*x on the deseasonalized series and returns b.
func linearSlope(samples, seasonal []float64, period int) float64 {
	n := len(samples)
	if n < 2 {
		return 0
	}
	var sx, sy, sxx, sxy float64
	for i, v := range samples {
		x := float64(i)
		y := v - seasonal[i%period]
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
