package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/evaluation/domain/opportunity"
)

// OpportunityRepository is the SQLite implementation of opportunity.Repository.
type OpportunityRepository struct {
	db *DB
}

// NewOpportunityRepository wires the repository.
func NewOpportunityRepository(db *DB) *OpportunityRepository {
	return &OpportunityRepository{db: db}
}

// Get loads one opportunity.
func (r *OpportunityRepository) Get(ctx context.Context, key hotsearch.Key) (*opportunity.Opportunity, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT intent, trend, state, note, evidence, created_at, updated_at
		FROM opportunities WHERE term = ? AND country = ? AND entity = ?`,
		string(key.Term), string(key.Country), string(key.Entity))

	var intent, trend, state, note, evidence, createdAt, updatedAt string
	if err := row.Scan(&intent, &trend, &state, &note, &evidence, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, opportunity.ErrNotFound
		}
		return nil, fmt.Errorf("sqlite: get opportunity %s: %w", key, err)
	}
	return opportunity.Rehydrate(opportunity.Snapshot{
		Key:       key,
		Intent:    hotsearch.Intent(intent),
		Trend:     hotsearch.Verdict(trend),
		State:     opportunity.State(state),
		Note:      note,
		Evidence:  decodeEvidence(evidence),
		CreatedAt: parseTimeOrZero(createdAt),
		UpdatedAt: parseTimeOrZero(updatedAt),
	})
}

// Save upserts the decision.
func (r *OpportunityRepository) Save(ctx context.Context, o *opportunity.Opportunity) error {
	s := o.Snapshot()
	evidence, err := json.Marshal(s.Evidence)
	if err != nil {
		return fmt.Errorf("sqlite: encode evidence %s: %w", s.Key, err)
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO opportunities (term, country, entity, intent, trend, state, note, evidence, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(term, country, entity) DO UPDATE SET
			intent = excluded.intent,
			trend = excluded.trend,
			state = excluded.state,
			note = excluded.note,
			evidence = excluded.evidence,
			updated_at = excluded.updated_at`,
		string(s.Key.Term), string(s.Key.Country), string(s.Key.Entity),
		string(s.Intent), string(s.Trend), string(s.State), s.Note, string(evidence),
		formatTime(s.CreatedAt), formatTime(s.UpdatedAt),
	); err != nil {
		return fmt.Errorf("sqlite: save opportunity %s: %w", s.Key, err)
	}
	return nil
}

// Exists reports presence.
func (r *OpportunityRepository) Exists(ctx context.Context, key hotsearch.Key) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM opportunities WHERE term = ? AND country = ? AND entity = ? LIMIT 1`,
		string(key.Term), string(key.Country), string(key.Entity)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: exists opportunity %s: %w", key, err)
	}
	return true, nil
}

func decodeEvidence(raw string) []opportunity.Evidence {
	var out []opportunity.Evidence
	if raw == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func parseTimeOrZero(raw string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}
