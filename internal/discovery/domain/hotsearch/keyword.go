package hotsearch

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"seo/internal/shared/kernel"
)

// Keyword is the aggregate root for one node of the expansion graph.
type Keyword struct {
	key            Key
	parent         *Key
	depth          int
	source         Source
	discoveredAt   time.Time
	state          State
	tooBroad       bool
	tooBroadReason string
	verdict        Verdict
	signals        Signals
	cluster        ClusterRef
	observations   map[Window]Observation
}

// NewSeed creates a root keyword. Seeds have no parent and no source signal.
func NewSeed(key Key, now time.Time) *Keyword {
	return &Keyword{
		key:          key,
		depth:        0,
		source:       SourceSeed,
		discoveredAt: now.UTC(),
		state:        StateDiscovered,
		verdict:      VerdictUnknown,
		observations: map[Window]Observation{},
	}
}

// NewChild creates the next-round node for a discovered term. It enforces the
// parent's expandability and the BR-09 depth bound.
func NewChild(parent *Keyword, term kernel.Term, source Source, now time.Time, policy ExpansionPolicy) (*Keyword, error) {
	if parent == nil {
		return nil, errors.New("keyword: nil parent")
	}
	if !parent.CanBeExpanded() {
		return nil, fmt.Errorf("%w: parent %s in state %s", ErrInvalidTransition, parent.key, parent.state)
	}
	if term == "" {
		return nil, errors.New("keyword: empty term")
	}
	if !source.Valid() {
		return nil, fmt.Errorf("keyword: invalid source %q", source)
	}
	depth := parent.depth + 1
	if !policy.AllowsDepth(depth) {
		return nil, fmt.Errorf("%w: %d > %d", ErrDepthExceeded, depth, policy.MaxDepth)
	}
	key, err := NewKey(term, parent.key.Country, parent.key.Entity)
	if err != nil {
		return nil, err
	}
	parentKey := parent.key
	return &Keyword{
		key:          key,
		parent:       &parentKey,
		depth:        depth,
		source:       source,
		discoveredAt: now.UTC(),
		state:        StateDiscovered,
		verdict:      VerdictUnknown,
		observations: map[Window]Observation{},
	}, nil
}

// Key returns the natural identity.
func (k *Keyword) Key() Key { return k.key }

// Parent returns the parent key, or nil for a root seed.
func (k *Keyword) Parent() *Key {
	if k.parent == nil {
		return nil
	}
	p := *k.parent
	return &p
}

// Depth returns the expansion depth.
func (k *Keyword) Depth() int { return k.depth }

// Source returns the discovery source (BR-02/BR-07).
func (k *Keyword) Source() Source { return k.source }

// DiscoveredAt returns when the node was first seen.
func (k *Keyword) DiscoveredAt() time.Time { return k.discoveredAt }

// State returns the pipeline state.
func (k *Keyword) State() State { return k.state }

// Verdict returns the last BR-01 classification.
func (k *Keyword) Verdict() Verdict { return k.verdict }

// Signals returns the last computed analysis snapshot.
func (k *Keyword) Signals() Signals { return k.signals }

// TooBroad reports the BR-04 flag.
func (k *Keyword) TooBroad() bool { return k.tooBroad }

// TooBroadReason returns why the word was judged too broad.
func (k *Keyword) TooBroadReason() string { return k.tooBroadReason }

// Cluster returns the cluster reference, if any.
func (k *Keyword) Cluster() ClusterRef { return k.cluster }

// Observation returns the series for one window.
func (k *Keyword) Observation(w Window) (Observation, bool) {
	o, ok := k.observations[w]
	return o, ok
}

// Observations returns all series, shortest window first.
func (k *Keyword) Observations() []Observation {
	out := make([]Observation, 0, len(k.observations))
	for _, w := range AllWindows() {
		if o, ok := k.observations[w]; ok {
			out = append(out, o)
		}
	}
	return out
}

// CanBeExpanded reports whether this node may drive another round: only the
// root seed and confirmed seed candidates can.
func (k *Keyword) CanBeExpanded() bool {
	if k.state.IsTerminal() || k.tooBroad {
		return false
	}
	return k.depth == 0 || k.state == StateSeedCandidate
}

