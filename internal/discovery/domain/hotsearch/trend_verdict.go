package hotsearch

// Verdict is the outcome of BR-01's multi-window cross-check.
type Verdict string

// Verdict values. Only Emerging, Sustained and SeasonalGrowth may become the
// next seed.
const (
	VerdictUnknown        Verdict = "UNKNOWN"
	VerdictInsufficient   Verdict = "INSUFFICIENT"
	VerdictDeclining      Verdict = "DECLINING"
	VerdictStable         Verdict = "STABLE"
	VerdictShortLived     Verdict = "SHORT_LIVED"
	VerdictEmerging       Verdict = "EMERGING"
	VerdictSustained      Verdict = "SUSTAINED"
	VerdictLongTerm       Verdict = "LONG_TERM"
	VerdictSeasonal       Verdict = "SEASONAL"
	VerdictSeasonalGrowth Verdict = "SEASONAL_GROWTH"
	VerdictEvent          Verdict = "EVENT"
)

// AllowsSeed implements BR-10: only demand that is both current and durable may
// drive the next round. Long-term tracks are real but are not new opportunities;
// seasonal growth is allowed because the structural trend is up.
func (v Verdict) AllowsSeed() bool {
	switch v {
	case VerdictEmerging, VerdictSustained, VerdictSeasonalGrowth:
		return true
	default:
		return false
	}
}

// ParseVerdict accepts a stored code.
func ParseVerdict(raw string) (Verdict, error) {
	switch v := Verdict(raw); v {
	case VerdictUnknown, VerdictInsufficient, VerdictDeclining, VerdictStable,
		VerdictShortLived, VerdictEmerging, VerdictSustained, VerdictLongTerm,
		VerdictSeasonal, VerdictSeasonalGrowth, VerdictEvent:
		return v, nil
	default:
		return "", ErrInvalidVerdict
	}
}
