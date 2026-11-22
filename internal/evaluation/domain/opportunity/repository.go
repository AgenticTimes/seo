package opportunity

import (
	"context"

	"seo/internal/discovery/domain/hotsearch"
)

// Repository stores one opportunity per keyword.
type Repository interface {
	Get(ctx context.Context, key hotsearch.Key) (*Opportunity, error)
	Save(ctx context.Context, o *Opportunity) error
	Exists(ctx context.Context, key hotsearch.Key) (bool, error)
}