// RecordObservation stores or replaces the series of one window. One window has
// exactly one current observation.
func (k *Keyword) RecordObservation(o Observation) error {
	if o.IsZero() {
		return ErrInvalidObservation
	}
	if k.state.IsTerminal() {
		return fmt.Errorf("%w: %s keyword accepts no observations", ErrInvalidTransition, k.state)
	}
	if k.observations == nil {
		k.observations = map[Window]Observation{}
	}
	k.observations[o.Window()] = o
	return nil
}

// AssessTrend applies BR-01 and stores the verdict. A validated verdict moves an
// unvalidated keyword forward; later states are never downgraded by reassessment.
func (k *Keyword) AssessTrend(policy TrendPolicy) Verdict {
	if k.state.IsTerminal() {
		return k.verdict
	}
	k.signals = ComputeSignals(k.key.Term.String(), k.observations, policy)
	verdict, reasons := analyzeTrend(k.observations, k.signals, policy)
	k.signals.Reasons = reasons
	k.verdict = verdict
	if k.state == StateDiscovered && verdict != VerdictInsufficient {
		k.state = StateValidated
	}
	return verdict
}

// AnalyzeChildren applies BR-18 to the neighborhood. The aggregate cannot load
// its children, so the application layer supplies the counts.
func (k *Keyword) AnalyzeChildren(total, brands int) {
	if total <= 0 {
		k.signals.BrandShare = 0
		k.signals.ChildCount = 0
		return
	}
	k.signals.ChildCount = total
	k.signals.BrandShare = float64(brands) / float64(total)
}

// MarkTooBroad applies BR-04. Terminal keywords stay closed: flagging them
// again would erase the reason they left the path.
func (k *Keyword) MarkTooBroad(reason string) error {
	if reason == "" {
		return ErrReasonRequired
	}
	if k.state.IsTerminal() {
		return fmt.Errorf("%w: %s keyword is closed", ErrInvalidTransition, k.state)
	}
	if k.state == StateSeedCandidate {
		return fmt.Errorf("%w: already advanced", ErrInvalidTransition)
	}
	k.tooBroad = true
	k.tooBroadReason = reason
	k.state = StateTooBroad
	return nil
}

// ConfirmAsSeedCandidate applies BR-09 and BR-10 before a word may drive the
// next round.
func (k *Keyword) ConfirmAsSeedCandidate(policy ExpansionPolicy) error {
	if k.tooBroad {
		return ErrTooBroad
	}
	if k.state.IsTerminal() {
		return fmt.Errorf("%w: %s", ErrInvalidTransition, k.state)
	}
	if !k.verdict.AllowsSeed() {
		return fmt.Errorf("%w: verdict %s", ErrNotValidated, k.verdict)
	}
	if !policy.AllowsDepth(k.depth) {
		return fmt.Errorf("%w: depth %d > %d", ErrDepthExceeded, k.depth, policy.MaxDepth)
	}
	if k.depth == 0 {
		return fmt.Errorf("%w: root seed is already in the next round", ErrInvalidTransition)
	}
	k.state = StateSeedCandidate
	return nil
}

// Discard exits the discovery path with a recorded reason.
func (k *Keyword) Discard(reason string) error {
	if reason == "" {
		return ErrReasonRequired
	}
	k.state = StateDiscarded
	return nil
}

// AttachCluster links the keyword to a synonym cluster (BR-05).
func (k *Keyword) AttachCluster(ref ClusterRef) error {
	if ref == "" {
		return errors.New("keyword: empty cluster reference")
	}
	k.cluster = ref
	return nil
}

// Reopen returns a decided keyword to the queue for revalidation.
func (k *Keyword) Reopen() error {
	if k.tooBroad {
		return fmt.Errorf("%w: too broad keywords stay closed", ErrInvalidTransition)
	}
	switch k.state {
	case StateValidated, StateSeedCandidate, StateDiscarded:
	default:
		return fmt.Errorf("%w: nothing to reopen from %s", ErrInvalidTransition, k.state)
	}
	k.state = StateDiscovered
	k.verdict = VerdictUnknown
	// Analysis is derived; force a fresh assessment instead of showing stale reasons.
	k.signals = Signals{}
	return nil
}

