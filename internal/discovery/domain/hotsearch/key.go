package hotsearch

import (
	"errors"
	"fmt"

	"seo/internal/shared/kernel"
)

// Key is a keyword's natural identity. Two discoveries with the same key are
// the same node in the graph, so dedupe is a key comparison, not a scan.
type Key struct {
	Term    kernel.Term
	Country kernel.Country
	Entity  kernel.EntityType
}

// NewKey validates the identity parts.
func NewKey(term kernel.Term, country kernel.Country, entity kernel.EntityType) (Key, error) {
	if term == "" {
		return Key{}, errors.New("keyword: empty term")
	}
	if country == "" {
		return Key{}, errors.New("keyword: empty country")
	}
	if !entity.Valid() {
		return Key{}, fmt.Errorf("keyword: invalid entity type %q", entity)
	}
	return Key{Term: term, Country: country, Entity: entity}, nil
}

// String is the stable storage form.
func (k Key) String() string {
	return string(k.Term) + "|" + string(k.Country) + "|" + string(k.Entity)
}

// IsZero reports whether the key is unset.
func (k Key) IsZero() bool { return k == Key{} }

// Equal reports identity equality.
func (k Key) Equal(other Key) bool { return k == other }
