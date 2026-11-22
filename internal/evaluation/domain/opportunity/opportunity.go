// Package opportunity is the evaluation context's aggregate: one business
// decision per keyword. It reads the discovery context's published language but
// never writes to it.
package opportunity

import (
	"errors"
	"fmt"
	"time"

	"seo/internal/discovery/domain/hotsearch"
)

// State is the decision lifecycle. It is not the discovery pipeline state.
type State string

// States.
const (
	StateCandidate State = "CANDIDATE"
	StateEvidence  State = "EVIDENCE_GATHERED"
	StateApproved  State = "APPROVED"
	StateRejected  State = "REJECTED"
)

// Valid reports whether the state is known.
func (s State) Valid() bool {
	switch s {
	case StateCandidate, StateEvidence, StateApproved, StateRejected:
		return true
	default:
		return false
	}
}

// IsClosed reports terminal decisions.
func (s State) IsClosed() bool { return s == StateApproved || s == StateRejected }

// SourceType names where evidence came from.
type SourceType string

// Evidence sources.
const (
	SourceReddit     SourceType = "REDDIT"
	SourceCompetitor SourceType = "COMPETITOR"
	SourceReview     SourceType = "REVIEW"
	SourceSearch     SourceType = "SEARCH_VOLUME"
	SourceOther      SourceType = "OTHER"
)

// ParseSourceType accepts a stored or form source name.
func ParseSourceType(raw string) (SourceType, error) {
	s := SourceType(raw)
	if !s.Valid() {
		return "", fmt.Errorf("opportunity: invalid source %q", raw)
	}
	return s, nil
}

// Valid reports whether the source is supported.
func (s SourceType) Valid() bool {
	switch s {
	case SourceReddit, SourceCompetitor, SourceReview, SourceSearch, SourceOther:
		return true
	default:
		return false
	}
}

// Evidence is one external observation. It is what BR-11 requires before a
// trend can become an approved opportunity.
type Evidence struct {
	Source     SourceType `json:"source"`
	URL        string     `json:"url"`
	Summary    string     `json:"summary"`
	CapturedAt time.Time  `json:"captured_at"`
}

// NewEvidence validates one observation.
func NewEvidence(source SourceType, url, summary string, now time.Time) (Evidence, error) {
	if !source.Valid() {
		return Evidence{}, fmt.Errorf("opportunity: invalid source %q", source)
	}
	if url == "" && summary == "" {
		return Evidence{}, ErrEvidenceEmpty
	}
	return Evidence{Source: source, URL: url, Summary: summary, CapturedAt: now.UTC()}, nil
}

// Gate is the evidence BR-20 needs before an approval. The application layer
// computes it from the discovery aggregate, so this context stays downstream.
type Gate struct {
	TrendValid  bool
	TooBroad    bool
	MinEvidence int
}

// Opportunity is keyed by the keyword it evaluates.
type Opportunity struct {
	key       hotsearch.Key
	intent    hotsearch.Intent
	trend     hotsearch.Verdict
	evidence  []Evidence
	state     State
	note      string
	createdAt time.Time
	updatedAt time.Time
}

// ErrNotEligible rejects keywords that cannot become opportunities.
var ErrNotEligible = errors.New("opportunity: keyword is not eligible")

// New creates a candidate from a validated keyword. Navigational intent is
// rejected by BR-17; a trend that cannot advance is rejected by BR-10/20.
func New(key hotsearch.Key, intent hotsearch.Intent, trend hotsearch.Verdict, now time.Time) (*Opportunity, error) {
	if key.IsZero() {
		return nil, errors.New("opportunity: empty key")
	}
	if intent == hotsearch.IntentNavigational {
		return nil, fmt.Errorf("%w: navigational intent", ErrNotEligible)
	}
	if !trend.AllowsSeed() {
		return nil, fmt.Errorf("%w: verdict %s", ErrNotEligible, trend)
	}
	return &Opportunity{
		key:       key,
		intent:    intent,
		trend:     trend,
		state:     StateCandidate,
		createdAt: now.UTC(),
		updatedAt: now.UTC(),
	}, nil
}

