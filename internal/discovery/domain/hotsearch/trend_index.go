package hotsearch

import "errors"

// Index is a Google Trends normalized interest value in [0, 100].
//
// BR-06: an Index is meaningless without its window. Index deliberately has no
// cross-window comparison API; all comparison happens inside Observation, which
// is bound to exactly one window. Never rank keywords by Index across windows
// or against another keyword — only compare the shape of one series.
type Index int

// ErrIndexOutOfRange rejects values outside Google's 0-100 scale.
var ErrIndexOutOfRange = errors.New("keyword: trend index outside 0-100")

// NewIndex validates a normalized index sample.
func NewIndex(v int) (Index, error) {
	if v < 0 || v > 100 {
		return 0, ErrIndexOutOfRange
	}
	return Index(v), nil
}

// Int returns the raw value.
func (i Index) Int() int { return int(i) }
