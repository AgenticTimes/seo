package hotsearch

import (
	"context"
	"time"
)

// Repository persists Keyword aggregates. The interface lives beside the
// aggregate it serves; implementations live under infrastructure.
type Repository interface {
	Get(ctx context.Context, key Key) (*Keyword, error)
	Save(ctx context.Context, k *Keyword) error
	Exists(ctx context.Context, key Key) (bool, error)
	FindChildren(ctx context.Context, parent Key) ([]*Keyword, error)
	ListByState(ctx context.Context, state State, limit int) ([]*Keyword, error)
	ListAll(ctx context.Context, limit int) ([]*Keyword, error)
	ListStale(ctx context.Context, before time.Time) ([]*Keyword, error)
}
