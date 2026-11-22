package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"seo/internal/discovery/domain/hotsearch"
	"seo/internal/shared/kernel"
)

// KeywordRepository is the SQLite implementation of hotsearch.Repository.
type KeywordRepository struct {
	db *DB
}

// NewKeywordRepository wires the repository to an open database.
func NewKeywordRepository(db *DB) *KeywordRepository {
	return &KeywordRepository{db: db}
}

const keywordColumns = `term, country, entity,
	parent_term, parent_country, parent_entity,
	depth, source, discovered_at, state,
	too_broad, too_broad_reason, verdict, cluster_ref, signals`

// Get loads one keyword with its observations.
func (r *KeywordRepository) Get(ctx context.Context, key hotsearch.Key) (*hotsearch.Keyword, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+keywordColumns+` FROM keywords WHERE term = ? AND country = ? AND entity = ?`,
		string(key.Term), string(key.Country), string(key.Entity))
	snapshot, err := scanSnapshot(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, hotsearch.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get keyword %s: %w", key, err)
	}
	if err := r.attachObservations(ctx, []*hotsearch.Snapshot{&snapshot}); err != nil {
		return nil, err
	}
	return hotsearch.Rehydrate(snapshot)
}

// Save upserts the keyword and replaces its observations in one transaction.
func (r *KeywordRepository) Save(ctx context.Context, k *hotsearch.Keyword) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin save %s: %w", k.Key(), err)
	}
	defer func() { _ = tx.Rollback() }()

	s := k.Snapshot()
	signals, err := json.Marshal(s.Signals)
	if err != nil {
		return fmt.Errorf("sqlite: encode signals %s: %w", s.Key, err)
	}
	var parentTerm, parentCountry, parentEntity any
	if s.Parent != nil {
		parentTerm = string(s.Parent.Term)
		parentCountry = string(s.Parent.Country)
		parentEntity = string(s.Parent.Entity)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO keywords (`+keywordColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(term, country, entity) DO UPDATE SET
			parent_term = excluded.parent_term,
			parent_country = excluded.parent_country,
			parent_entity = excluded.parent_entity,
			depth = excluded.depth,
			source = excluded.source,
			discovered_at = excluded.discovered_at,
			state = excluded.state,
			too_broad = excluded.too_broad,
			too_broad_reason = excluded.too_broad_reason,
			verdict = excluded.verdict,
			cluster_ref = excluded.cluster_ref,
			signals = excluded.signals`,
		string(s.Key.Term), string(s.Key.Country), string(s.Key.Entity),
		parentTerm, parentCountry, parentEntity,
		s.Depth, string(s.Source), formatTime(s.DiscoveredAt), string(s.State),
		boolToInt(s.TooBroad), s.TooBroadReason, string(s.Verdict), string(s.Cluster), string(signals),
	); err != nil {
		return fmt.Errorf("sqlite: upsert keyword %s: %w", s.Key, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM observations WHERE term = ? AND country = ? AND entity = ?`,
		string(s.Key.Term), string(s.Key.Country), string(s.Key.Entity)); err != nil {
		return fmt.Errorf("sqlite: clear observations %s: %w", s.Key, err)
	}
	for _, obs := range s.Observations {
		samples := make([]int, 0, len(obs.Samples()))
		for _, idx := range obs.Samples() {
			samples = append(samples, idx.Int())
		}
		encoded, err := json.Marshal(samples)
		if err != nil {
			return fmt.Errorf("sqlite: encode samples %s/%s: %w", s.Key, obs.Window(), err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO observations (term, country, entity, window, start_at, end_at, samples)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			string(s.Key.Term), string(s.Key.Country), string(s.Key.Entity),
			string(obs.Window()), formatTime(obs.Start()), formatTime(obs.End()), string(encoded),
		); err != nil {
			return fmt.Errorf("sqlite: insert observation %s/%s: %w", s.Key, obs.Window(), err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit save %s: %w", k.Key(), err)
	}
	return nil
}

// Exists reports presence by natural key.
func (r *KeywordRepository) Exists(ctx context.Context, key hotsearch.Key) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM keywords WHERE term = ? AND country = ? AND entity = ? LIMIT 1`,
		string(key.Term), string(key.Country), string(key.Entity)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: exists %s: %w", key, err)
	}
	return true, nil
}

// FindChildren lists direct children of a node.
func (r *KeywordRepository) FindChildren(ctx context.Context, parent hotsearch.Key) ([]*hotsearch.Keyword, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+keywordColumns+` FROM keywords
		 WHERE parent_term = ? AND parent_country = ? AND parent_entity = ?
		 ORDER BY depth, term`,
		string(parent.Term), string(parent.Country), string(parent.Entity))
	if err != nil {
		return nil, fmt.Errorf("sqlite: find children %s: %w", parent, err)
	}
	defer rows.Close()
	return r.collect(ctx, rows)
}

// ListByState lists one pipeline state, newest first.
func (r *KeywordRepository) ListByState(ctx context.Context, state hotsearch.State, limit int) ([]*hotsearch.Keyword, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+keywordColumns+` FROM keywords WHERE state = ?
		 ORDER BY discovered_at DESC, term LIMIT ?`,
		string(state), limitOrDefault(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite: list state %s: %w", state, err)
	}
	defer rows.Close()
	return r.collect(ctx, rows)
}

