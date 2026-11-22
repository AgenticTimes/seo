package hotsearch

// State is the keyword's position in the discovery pipeline. It is not the
// business decision state — that belongs to evaluation.Opportunity.
type State string

// Pipeline states.
const (
	StateDiscovered    State = "DISCOVERED"
	StateValidated     State = "VALIDATED"
	StateSeedCandidate State = "SEED_CANDIDATE"
	StateTooBroad      State = "TOO_BROAD"
	StateDiscarded     State = "DISCARDED"
)

// ParseState accepts a stored code.
func ParseState(raw string) (State, error) {
	switch s := State(raw); s {
	case StateDiscovered, StateValidated, StateSeedCandidate, StateTooBroad, StateDiscarded:
		return s, nil
	default:
		return "", ErrInvalidState
	}
}

// Valid reports whether the state is known.
func (s State) Valid() bool {
	switch s {
	case StateDiscovered, StateValidated, StateSeedCandidate, StateTooBroad, StateDiscarded:
		return true
	default:
		return false
	}
}

// IsTerminal reports states that end this keyword's discovery path.
func (s State) IsTerminal() bool {
	return s == StateTooBroad || s == StateDiscarded
}

// ClusterRef is a reference to a KeywordCluster in the same context. It stays a
// string so the aggregate never imports the cluster package.
type ClusterRef string
