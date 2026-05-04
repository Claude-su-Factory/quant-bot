// Package repo는 DB 테이블에 대한 CRUD 액세스 레이어다.
// 각 테이블당 한 파일 (macro_series.go, runs.go, ...).
package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Observation은 macro_series 테이블의 한 행 (FRED 관측).
type Observation struct {
	SeriesID   string
	ObservedAt time.Time
	Value      *float64 // NULL 가능 (FRED 휴장일)
}

// InsertObservations는 obs를 macro_series에 INSERT한다 (ON CONFLICT DO NOTHING).
// 반환값은 실제 새로 들어간 행 수 (중복은 카운트 안 됨).
func InsertObservations(ctx context.Context, pool *pgxpool.Pool, obs []Observation) (int, error) {
	if len(obs) == 0 {
		return 0, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("Begin: %w", err)
	}
	defer tx.Rollback(ctx)

	const batch = 100
	inserted := 0
	for start := 0; start < len(obs); start += batch {
		end := start + batch
		if end > len(obs) {
			end = len(obs)
		}
		ct, err := insertBatch(ctx, tx, obs[start:end])
		if err != nil {
			return 0, err
		}
		inserted += ct
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("Commit: %w", err)
	}
	return inserted, nil
}

func insertBatch(ctx context.Context, tx pgx.Tx, obs []Observation) (int, error) {
	args := make([]any, 0, len(obs)*3)
	values := ""
	for i, o := range obs {
		if i > 0 {
			values += ","
		}
		values += fmt.Sprintf("($%d,$%d,$%d)", i*3+1, i*3+2, i*3+3)
		args = append(args, o.SeriesID, o.ObservedAt, o.Value)
	}
	q := fmt.Sprintf(
		"INSERT INTO macro_series (series_id, observed_at, value) VALUES %s ON CONFLICT (series_id, observed_at) DO NOTHING",
		values,
	)
	ct, err := tx.Exec(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("INSERT: %w", err)
	}
	return int(ct.RowsAffected()), nil
}

// LastObservedAt은 series_id의 마지막 observed_at을 반환한다.
// 데이터 없으면 zero time + nil.
func LastObservedAt(ctx context.Context, pool *pgxpool.Pool, seriesID string) (time.Time, error) {
	var t *time.Time
	err := pool.QueryRow(ctx,
		"SELECT MAX(observed_at) FROM macro_series WHERE series_id = $1",
		seriesID,
	).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("MAX(observed_at): %w", err)
	}
	if t == nil {
		return time.Time{}, nil
	}
	return *t, nil
}
