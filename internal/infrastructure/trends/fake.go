// Package trends contains adapters for the TrendsGateway port.
package trends

import (
	"context"
	"hash/fnv"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
	"seo/internal/shared/kernel"
)

// Fake is a deterministic gateway used to exercise the manual flow before a
// real Google Trends adapter exists. Same term always yields the same data.
type Fake struct {
	now func() time.Time
}

// NewFake builds a fake with the wall clock.
func NewFake() *Fake { return &Fake{now: time.Now} }

// NewFakeWithClock allows tests to pin time.
func NewFakeWithClock(now func() time.Time) *Fake { return &Fake{now: now} }

type fixture struct {
	term     string
	bucket   string
	rise     int
	breakout bool
}

// curated mirrors the example graph in find_trend.md so the UI demos real terms.
var curated = map[string][]fixture{
	"ai agent": {
		{"ai coding agent", "RISING", 180, false},
		{"browser agent", "RISING", 320, false},
		{"computer use", "RISING", 120, false},
		{"personal ai assistant", "RISING", 90, false},
		{"agent memory", "RISING", 400, true},
		{"ai agent framework", "TOP", 0, false},
	},
	"browser agent": {
		{"browser automation", "RISING", 140, false},
		{"web agent", "RISING", 110, false},
		{"ai web research", "RISING", 260, false},
		{"browser use api", "RISING", 150, false},
		{"website interaction automation", "RISING", 80, false},
	},
	"ai coding": {
		{"coding agent", "RISING", 210, false},
		{"ai code review", "RISING", 130, false},
		{"coding agent benchmark", "RISING", 170, false},
		{"autonomous coding", "RISING", 95, false},
		{"claude code", "TOP", 0, false},
	},
	"ai coding agent": {
		{"coding agent benchmark", "RISING", 170, false},
		{"ai code review", "RISING", 130, false},
		{"openhands", "RISING", 240, false},
		{"claude code", "TOP", 0, false},
	},
	"ai workspace": {
		{"ai workspace tool", "RISING", 120, false},
		{"ai productivity", "RISING", 90, false},
		{"ai docs", "RISING", 110, false},
		{"ai knowledge base", "RISING", 75, false},
	},
	"ai automation": {
		{"workflow automation", "RISING", 100, false},
		{"ai workflow", "RISING", 160, false},
		{"automation agent", "RISING", 140, false},
		{"ai ops", "TOP", 0, false},
	},
	"ai personal assistant": {
		{"personal ai assistant", "TOP", 0, false},
		{"ai assistant app", "RISING", 130, false},
		{"ai companion", "RISING", 220, false},
		{"ai scheduler", "RISING", 85, false},
	},
	"agent memory": {
		{"long term agent memory", "RISING", 190, false},
		{"persistent memory", "RISING", 150, false},
		{"llm memory", "RISING", 170, false},
		{"ai memory management", "RISING", 120, false},
	},
}

var fallbackSuffixes = []string{"tool", "api", "automation", "tutorial", "framework", "benchmark"}

var topics = []ports.RelatedTopic{
	{Topic: "Artificial intelligence", RisePct: 90},
	{Topic: "Software development", RisePct: 45},
	{Topic: "Automation", RisePct: 70},
}

// RelatedQueries returns curated children when known, else a deterministic set.
func (f *Fake) RelatedQueries(_ context.Context, req ports.RelatedRequest) (ports.RelatedResult, error) {
	base := req.Term.String()
	fixtures, ok := curated[base]
	if !ok {
		fixtures = make([]fixture, 0, len(fallbackSuffixes))
		for _, suffix := range fallbackSuffixes {
			fixtures = append(fixtures, fixture{base + " " + suffix, "RISING", 50 + int(hash(base+suffix)%180), false})
		}
	}
	out := ports.RelatedResult{Topics: topics}
	for _, item := range fixtures {
		term, err := kernel.NewTerm(item.term)
		if err != nil {
			continue
		}
		out.Queries = append(out.Queries, ports.DiscoveredTerm{
			Term:     term,
			Source:   hotsearch.ParseSource(item.bucket, item.breakout),
			RisePct:  item.rise,
			Breakout: item.breakout,
		})
	}
	return out, nil
}

// InterestOverTime returns a deterministic four-point series shaped by the
// term's profile so every BR-01 verdict appears in the demo graph.
func (f *Fake) InterestOverTime(_ context.Context, req ports.InterestRequest) (ports.InterestSeries, error) {
	end := f.now().UTC()
	start := end.Add(-windowDuration(req.Window))
	profile := hash(req.Term.String()) % 6
	return ports.InterestSeries{
		Window: req.Window,
		Start:  start,
		End:    end,
		Values: shape(profile, req.Window, req.Term.String()),
	}, nil
}

func windowDuration(w hotsearch.Window) time.Duration {
	switch w {
	case hotsearch.Window7D:
		return 7 * 24 * time.Hour
	case hotsearch.Window30D:
		return 30 * 24 * time.Hour
	case hotsearch.Window90D:
		return 90 * 24 * time.Hour
	case hotsearch.Window12M:
		return 365 * 24 * time.Hour
	case hotsearch.Window5Y:
		return 5 * 365 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

func shape(profile uint32, w hotsearch.Window, term string) []int {
	base := 20 + int(hash(term+string(w))%25)
	up := []int{base, base + 3, base + 6, base + 9, base + 12, base + 15, base + 18, base + 22}
	flat := []int{base, base, base, base, base, base, base, base}
	down := []int{base + 22, base + 18, base + 15, base + 12, base + 9, base + 6, base + 3, base}

	switch profile {
	case 0: // emerging: 30d/90d up, flat year
		switch w {
		case hotsearch.Window30D, hotsearch.Window90D, hotsearch.Window7D:
			return up
		default:
			return flat
		}
	case 1: // sustained: 30d/90d/year up
		switch w {
		case hotsearch.Window12M:
			return up
		case hotsearch.Window5Y:
			return flat
		default:
			return up
		}
	case 2: // short lived: only 30d up
		if w == hotsearch.Window30D {
			return up
		}
		return flat
	case 3: // stable
		return flat
	case 4: // declining
		if w == hotsearch.Window30D || w == hotsearch.Window90D {
			return down
		}
		return flat
	default: // long term: five year growth
		return up
	}
}

func hash(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}