// classify keeps the old signature for tests and callers that only have
// observations. It computes signals from the series alone.
func classify(obs map[Window]Observation, p TrendPolicy) Verdict {
	return classifyTrend(obs, ComputeSignals("", obs, p), p)
}

// Reason codes explain a verdict. Presentation maps them to text.
const (
	ReasonNo30D          = "NO_30D"
	ReasonNoisy          = "NOISY"
	ReasonDeclining30D   = "DECLINING_30D"
	ReasonFlat30D        = "FLAT_30D"
	ReasonSpike          = "SPIKE"
	ReasonSeasonal       = "SEASONAL"
	ReasonSeasonalGrowth = "SEASONAL_GROWTH"
	ReasonMissing90D     = "MISSING_90D"
	ReasonMissing12M     = "MISSING_12M"
	ReasonThinLongWindow = "THIN_LONG_WINDOW"
	ReasonLongTerm       = "LONG_TERM"
	ReasonShortLived     = "SHORT_LIVED"
	ReasonStable90D      = "STABLE_90D"
	ReasonSustained      = "SUSTAINED"
	ReasonEmerging       = "EMERGING"
)

// analyzeTrend returns the verdict and the reasons that produced it.
func analyzeTrend(obs map[Window]Observation, sig Signals, p TrendPolicy) (Verdict, []string) {
	verdict := classifyTrend(obs, sig, p)
	reasons := make([]string, 0, 4)
	switch verdict {
	case VerdictInsufficient:
		switch {
		case !hasWindow(obs, Window30D):
			reasons = append(reasons, ReasonNo30D)
		case sig.Noisy(p):
			reasons = append(reasons, ReasonNoisy)
		case !hasWindow(obs, Window90D):
			reasons = append(reasons, ReasonMissing90D)
		case thinLongWindow(obs, p):
			reasons = append(reasons, ReasonThinLongWindow)
		default:
			reasons = append(reasons, ReasonMissing12M)
		}
	case VerdictDeclining:
		reasons = append(reasons, ReasonDeclining30D)
	case VerdictStable:
		if d30, ok := obs[Window30D]; ok && d30.Direction(p) == DirectionFlat {
			reasons = append(reasons, ReasonFlat30D)
		} else {
			reasons = append(reasons, ReasonStable90D)
		}
	case VerdictEvent:
		reasons = append(reasons, ReasonSpike)
	case VerdictSeasonal:
		reasons = append(reasons, ReasonSeasonal)
	case VerdictSeasonalGrowth:
		reasons = append(reasons, ReasonSeasonal, ReasonSeasonalGrowth)
	case VerdictLongTerm:
		reasons = append(reasons, ReasonLongTerm)
	case VerdictShortLived:
		reasons = append(reasons, ReasonShortLived)
	case VerdictSustained:
		reasons = append(reasons, ReasonSustained)
	case VerdictEmerging:
		reasons = append(reasons, ReasonEmerging)
	}
	return verdict, reasons
}

func hasWindow(obs map[Window]Observation, w Window) bool {
	_, ok := obs[w]
	return ok
}

// thinLongWindow reports whether a required long window is present but has too
// few samples to classify a direction (BR-16). Such a window may not decide
// the cross-check.
func thinLongWindow(obs map[Window]Observation, p TrendPolicy) bool {
	for _, w := range []Window{Window90D, Window12M} {
		if o, ok := obs[w]; ok && o.Direction(p) == DirectionUnknown {
			return true
		}
	}
	return false
}

