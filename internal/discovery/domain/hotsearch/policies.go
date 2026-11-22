package hotsearch

// ExpansionPolicy bounds how deep the keyword tree may grow.
type ExpansionPolicy struct {
	MaxDepth int
}

// DefaultExpansionPolicy implements BR-09: expand two rounds, then stop.
func DefaultExpansionPolicy() ExpansionPolicy {
	return ExpansionPolicy{MaxDepth: 2}
}

// AllowsDepth reports whether a node at depth may exist or be expanded.
func (p ExpansionPolicy) AllowsDepth(depth int) bool {
	return p.MaxDepth >= 0 && depth >= 0 && depth <= p.MaxDepth
}

// TrendPolicy holds the thresholds for BR-01 and the signal rules. Keeping them
// here means the verdict logic stays testable and the numbers stay tunable.
type TrendPolicy struct {
	// MinChangePct is the smallest relative move that counts as up or down.
	MinChangePct float64
	// MinSeasonality is the seasonal variance share that triggers BR-12.
	MinSeasonality float64
	// SpikeRatio is the max/median ratio that marks a one-off peak (BR-15).
	SpikeRatio float64
	// MinPoints30 is the smallest 30D sample count that is trustworthy (BR-16).
	MinPoints30 int
	// BrandShareThreshold marks a crowded entry point (BR-18).
	BrandShareThreshold float64
}

// DefaultTrendPolicy is a deliberately conservative starting point.
func DefaultTrendPolicy() TrendPolicy {
	return TrendPolicy{
		MinChangePct:        0.10,
		MinSeasonality:      0.30,
		SpikeRatio:          3.0,
		MinPoints30:         7,
		BrandShareThreshold: 0.40,
	}
}
