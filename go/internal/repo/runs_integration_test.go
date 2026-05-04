//go:build integration

package repo

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRunsCRUD_StartFinishSuccess(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	id, err := StartRun(ctx, pool, "ingest_fred", "paper")
	if err != nil {
		t.Fatalf("StartRun 실패: %v", err)
	}
	if id == 0 {
		t.Errorf("id 0이면 안 됨")
	}

	if err := FinishRun(ctx, pool, id, RunResult{
		Status:        "success",
		RowsProcessed: 100,
		RetryCount:    0,
	}); err != nil {
		t.Fatalf("FinishRun 실패: %v", err)
	}

	var status string
	var rows int
	pool.QueryRow(ctx, "SELECT status, rows_processed FROM runs WHERE id=$1", id).
		Scan(&status, &rows)
	if status != "success" {
		t.Errorf("status 기대 success, 실제 %q", status)
	}
	if rows != 100 {
		t.Errorf("rows 기대 100, 실제 %d", rows)
	}
}

func TestRunsCRUD_FinishFail_RecordsError(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	id, _ := StartRun(ctx, pool, "ingest_fred", "paper")
	wantErr := errors.New("FRED 503")
	FinishRun(ctx, pool, id, RunResult{
		Status: "failed",
		Error:  wantErr,
	})

	var msg string
	pool.QueryRow(ctx, "SELECT error_message FROM runs WHERE id=$1", id).Scan(&msg)
	if msg == "" {
		t.Errorf("error_message 기록 안 됨")
	}
}

func TestRecentRuns_OrderedByStarted(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		id, _ := StartRun(ctx, pool, "ingest_fred", "paper")
		FinishRun(ctx, pool, id, RunResult{Status: "success", RowsProcessed: i})
		time.Sleep(5 * time.Millisecond)
	}

	runs, err := RecentRuns(ctx, pool, 10)
	if err != nil {
		t.Fatalf("RecentRuns 실패: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("3개 기대, 실제 %d", len(runs))
	}
	if runs[0].RowsProcessed != 2 {
		t.Errorf("최신 우선 정렬 X, 첫 row=%d", runs[0].RowsProcessed)
	}
}

func TestStaleRuns_FindsAbnormallyOpen(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	pool.Exec(ctx,
		"INSERT INTO runs (job_name, started_at, status) VALUES ($1, NOW() - INTERVAL '2 hours', 'running')",
		"ingest_fred",
	)
	stale, err := StaleRuns(ctx, pool)
	if err != nil {
		t.Fatalf("StaleRuns 실패: %v", err)
	}
	if len(stale) != 1 {
		t.Errorf("stale 1개 기대, 실제 %d", len(stale))
	}
}