// Key returns the evaluated keyword.
func (o *Opportunity) Key() hotsearch.Key { return o.key }

// Intent returns the intent captured at creation.
func (o *Opportunity) Intent() hotsearch.Intent { return o.intent }

// Trend returns the verdict captured at creation.
func (o *Opportunity) Trend() hotsearch.Verdict { return o.trend }

// Evidence returns a defensive copy.
func (o *Opportunity) Evidence() []Evidence {
	out := make([]Evidence, len(o.evidence))
	copy(out, o.evidence)
	return out
}

// State returns the decision state.
func (o *Opportunity) State() State { return o.state }

// Note returns the decision note.
func (o *Opportunity) Note() string { return o.note }

// CreatedAt returns creation time.
func (o *Opportunity) CreatedAt() time.Time { return o.createdAt }

// AttachEvidence applies BR-11. A duplicate URL is rejected.
func (o *Opportunity) AttachEvidence(e Evidence) error {
	if o.state.IsClosed() {
		return ErrClosed
	}
	if e.URL != "" {
		for _, existing := range o.evidence {
			if existing.URL == e.URL {
				return ErrDuplicateEvidence
			}
		}
	}
	o.evidence = append(o.evidence, e)
	o.state = StateEvidence
	o.updatedAt = e.CapturedAt
	return nil
}

// Approve applies the BR-20 gate. It needs trend validity, a non-broad word, and
// at least MinEvidence observations.
func (o *Opportunity) Approve(gate Gate, note string, now time.Time) error {
	if o.state.IsClosed() {
		return ErrClosed
	}
	if !gate.TrendValid {
		return fmt.Errorf("%w: trend is not validated", ErrGateFailed)
	}
	if gate.TooBroad {
		return fmt.Errorf("%w: keyword is too broad", ErrGateFailed)
	}
	minimum := gate.MinEvidence
	if minimum <= 0 {
		minimum = 1
	}
	if len(o.evidence) < minimum {
		return fmt.Errorf("%w: need at least %d evidence items, have %d", ErrGateFailed, minimum, len(o.evidence))
	}
	o.state = StateApproved
	o.note = note
	o.updatedAt = now.UTC()
	return nil
}

// Reject closes the opportunity with a required reason.
func (o *Opportunity) Reject(note string, now time.Time) error {
	if o.state.IsClosed() {
		return ErrClosed
	}
	if note == "" {
		return ErrNoteRequired
	}
	o.state = StateRejected
	o.note = note
	o.updatedAt = now.UTC()
	return nil
}

// Snapshot is the persistence shape.
type Snapshot struct {
	Key       hotsearch.Key
	Intent    hotsearch.Intent
	Trend     hotsearch.Verdict
	Evidence  []Evidence
	State     State
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Snapshot exports state.
func (o *Opportunity) Snapshot() Snapshot {
	return Snapshot{
		Key:       o.key,
		Intent:    o.intent,
		Trend:     o.trend,
		Evidence:  o.Evidence(),
		State:     o.state,
		Note:      o.note,
		CreatedAt: o.createdAt,
		UpdatedAt: o.updatedAt,
	}
}

// Rehydrate rebuilds the aggregate and checks invariants.
func Rehydrate(s Snapshot) (*Opportunity, error) {
	if s.Key.IsZero() {
		return nil, errors.New("opportunity: rehydrate with empty key")
	}
	if !s.State.Valid() {
		return nil, ErrInvalidState
	}
	for _, e := range s.Evidence {
		if !e.Source.Valid() {
			return nil, fmt.Errorf("opportunity: invalid evidence source %q", e.Source)
		}
	}
	return &Opportunity{
		key:       s.Key,
		intent:    s.Intent,
		trend:     s.Trend,
		evidence:  append([]Evidence(nil), s.Evidence...),
		state:     s.State,
		note:      s.Note,
		createdAt: s.CreatedAt,
		updatedAt: s.UpdatedAt,
	}, nil
}
