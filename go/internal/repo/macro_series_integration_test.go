//go:build integration

package repo

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
	"github.com/Claude-su-Factory/quant-bot/go/internal/db"
	"github.com/Claude-su-Factory/quant-bot/go/internal/migrate"
)

func setupDB(t *testing.T) *pgxpool.Pool {
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
			t.Fatalf("setupDB DROP %q 실패: %v", stmt, err)
		}
	}
	if err := migrate.Up(bg, pool); err != nil {
		t.Fatalf("마이그레이션 실패: %v", err)
	}
	return pool
}

func TestInsertObservations_InsertsRows(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	obs := []Observation{
		{SeriesID: "T10Y2Y", ObservedAt: mustDate("2026-04-01"), Value: floatPtr(0.25)},
		{SeriesID: "T10Y2Y", ObservedAt: mustDate("2026-04-02"), Value: floatPtr(0.30)},
		{SeriesID: "T10Y2Y", ObservedAt: mustDate("2026-04-03"), Value: nil},
	}
	n, err := InsertObservations(ctx, pool, obs)
	if err != nil {
		t.Fatalf("InsertObservations 실패: %v", err)
	}
	if n != 3 {
		t.Errorf("inserted 기대 3, 실제 %d", n)
	}

	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM macro_series WHERE series_id='T10Y2Y'").Scan(&count)
	if count != 3 {
		t.Errorf("DB 행수 기대 3, 실제 %d", count)
	}
}

func TestInsertObservations_IdempotentOnConflict(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	obs := []Observation{
		{SeriesID: "VIXCLS", ObservedAt: mustDate("2026-04-01"), Value: floatPtr(15.5)},
	}
	if _, err := InsertObservations(ctx, pool, obs); err != nil {
		t.Fatalf("첫 insert 실패: %v", err)
	}

	n, err := InsertObservations(ctx, pool, obs)
	if err != nil {
		t.Fatalf("두번째 insert 실패: %v", err)
	}
	if n != 0 {
		t.Errorf("중복 insert 기대 0행, 실제 %d", n)
	}
}

func TestLastObservedAt_EmptyTable(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	last, err := LastObservedAt(ctx, pool, "BAMLH0A0HYM2")
	if err != nil {
		t.Fatalf("LastObservedAt 실패: %v", err)
	}
	if !last.IsZero() {
		t.Errorf("빈 테이블이면 zero time 기대, 실제 %v", last)
	}
}

func TestLastObservedAt_AfterInsert(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	obs := []Observation{
		{SeriesID: "DFF", ObservedAt: mustDate("2026-04-01"), Value: floatPtr(5.25)},
		{SeriesID: "DFF", ObservedAt: mustDate("2026-04-02"), Value: floatPtr(5.25)},
	}
	InsertObservations(ctx, pool, obs)

	last, err := LastObservedAt(ctx, pool, "DFF")
	if err != nil {
		t.Fatalf("LastObservedAt 실패: %v", err)
	}
	want := mustDate("2026-04-02")
	if !last.Equal(want) {
		t.Errorf("기대 %v, 실제 %v", want, last)
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}

func floatPtr(f float64) *float64 {
	return &f
}
