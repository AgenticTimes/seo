package hotsearch

import (
	"errors"
	"fmt"
	"time"
)

// Direction is the shape of one series over time.
type Direction int

// Direction outcomes.
const (
	DirectionUnknown Direction = iota
	DirectionUp
	DirectionDown
	DirectionFlat
)

// String is used by logs and the UI.
func (d Direction) String() string {
	switch d {
	case DirectionUp:
		return "up"
	case DirectionDown:
		return "down"
	case DirectionFlat:
		return "flat"
	default:
		return "unknown"
	}
}

// Observation is one window's series. It is bound to a single Window so that
// cross-window arithmetic is impossible to express (BR-06).
type Observation struct {
	window  Window
	start   time.Time
	end     time.Time
	samples []Index
}

// ErrInvalidObservation rejects malformed series.
var ErrInvalidObservation = errors.New("keyword: invalid observation")

// NewObservation validates and stores one window's series.
func NewObservation(w Window, start, end time.Time, rawSamples []int) (Observation, error) {
	if !w.Valid() {
		return Observation{}, ErrInvalidWindow
	}
	if len(rawSamples) == 0 {
		return Observation{}, fmt.Errorf("%w: no samples", ErrInvalidObservation)
	}
	if end.Before(start) {
		return Observation{}, fmt.Errorf("%w: end before start", ErrInvalidObservation)
	}
	samples := make([]Index, 0, len(rawSamples))
	for _, v := range rawSamples {
		idx, err := NewIndex(v)
		if err != nil {
			return Observation{}, err
		}
		samples = append(samples, idx)
	}
	return Observation{window: w, start: start, end: end, samples: samples}, nil
}

// Window returns the bound window.
func (o Observation) Window() Window { return o.window }

// Start returns the first sample time.
func (o Observation) Start() time.Time { return o.start }

// End returns the last sample time.
func (o Observation) End() time.Time { return o.end }

// Samples returns a defensive copy of the series.
func (o Observation) Samples() []Index {
	out := make([]Index, len(o.samples))
	copy(out, o.samples)
	return out
}

// IsZero reports whether the observation is unset.
func (o Observation) IsZero() bool { return o.window == "" || len(o.samples) == 0 }

// ChangePct is the relative change from the first to the last sample. A zero
// baseline is floored at 1 so a 0→50 series reports a large but finite rise.
func (o Observation) ChangePct() float64 {
	if len(o.samples) < 2 {
		return 0
	}
	first := float64(o.samples[0])
	last := float64(o.samples[len(o.samples)-1])
	base := first
	if base < 1 {
		base = 1
	}
	return (last - first) / base
}

// Direction classifies this series against the policy threshold. It only ever
// looks at its own window's samples.
func (o Observation) Direction(p TrendPolicy) Direction {
	if len(o.samples) < 2 {
		return DirectionUnknown
	}
	change := o.ChangePct()
	switch {
	case change >= p.MinChangePct:
		return DirectionUp
	case change <= -p.MinChangePct:
		return DirectionDown
	default:
		return DirectionFlat
	}
}
