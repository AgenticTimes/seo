package hotsearch

// Source is where a discovery came from in Google Trends.
//
// BR-02: Breakout means "grew more than 5000% relative to the baseline". It is a
// signal of change, not of absolute volume, and must never be used to rank
// keywords by heat.
type Source string

// Discovery sources.
const (
	SourceSeed     Source = "SEED"
	SourceTop      Source = "TOP"
	SourceRising   Source = "RISING"
	SourceBreakout Source = "BREAKOUT"
)

// ParseSource maps a provider bucket to a domain source. A rising entry flagged
// as breakout is stored as Breakout so the flag cannot be lost.
func ParseSource(bucket string, breakout bool) Source {
	if breakout {
		return SourceBreakout
	}
	if bucket == string(SourceTop) {
		return SourceTop
	}
	return SourceRising
}

// Valid reports whether the source is known.
func (s Source) Valid() bool {
	return s == SourceSeed || s == SourceTop || s == SourceRising || s == SourceBreakout
}

// IsBreakout reports the BR-02 flag.
func (s Source) IsBreakout() bool { return s == SourceBreakout }

// IsRising reports whether the source is a growth signal (BR-07).
func (s Source) IsRising() bool { return s == SourceRising || s == SourceBreakout }
