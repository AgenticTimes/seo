// Package ports defines the outbound contracts of the discovery context.
package ports

import (
	"context"
	"errors"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/shared/kernel"
)

// Adapter-level failures. Presentation maps these to operator language.
var (
	// ErrRateLimited means the provider returned 429 or an equivalent block.
	ErrRateLimited = errors.New("trends: provider rate limit")
	// ErrConsentRequired means a consent or interstitial page blocked the data.
	ErrConsentRequired = errors.New("trends: consent or interstitial page blocks data")
	// ErrUnexpectedResponse means the provider returned data in an unknown shape.
	ErrUnexpectedResponse = errors.New("trends: unexpected response")
	// ErrUnavailable means the browser could not reach the provider at all.
	ErrUnavailable = errors.New("trends: provider unreachable")
	// ErrCaptchaRequired means Google redirected to its "unusual traffic" check.
	ErrCaptchaRequired = errors.New("trends: human verification required")
)

// RelatedRequest asks for related queries of one seed.
type RelatedRequest struct {
	Term    kernel.Term
	Country kernel.Country
	Entity  kernel.EntityType
}

// DiscoveredTerm is one related query with its provider bucket.
type DiscoveredTerm struct {
	Term     kernel.Term
	Source   hotsearch.Source
	RisePct  int
	Breakout bool
}

// RelatedTopic is one related topic; it explains the domain, not the phrasing.
type RelatedTopic struct {
	Topic    string
	RisePct  int
	Breakout bool
}

// RelatedResult is the collector's answer for one seed.
type RelatedResult struct {
	Queries []DiscoveredTerm
	Topics  []RelatedTopic
}

// InterestRequest asks for one window's series.
type InterestRequest struct {
	Term    kernel.Term
	Country kernel.Country
	Entity  kernel.EntityType
	Window  hotsearch.Window
}

// InterestSeries is a raw normalized series. Values are 0-100 and only
// meaningful inside this window (BR-06).
type InterestSeries struct {
	Window hotsearch.Window
	Start  time.Time
	End    time.Time
	Values []int
}

// VerificationRequest asks the adapter to open a page that a human can act on.
type VerificationRequest struct {
	Term    kernel.Term
	Country kernel.Country
	Entity  kernel.EntityType
}

// VerificationCapable is an optional capability. Adapters that drive a visible
// browser implement it so the operator can complete a CAPTCHA.
type VerificationCapable interface {
	OpenVerification(ctx context.Context, req VerificationRequest) error
	VerificationVisible() bool
}

// TrendsGateway is the anti-corruption port over Google Trends.
type TrendsGateway interface {
	RelatedQueries(ctx context.Context, req RelatedRequest) (RelatedResult, error)
	InterestOverTime(ctx context.Context, req InterestRequest) (InterestSeries, error)
}
