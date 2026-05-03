//go:build integration

package migrate

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
	"github.com/Claude-su-Factory/quant-bot/go/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func skipIfNoIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("RUN_INTEGRATION=1 환경변수 없음")
	}
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg := config.DatabaseConfig{
		Host: "localhost", Port: 5432, Name: "quantbot",
		User: "quantbot", Password: "changeme",
		PoolMin: 1, PoolMax: 5,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("DB 풀 생성 실패: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

// resetDB는 모든 마이그레이션 산출물을 제거하여 clean slate 상태로 만든다.
// TimescaleDB 하이퍼테이블은 다른 테이블과 함께 DROP할 수 없으므로 개별 DROP을 사용한다.
// timescaledb extension은 shared_preload_libraries로 서버 프로세스에 이미 로드되어 있어
// DROP EXTENSION 후 재생성 시 충돌하므로 extension은 제거하지 않는다.
func resetDB(t *testing.T, pool *pgxpool.Pool, ctx context.Context) {
	t.Helper()
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS runs CASCADE",
		"DROP TABLE IF EXISTS macro_series CASCADE",
		"DROP TABLE IF EXISTS goose_db_version CASCADE",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			t.Fatalf("resetDB %q 실패: %v", stmt, err)
		}
	}
}

func TestUp_AppliesAllMigrations(t *testing.T) {
	skipIfNoIntegration(t)
	pool := newTestPool(t)
	ctx := context.Background()

	resetDB(t, pool, ctx)

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("Up 실패: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_name IN ('macro_series', 'runs')",
	).Scan(&n); err != nil {
		t.Fatalf("테이블 조회 실패: %v", err)
	}
	if n != 2 {
		t.Errorf("macro_series + runs 기대 2개, 실제 %d", n)
	}
}

func TestAssertUpToDate_FreshDB_ReturnsPending(t *testing.T) {
	skipIfNoIntegration(t)
	pool := newTestPool(t)
	ctx := context.Background()

	resetDB(t, pool, ctx)

	err := AssertUpToDate(ctx, pool)
	if !errors.Is(err, ErrMigrationsPending) {
		t.Errorf("ErrMigrationsPending 기대, 실제 %v", err)
	}
}

func TestAssertUpToDate_AfterUp_ReturnsNil(t *testing.T) {
	skipIfNoIntegration(t)
	pool := newTestPool(t)
	ctx := context.Background()

	resetDB(t, pool, ctx)
	if err := Up(ctx, pool); err != nil {
		t.Fatalf("Up 실패: %v", err)
	}

	if err := AssertUpToDate(ctx, pool); err != nil {
		t.Errorf("Up 후 AssertUpToDate 통과 기대, 실제 %v", err)
	}
}
