// runs.go: 운영 실행 메타 (StartRun/FinishRun/RecentRuns/StaleRuns).
// quantbot status 명령과 ingest 명령이 호출.
package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run은 runs 테이블의 한 행.
type Run struct {
	ID            int64
	JobName       string
	Instance      string
	StartedAt     time.Time
	FinishedAt    *time.Time
	Status        string
	RowsProcessed int
	RetryCount    int
	ErrorMessage  string
}

// RunResult는 FinishRun에 넘길 결과 정보.
type RunResult struct {
	Status        string // "success" / "failed"
	RowsProcessed int
	RetryCount    int
	Error         error // failed일 때만 사용
}

// StartRun은 새 run row를 insert하고 id를 반환한다.
func StartRun(ctx context.Context, pool *pgxpool.Pool, jobName, instance string) (int64, error) {
	var id int64
	err := pool.QueryRow(ctx,
		`INSERT INTO runs (job_name, instance, started_at, status)
		 VALUES ($1, $2, NOW(), 'running')
		 RETURNING id`,
		jobName, instance,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("StartRun INSERT: %w", err)
	}
	return id, nil
}

// FinishRun은 run row의 status, finished_at, 통계 등을 갱신한다.
func FinishRun(ctx context.Context, pool *pgxpool.Pool, id int64, r RunResult) error {
	var errMsg *string
	if r.Error != nil {
		s := r.Error.Error()
		errMsg = &s
	}
	ct, err := pool.Exec(ctx,
		`UPDATE runs SET finished_at = NOW(), status = $1, rows_processed = $2,
		                  retry_count = $3, error_message = $4
		 WHERE id = $5`,
		r.Status, r.RowsProcessed, r.RetryCount, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("FinishRun UPDATE: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("FinishRun: run id %d not found", id)
	}
	return nil
}

// RecentRuns은 최근 limit개 run을 시작 시각 내림차순으로 반환한다.
func RecentRuns(ctx context.Context, pool *pgxpool.Pool, limit int) ([]Run, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, job_name, instance, started_at, finished_at, status,
		        rows_processed, retry_count, COALESCE(error_message, '')
		 FROM runs ORDER BY started_at DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("RecentRuns SELECT: %w", err)
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.JobName, &r.Instance, &r.StartedAt,
			&r.FinishedAt, &r.Status, &r.RowsProcessed, &r.RetryCount, &r.ErrorMessage); err != nil {
			return nil, fmt.Errorf("RecentRuns Scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// StaleRuns은 finished_at IS NULL 이면서 시작된 지 1시간 넘은 run (비정상 종료 의심).
func StaleRuns(ctx context.Context, pool *pgxpool.Pool) ([]Run, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, job_name, instance, started_at, finished_at, status,
		        rows_processed, retry_count, COALESCE(error_message, '')
		 FROM runs
		 WHERE finished_at IS NULL AND started_at < NOW() - INTERVAL '1 hour'
		 ORDER BY started_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("StaleRuns SELECT: %w", err)
	}
	defer rows.Close()

	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.JobName, &r.Instance, &r.StartedAt,
			&r.FinishedAt, &r.Status, &r.RowsProcessed, &r.RetryCount, &r.ErrorMessage); err != nil {
			return nil, fmt.Errorf("StaleRuns Scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