// ListAll lists every keyword, shallowest first.
func (r *KeywordRepository) ListAll(ctx context.Context, limit int) ([]*hotsearch.Keyword, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+keywordColumns+` FROM keywords ORDER BY depth, term LIMIT ?`,
		limitOrDefault(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite: list all: %w", err)
	}
	defer rows.Close()
	return r.collect(ctx, rows)
}

// ListStale lists keywords discovered before the cutoff for revalidation.
func (r *KeywordRepository) ListStale(ctx context.Context, before time.Time) ([]*hotsearch.Keyword, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+keywordColumns+` FROM keywords
		 WHERE discovered_at < ? ORDER BY discovered_at LIMIT ?`,
		formatTime(before), 200)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list stale: %w", err)
	}
	defer rows.Close()
	return r.collect(ctx, rows)
}

func (r *KeywordRepository) collect(ctx context.Context, rows *sql.Rows) ([]*hotsearch.Keyword, error) {
	var snapshots []*hotsearch.Snapshot
	for rows.Next() {
		snapshot, err := scanSnapshot(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan keyword: %w", err)
		}
		snapshots = append(snapshots, &snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := r.attachObservations(ctx, snapshots); err != nil {
		return nil, err
	}
	out := make([]*hotsearch.Keyword, 0, len(snapshots))
	for _, snapshot := range snapshots {
		k, err := hotsearch.Rehydrate(*snapshot)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

// attachObservations bulk-loads series. The table is small (a few hundred rows
// for a manual run), so one unfiltered read beats N+1 queries.
func (r *KeywordRepository) attachObservations(ctx context.Context, snapshots []*hotsearch.Snapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT term, country, entity, window, start_at, end_at, samples FROM observations`)
	if err != nil {
		return fmt.Errorf("sqlite: load observations: %w", err)
	}
	defer rows.Close()

	byKey := map[string][]hotsearch.Observation{}
	for rows.Next() {
		var term, country, entity, window, startAt, endAt, samples string
		if err := rows.Scan(&term, &country, &entity, &window, &startAt, &endAt, &samples); err != nil {
			return fmt.Errorf("sqlite: scan observation: %w", err)
		}
		w, err := hotsearch.ParseWindow(window)
		if err != nil {
			return err
		}
		start, err := parseTime(startAt)
		if err != nil {
			return err
		}
		end, err := parseTime(endAt)
		if err != nil {
			return err
		}
		var values []int
		if err := json.Unmarshal([]byte(samples), &values); err != nil {
			return fmt.Errorf("sqlite: decode samples: %w", err)
		}
		obs, err := hotsearch.NewObservation(w, start, end, values)
		if err != nil {
			return err
		}
		id := term + "|" + country + "|" + entity
		byKey[id] = append(byKey[id], obs)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		snapshot.Observations = byKey[snapshot.Key.String()]
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanSnapshot(row scannable) (hotsearch.Snapshot, error) {
	var (
		term, country, entity string
		parentTerm            sql.NullString
		parentCountry         sql.NullString
		parentEntity          sql.NullString
		depth                 int
		source, discoveredAt  string
		state, tooBroadReason string
		verdict, clusterRef   string
		signals               string
		tooBroad              int
	)
	if err := row.Scan(
		&term, &country, &entity,
		&parentTerm, &parentCountry, &parentEntity,
		&depth, &source, &discoveredAt, &state,
		&tooBroad, &tooBroadReason, &verdict, &clusterRef, &signals,
	); err != nil {
		return hotsearch.Snapshot{}, err
	}
	key, err := hotsearch.NewKey(kernel.Term(term), kernel.Country(country), kernel.EntityType(entity))
	if err != nil {
		return hotsearch.Snapshot{}, err
	}
	var parent *hotsearch.Key
	if parentTerm.Valid && parentCountry.Valid && parentEntity.Valid {
		p, err := hotsearch.NewKey(
			kernel.Term(parentTerm.String),
			kernel.Country(parentCountry.String),
			kernel.EntityType(parentEntity.String),
		)
		if err != nil {
			return hotsearch.Snapshot{}, err
		}
		parent = &p
	}
	discovered, err := parseTime(discoveredAt)
	if err != nil {
		return hotsearch.Snapshot{}, err
	}
	parsedVerdict, err := hotsearch.ParseVerdict(verdict)
	if err != nil {
		return hotsearch.Snapshot{}, err
	}
	var parsedSignals hotsearch.Signals
	if signals != "" {
		if err := json.Unmarshal([]byte(signals), &parsedSignals); err != nil {
			return hotsearch.Snapshot{}, fmt.Errorf("sqlite: decode signals: %w", err)
		}
	}
	return hotsearch.Snapshot{
		Key:            key,
		Parent:         parent,
		Depth:          depth,
		Source:         hotsearch.Source(source),
		DiscoveredAt:   discovered,
		State:          hotsearch.State(state),
		TooBroad:       tooBroad != 0,
		TooBroadReason: tooBroadReason,
		Verdict:        parsedVerdict,
		Signals:        parsedSignals,
		Cluster:        hotsearch.ClusterRef(clusterRef),
	}, nil
}

func formatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func parseTime(raw string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlite: parse time %q: %w", raw, err)
	}
	return t, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func limitOrDefault(limit int) int {
	if limit <= 0 {
		return 500
	}
	return limit
}
