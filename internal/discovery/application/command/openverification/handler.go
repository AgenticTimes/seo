// Package openverification opens a page that the operator must complete by hand
// when the provider asks for human verification. It is operator recovery, not a
// business step, so it only talks to the gateway capability.
package openverification

import (
	"context"
	"errors"
	"fmt"

	"seo/internal/discovery/application/support"
	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/discovery/ports"
)

// Command is the intent to open the verification page for one keyword.
type Command struct {
	Term    string
	Country string
	Entity  string
}

// Result reports whether the tab is in a browser the operator can see.
type Result struct {
	Keyword hotsearch.Key
	Visible bool
}

// Handler delegates to adapters that support a visible browser.
type Handler struct {
	capable ports.VerificationCapable
}

// New detects the optional capability once.
func New(gateway ports.TrendsGateway) *Handler {
	capable, _ := gateway.(ports.VerificationCapable)
	return &Handler{capable: capable}
}

// Available reports whether this adapter can open a verification page.
func (h *Handler) Available() bool { return h.capable != nil }

// Visible reports whether the operator can see the browser window.
func (h *Handler) Visible() bool { return h.capable != nil && h.capable.VerificationVisible() }

// Execute opens the page and keeps it open.
func (h *Handler) Execute(ctx context.Context, cmd Command) (Result, error) {
	var out Result
	key, err := support.KeyFromStrings(cmd.Term, cmd.Country, cmd.Entity)
	if err != nil {
		return out, err
	}
	out.Keyword = key
	out.Visible = h.Visible()
	if h.capable == nil {
		return out, errors.New("当前数据源不支持打开验证页")
	}
	if err := h.capable.OpenVerification(ctx, ports.VerificationRequest{
		Term:    key.Term,
		Country: key.Country,
		Entity:  key.Entity,
	}); err != nil {
		return out, fmt.Errorf("open verification for %s: %w", key, err)
	}
	return out, nil
}