// classifyTrend is the pure BR-01 table extended by the signal rules.
//
// Order matters:
//  1. A missing or noisy 30-day shape cannot support any verdict (BR-16).
//  2. The current direction decides whether the word is moving at all.
//  3. A one-off spike is an event, not growth (BR-15).
//  4. Strong seasonality switches to the structural trend (BR-12/13).
//  5. Otherwise the window cross-check applies. A long window that is present
//     but too thin to classify a direction (BR-16) cannot decide the verdict.
func classifyTrend(obs map[Window]Observation, sig Signals, p TrendPolicy) Verdict {
	d30, ok := obs[Window30D]
	if !ok || sig.Noisy(p) {
		return VerdictInsufficient
	}
	switch d30.Direction(p) {
	case DirectionDown:
		return VerdictDeclining
	case DirectionFlat, DirectionUnknown:
		return VerdictStable
	}

	d90, ok90 := obs[Window90D]
	d12, ok12 := obs[Window12M]

	if sig.IsSpike(p) && (!ok90 || d90.Direction(p) != DirectionUp) {
		return VerdictEvent
	}

	if sig.HasSeasonality(p) {
		if sig.StructuralGrowth > 0 {
			return VerdictSeasonalGrowth
		}
		return VerdictSeasonal
	}

	if !ok90 || !ok12 {
		return VerdictInsufficient
	}

	d90Dir := d90.Direction(p)
	d12Dir := d12.Direction(p)
	if d90Dir == DirectionUnknown || d12Dir == DirectionUnknown {
		return VerdictInsufficient
	}

	if y5, ok5 := obs[Window5Y]; ok5 &&
		y5.Direction(p) == DirectionUp &&
		d90Dir == DirectionUp {
		return VerdictLongTerm
	}

	if d90Dir != DirectionUp {
		if d12Dir == DirectionUp {
			return VerdictStable
		}
		return VerdictShortLived
	}

	if d12Dir == DirectionUp {
		return VerdictSustained
	}
	return VerdictEmerging
}

// Snapshot is the persistence shape. Repositories translate it; the aggregate
// stays free of storage concerns.
type Snapshot struct {
	Key            Key
	Parent         *Key
	Depth          int
	Source         Source
	DiscoveredAt   time.Time
	State          State
	TooBroad       bool
	TooBroadReason string
	Verdict        Verdict
	Signals        Signals
	Cluster        ClusterRef
	Observations   []Observation
}

// Snapshot exports the aggregate state.
func (k *Keyword) Snapshot() Snapshot {
	obs := k.Observations()
	out := make([]Observation, len(obs))
	copy(out, obs)
	var parent *Key
	if k.parent != nil {
		p := *k.parent
		parent = &p
	}
	return Snapshot{
		Key:            k.key,
		Parent:         parent,
		Depth:          k.depth,
		Source:         k.source,
		DiscoveredAt:   k.discoveredAt,
		State:          k.state,
		TooBroad:       k.tooBroad,
		TooBroadReason: k.tooBroadReason,
		Verdict:        k.verdict,
		Signals:        k.signals,
		Cluster:        k.cluster,
		Observations:   out,
	}
}

// Rehydrate rebuilds an aggregate from storage and re-checks its invariants.
func Rehydrate(s Snapshot) (*Keyword, error) {
	if s.Key.IsZero() {
		return nil, errors.New("keyword: rehydrate with empty key")
	}
	if s.Depth < 0 {
		return nil, errors.New("keyword: negative depth")
	}
	if !s.Source.Valid() {
		return nil, fmt.Errorf("keyword: invalid source %q", s.Source)
	}
	state := s.State
	if state == "" {
		state = StateDiscovered
	}
	if !state.Valid() {
		return nil, ErrInvalidState
	}
	verdict := s.Verdict
	if verdict == "" {
		verdict = VerdictUnknown
	}
	if _, err := ParseVerdict(string(verdict)); err != nil {
		return nil, err
	}
	observations := make(map[Window]Observation, len(s.Observations))
	for _, o := range s.Observations {
		if !o.Window().Valid() {
			return nil, ErrInvalidWindow
		}
		if _, dup := observations[o.Window()]; dup {
			return nil, fmt.Errorf("keyword: duplicate window %s", o.Window())
		}
		observations[o.Window()] = o
	}
	k := &Keyword{
		key:            s.Key,
		depth:          s.Depth,
		source:         s.Source,
		discoveredAt:   s.DiscoveredAt,
		state:          state,
		tooBroad:       s.TooBroad,
		tooBroadReason: s.TooBroadReason,
		verdict:        verdict,
		signals:        s.Signals,
		cluster:        s.Cluster,
		observations:   observations,
	}
	if s.Parent != nil {
		p := *s.Parent
		k.parent = &p
	}
	return k, nil
}

// SortedKeys orders keys deterministically for diffable reports.
func SortedKeys(keys []Key) []Key {
	out := make([]Key, len(keys))
	copy(out, keys)
	sort.SliceStable(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}
