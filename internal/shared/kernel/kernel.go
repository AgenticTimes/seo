// Package kernel holds rule-free value objects shared by bounded contexts.
package kernel

import (
	"errors"
	"strings"
	"unicode"
)

// ErrEmptyTerm rejects blank input before it reaches the domain.
var ErrEmptyTerm = errors.New("term is empty")

// Term is a normalized search term (trimmed, lowercased, single-spaced).
type Term string

// NewTerm normalizes raw user input. Two spellings of the same word must
// produce the same Term, otherwise keyword identity and dedupe break.
func NewTerm(raw string) (Term, error) {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	if b.Len() == 0 {
		return "", ErrEmptyTerm
	}
	return Term(b.String()), nil
}

// String returns the stored form.
func (t Term) String() string { return string(t) }

// Country is an uppercase market code; "WORLDWIDE" is the default market.
type Country string

// CountryWorldwide is the default market when the operator has no target.
const CountryWorldwide Country = "WORLDWIDE"

// NewCountry validates and uppercases a market code.
func NewCountry(raw string) (Country, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return "", errors.New("country is empty")
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && r != '-' {
			return "", errors.New("country must be letters or '-'")
		}
	}
	return Country(s), nil
}

// String returns the stored form.
func (c Country) String() string { return string(c) }

// EntityType is how Google Trends resolves a query (BR-08: Topic first).
type EntityType string

// Supported query entity types.
const (
	EntityTopic      EntityType = "TOPIC"
	EntitySearchTerm EntityType = "SEARCH_TERM"
)

// ParseEntityType accepts the canonical names, case-insensitively.
func ParseEntityType(raw string) (EntityType, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case string(EntityTopic):
		return EntityTopic, nil
	case string(EntitySearchTerm), "SEARCHTERM":
		return EntitySearchTerm, nil
	default:
		return "", errors.New("unknown entity type")
	}
}

// Valid reports whether the value is a supported entity type.
func (e EntityType) Valid() bool {
	return e == EntityTopic || e == EntitySearchTerm
}

// String returns the stored form.
func (e EntityType) String() string { return string(e) }

// ChooseEntityType encodes BR-08: prefer Topic when the provider offers it,
// because a Topic covers the concept across languages and spellings.
func ChooseEntityType(topicAvailable bool) EntityType {
	if topicAvailable {
		return EntityTopic
	}
	return EntitySearchTerm
}
