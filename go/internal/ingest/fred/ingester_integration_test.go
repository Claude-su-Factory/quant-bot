//go:build integration

package fred

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
	"github.com/Claude-su-Factory/quant-bot/go/internal/db"
	"github.com/Claude-su-Factory/quant-bot/go/internal/migrate"
	"github.com/Claude-su-Factory/quant-bot/go/internal/retry"
)

func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("RUN_INTEGRATION=1 환경변수 없음")
	}
	cfg := config.DatabaseConfig{
		Host: "localhost", Port: 5432, Name: "quantbot",
		User: "quantbot", Password: "changeme",
		PoolMin: 1, PoolMax: 5,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("DB 풀 실패: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	bg := context.Background()
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS runs CASCADE",
		"DROP TABLE IF EXISTS macro_series CASCADE",
		"DROP TABLE IF EXISTS goose_db_version CASCADE",
	} {
		if _, err := pool.Exec(bg, stmt); err != nil {
			t.Fatalf("freshDB DROP %q 실패: %v", stmt, err)
		}
	}
	if err := migrate.Up(bg, pool); err != nil {
		t.Fatalf("마이그레이션 실패: %v", err)
	}
	return pool
}

func retryCfg() retry.Config {
	return retry.Config{
		MaxAttempts:       3,
		BackoffInitialMs:  10,
		BackoffMultiplier: 2.0,
	}
}

func TestRun_IngestsFromMockFRED(t *testing.T) {
	pool := freshDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		series := r.URL.Query().Get("series_id")
		w.Header().Set("Content-Type", "application/json")
		switch series {
		case "T10Y2Y":
			w.Write([]byte(`{"observations":[{"date":"2026-04-01","value":"0.25"},{"date":"2026-04-02","value":"0.30"}]}`))
		case "VIXCLS":
			w.Write([]byte(`{"observations":[{"date":"2026-04-01","value":"15.5"}]}`))
		default:
			w.Write([]byte(`{"observations":[]}`))
		}
	}))
	defer server.Close()

	cfg := Config{
		Series:            []string{"T10Y2Y", "VIXCLS"},
		BackfillStartDate: mustDate("2026-04-01"),
		Retry:             retryCfg(),
	}
	client := New(server.URL, "test_api_key")
	ing := NewIngester(client, pool, cfg, "paper")

	ctx := context.Background()
	res, err := ing.Run(ctx)
	if err != nil {
		t.Fatalf("Run 실패: %v", err)
	}
	if res.RowsProcessed != 3 {
		t.Errorf("rows 기대 3, 실제 %d", res.RowsProcessed)
	}
}

func TestRun_Idempotent(t *testing.T) {
	pool := freshDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"observations":[{"date":"2026-04-01","value":"0.25"}]}`))
	}))
	defer server.Close()

	cfg := Config{
		Series:            []string{"T10Y2Y"},
		BackfillStartDate: mustDate("2026-04-01"),
		Retry:             retryCfg(),
	}
	client := New(server.URL, "test")
	ing := NewIngester(client, pool, cfg, "paper")
	ctx := context.Background()

	r1, err := ing.Run(ctx)
	if err != nil {
		t.Fatalf("첫번째 Run 실패: %v", err)
	}
	if r1.RowsProcessed != 1 {
		t.Errorf("첫 실행 rows 기대 1, 실제 %d", r1.RowsProcessed)
	}
	r2, err := ing.Run(ctx)
	if err != nil {
		t.Fatalf("두번째 Run 실패: %v", err)
	}
	if r2.RowsProcessed != 0 {
		t.Errorf("idempotent 두번째 rows 기대 0, 실제 %d", r2.RowsProcessed)
	}
}
