package hotsearch

import "errors"

var (
	// ErrInvalidVerdict rejects unknown verdict codes from storage.
	ErrInvalidVerdict = errors.New("keyword: invalid verdict")
	// ErrInvalidState rejects unknown pipeline states from storage.
	ErrInvalidState = errors.New("keyword: invalid state")
	// ErrReasonRequired forces a human-readable reason on every exit path.
	ErrReasonRequired = errors.New("keyword: reason is required")
	// ErrTooBroad blocks BR-04 words from becoming seeds or opportunities.
	ErrTooBroad = errors.New("keyword: too broad to advance")
	// ErrNotValidated blocks BR-10: only emerging or sustained words advance.
	ErrNotValidated = errors.New("keyword: trend is not validated for the next round")
	// ErrDepthExceeded blocks BR-09: expansion stops at the policy depth.
	ErrDepthExceeded = errors.New("keyword: expansion depth exceeded")
	// ErrInvalidTransition rejects business-verb calls that do not apply.
	ErrInvalidTransition = errors.New("keyword: transition not allowed in current state")
	// ErrNotFound is returned by repositories when a keyword is absent.
	ErrNotFound = errors.New("keyword: not found")
)
