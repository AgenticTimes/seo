package opportunity

import "errors"

var (
	// ErrInvalidState rejects unknown stored states.
	ErrInvalidState = errors.New("opportunity: invalid state")
	// ErrEvidenceEmpty requires a URL or a summary.
	ErrEvidenceEmpty = errors.New("opportunity: evidence needs a url or a summary")
	// ErrDuplicateEvidence rejects the same URL twice.
	ErrDuplicateEvidence = errors.New("opportunity: duplicate evidence")
	// ErrGateFailed blocks approval when BR-20 is not met.
	ErrGateFailed = errors.New("opportunity: approval gate failed")
	// ErrNoteRequired requires a reason for rejection.
	ErrNoteRequired = errors.New("opportunity: note is required")
	// ErrClosed rejects changes after a decision.
	ErrClosed = errors.New("opportunity: decision is final")
	// ErrNotFound is returned by repositories.
	ErrNotFound = errors.New("opportunity: not found")
)
