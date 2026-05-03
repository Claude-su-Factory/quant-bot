# Phase 1b-A — 데이터 인제스트 인프라 + FRED 수집기 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Code task implementer는 반드시 superpowers:test-driven-development 스킬을 호출. 코드 리뷰는 superpowers:requesting-code-review 형식. Steps use checkbox (`- [ ]`).

**Goal:** Phase 1a 인프라 위에 FRED 거시 데이터 4종을 매일 자동 수집하는 stateless CLI + macOS LaunchAgent 시스템 구축. 사용자 첫 셋업은 `make install` 1명령, 매일 운영 부담 0건.

**Architecture:** goose embed로 마이그레이션 → repo 패턴으로 DB I/O → retry helper로 외부 API 안정화 → fred ingester가 client·repo·retry 조립 → cli bootstrap이 DI 컨테이너 → cmd/quantbot이 단일 binary 진입점 → LaunchAgent가 매일 호출.

**Tech Stack:** Go 1.22+, pressly/goose v3.21.0, pgx/v5/pgxpool, BurntSushi/toml, log/slog (Phase 1a). 신규 외부 의존: `github.com/pressly/goose/v3`만.

**Reference Spec:** [`docs/superpowers/specs/2026-05-03-phase1b-a-ingest-infra-fred-design.md`](../specs/2026-05-03-phase1b-a-ingest-infra-fred-design.md)

---

## 작업 가정

- 작업 디렉터리: `/Users/yuhojin/Desktop/quant-bot`
- Phase 1a 완료 (config/logging/db 패키지 + Makefile up/down/db-check/test 동작)
- 태그 `v0.1.0-phase1a` 시작점
- macOS Sonoma+, Docker Desktop, Go 1.22+

---

## File Structure (이 plan으로 생성/수정될 파일)

**생성:**
- `shared/schema/migrations/20260503000001_enable_timescaledb.sql`
- `shared/schema/migrations/20260503000002_create_macro_series.sql`
- `shared/schema/migrations/20260503000003_create_runs.sql`
- `go/internal/migrate/migrate.go` + `migrate_test.go` + `migrate_integration_test.go`
- `go/internal/retry/retry.go` + `retry_test.go`
- `go/internal/repo/macro_series.go` + `macro_series_integration_test.go`
- `go/internal/repo/runs.go` + `runs_integration_test.go`
- `go/internal/ingest/fred/client.go` + `client_test.go`
- `go/internal/ingest/fred/ingester.go` + `ingester_integration_test.go`
- `go/internal/cli/bootstrap.go` + `bootstrap_test.go`
- `go/internal/cli/ingest.go` + `ingest_test.go`
- `go/internal/cli/status.go` + `status_test.go`
- `go/internal/cli/migrate.go`
- `go/internal/cli/version.go`
- `go/cmd/quantbot/main.go`
- `deploy/launchd/com.quantbot.ingest-fred.plist`

**수정:**
- `go/internal/config/config.go` (`[retry]`, `[ingest]` 섹션 + 검증)
- `go/internal/config/config_test.go` (새 검증 테스트)
- `go/internal/config/testdata/valid.toml` (`[retry]`, `[ingest]` 추가)
- `config/config.toml` + `config/config.example.toml` (같은 섹션 추가)
- `docker/docker-compose.yml` (TimescaleDB 버전 핀)
- `go/Makefile` (build target에 -ldflags + -o)
- `Makefile` (루트 — install/uninstall/logs/prepare-migrations/build 추가)
- `.gitignore` (`go/quantbot`, `go/internal/migrate/migrations/`, `logs/launchd-*.log`)
- `go/go.mod`, `go/go.sum` (goose 추가)
- `docs/ARCHITECTURE.md`, `CLAUDE.md` (R14 표 추가)
- `docs/STATUS.md`, `docs/ROADMAP.md` (Phase 1b-A 완료 표시)

---

## Task 1: SQL 마이그레이션 + prepare-migrations + TimescaleDB 버전 핀 (doc/infra task)

**Files:**
- Create: `shared/schema/migrations/{20260503000001_enable_timescaledb,20260503000002_create_macro_series,20260503000003_create_runs}.sql`
- Modify: `Makefile` (root), `docker/docker-compose.yml`, `.gitignore`

- [ ] **Step 1: SQL 마이그레이션 디렉터리 + 3개 파일 작성**

```bash
mkdir -p shared/schema/migrations
```

Create `shared/schema/migrations/20260503000001_enable_timescaledb.sql`:
```sql
-- +goose Up
-- +goose NO TRANSACTION
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- +goose Down
-- +goose NO TRANSACTION
DROP EXTENSION IF EXISTS timescaledb;
```

Create `shared/schema/migrations/20260503000002_create_macro_series.sql`:
```sql
-- +goose Up
CREATE TABLE macro_series (
    series_id   TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    value       NUMERIC(20, 8),
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source      TEXT NOT NULL DEFAULT 'fred',
    PRIMARY KEY (series_id, observed_at)
);
SELECT create_hypertable('macro_series', 'observed_at');

-- +goose Down
DROP TABLE macro_series;
```

Create `shared/schema/migrations/20260503000003_create_runs.sql`:
```sql
-- +goose Up
CREATE TABLE runs (
    id              BIGSERIAL PRIMARY KEY,
    job_name        TEXT NOT NULL,
    instance        TEXT NOT NULL DEFAULT 'paper',
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,
    status          TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
    rows_processed  INTEGER NOT NULL DEFAULT 0,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT
);
CREATE INDEX runs_recent_idx ON runs (started_at DESC);
CREATE INDEX runs_job_status_idx ON runs (job_name, status, started_at DESC);

-- +goose Down
DROP TABLE runs;
```

- [ ] **Step 2: TimescaleDB 이미지 버전 핀**

`docker/docker-compose.yml` 수정. `image:` 라인 변경:

OLD:
```yaml
    image: timescale/timescaledb:latest-pg16
```

NEW:
```yaml
    image: timescale/timescaledb:2.18.0-pg16
```

- [ ] **Step 3: 루트 Makefile에 prepare-migrations target 추가**

루트 `Makefile`의 `.PHONY` 라인 갱신 + 새 target. `.PHONY` OLD:
```
.PHONY: help up down db-check test test-integration fmt lint
```
NEW:
```
.PHONY: help up down db-check test test-integration fmt lint prepare-migrations
```

`lint:` target 다음에 새 target 추가:
```makefile
prepare-migrations:  ## shared/schema/migrations → go/internal/migrate/migrations 동기화 (R9)
	@mkdir -p go/internal/migrate/migrations
	@cp -r shared/schema/migrations/. go/internal/migrate/migrations/
```

- [ ] **Step 4: `.gitignore`에 go/internal/migrate/migrations 추가**

`.gitignore`의 `# Project` 섹션에 추가. OLD:
```
# Project
.env
config/config.toml
shared/artifacts/*
!shared/artifacts/.gitkeep
logs/
```
NEW:
```
# Project
.env
config/config.toml
shared/artifacts/*
!shared/artifacts/.gitkeep
logs/
logs/launchd-*.log
go/quantbot
go/internal/migrate/migrations/
```

- [ ] **Step 5: 검증 — Postgres 기동 + 마이그레이션 SQL 수동 적용 (sanity check)**

```bash
make up && sleep 8
docker compose -f docker/docker-compose.yml exec -T db psql -U quantbot -d quantbot < shared/schema/migrations/20260503000001_enable_timescaledb.sql
docker compose -f docker/docker-compose.yml exec -T db psql -U quantbot -d quantbot < shared/schema/migrations/20260503000002_create_macro_series.sql
docker compose -f docker/docker-compose.yml exec -T db psql -U quantbot -d quantbot < shared/schema/migrations/20260503000003_create_runs.sql
docker compose -f docker/docker-compose.yml exec -T db psql -U quantbot -d quantbot -c "\dt"
```

Expected: `macro_series`, `runs` 테이블 보임. timescaledb extension 활성화.

```bash
docker compose -f docker/docker-compose.yml exec -T db psql -U quantbot -d quantbot -c "DROP TABLE runs; DROP TABLE macro_series; DROP EXTENSION timescaledb;"
make down
```

(검증 후 정리. 실제 마이그레이션은 Task 2의 goose가 적용.)

- [ ] **Step 6: prepare-migrations 동작 확인 + 커밋**

```bash
make prepare-migrations
ls go/internal/migrate/migrations/
```

Expected: 3개 SQL 파일이 복사됨.

```bash
git add shared/schema/migrations/ Makefile docker/docker-compose.yml .gitignore
git commit -m "chore(db): add TimescaleDB migrations + prepare-migrations + version pin

- 3 SQL migrations (timescaledb extension, macro_series hypertable, runs)
- TimescaleDB image pinned to 2.18.0-pg16 (was floating latest-pg16)
- prepare-migrations target syncs shared/schema → go/internal/migrate (R9)
- gitignore: go/quantbot binary, copied migrations, launchd logs"
```

(`go/internal/migrate/migrations/`는 gitignore라 자동 제외됨.)

---

## Task 2: migrate 패키지 (goose embed wrapper) (TDD code task)

**Files:**
- Create: `go/internal/migrate/migrate.go`, `migrate_test.go`, `migrate_integration_test.go`
- Modify: `go/go.mod`, `go/go.sum`

**구현자에게**: TDD 강제. `superpowers:test-driven-development` 스킬 호출 후 작업.

- [ ] **Step 1: goose 의존성 추가**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go
go get github.com/pressly/goose/v3@v3.21.0
go mod tidy
```

- [ ] **Step 2: 미리 prepare-migrations 실행 (embed 위해 SQL 파일이 패키지 디렉터리에 있어야 함)**

```bash
cd /Users/yuhojin/Desktop/quant-bot
make prepare-migrations
```

- [ ] **Step 3: 단위 테스트 먼저 작성 (RED)**

Create `go/internal/migrate/migrate_test.go`:
```go
package migrate

import (
	"testing"
	"testing/fstest"
)

func TestMigrationsFS_HasFiles(t *testing.T) {
	// 임베드된 fs에 마이그레이션 파일이 모두 존재하는지 확인
	expected := []string{
		"migrations/20260503000001_enable_timescaledb.sql",
		"migrations/20260503000002_create_macro_series.sql",
		"migrations/20260503000003_create_runs.sql",
	}
	for _, name := range expected {
		if _, err := MigrationsFS.Open(name); err != nil {
			t.Errorf("임베드된 fs에 %q 없음: %v", name, err)
		}
	}
}

func TestMigrationsFS_OnlySQL(t *testing.T) {
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir 실패: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("마이그레이션 0개")
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("하위 디렉터리는 없어야 함: %s", e.Name())
		}
	}
}

// fstest 패키지로 helper만 사용 (compile-time import 검증)
var _ = fstest.MapFS{}
```

- [ ] **Step 4: 컴파일 실패 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/migrate/... -v
```

Expected: `MigrationsFS` 미정의 컴파일 에러.

- [ ] **Step 5: 최소 구현 (GREEN)**

Create `go/internal/migrate/migrate.go`:
```go
// Package migrate는 goose 기반 SQL 마이그레이션을 관리한다.
// SQL 파일은 build 시 shared/schema/migrations/에서 동기화되어 임베드된다 (R9 단일 진실).
package migrate

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

// ErrMigrationsPending는 적용 안 된 마이그레이션이 있을 때 반환된다.
var ErrMigrationsPending = errors.New("미적용 마이그레이션 발견")

// Up은 모든 미적용 마이그레이션을 적용한다.
func Up(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose SetDialect: %w", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("goose Up: %w", err)
	}
	return nil
}

// Status는 적용/미적용 마이그레이션 목록을 stdout에 출력한다.
func Status(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose SetDialect: %w", err)
	}
	if err := goose.StatusContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("goose Status: %w", err)
	}
	return nil
}

// AssertUpToDate는 미적용 마이그레이션 있으면 ErrMigrationsPending을 반환한다.
// Bootstrap의 fail-fast 검증에 사용 (R12).
func AssertUpToDate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()
	goose.SetBaseFS(MigrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose SetDialect: %w", err)
	}
	current, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		return fmt.Errorf("DB 버전 조회 실패: %w", err)
	}
	migrations, err := goose.CollectMigrations("migrations", 0, goose.MaxVersion)
	if err != nil {
		return fmt.Errorf("마이그레이션 목록 조회 실패: %w", err)
	}
	last := migrations[len(migrations)-1].Version
	if current < last {
		return fmt.Errorf("%w: DB %d, 최신 %d", ErrMigrationsPending, current, last)
	}
	return nil
}
```

- [ ] **Step 6: 단위 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/migrate/... -v
```

Expected: 2개 단위 테스트 PASS (TestMigrationsFS_HasFiles, TestMigrationsFS_OnlySQL).

- [ ] **Step 7: 통합 테스트 추가 (build tag로 격리)**

Create `go/internal/migrate/migrate_integration_test.go`:
```go
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

func TestUp_AppliesAllMigrations(t *testing.T) {
	skipIfNoIntegration(t)
	pool := newTestPool(t)
	ctx := context.Background()

	// 깨끗한 시작: 기존 테이블 삭제
	pool.Exec(ctx, "DROP TABLE IF EXISTS runs, macro_series, goose_db_version CASCADE")
	pool.Exec(ctx, "DROP EXTENSION IF EXISTS timescaledb CASCADE")

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("Up 실패: %v", err)
	}

	// macro_series, runs 테이블 존재 확인
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

	pool.Exec(ctx, "DROP TABLE IF EXISTS runs, macro_series, goose_db_version CASCADE")
	pool.Exec(ctx, "DROP EXTENSION IF EXISTS timescaledb CASCADE")

	err := AssertUpToDate(ctx, pool)
	if !errors.Is(err, ErrMigrationsPending) {
		t.Errorf("ErrMigrationsPending 기대, 실제 %v", err)
	}
}

func TestAssertUpToDate_AfterUp_ReturnsNil(t *testing.T) {
	skipIfNoIntegration(t)
	pool := newTestPool(t)
	ctx := context.Background()

	pool.Exec(ctx, "DROP TABLE IF EXISTS runs, macro_series, goose_db_version CASCADE")
	pool.Exec(ctx, "DROP EXTENSION IF EXISTS timescaledb CASCADE")
	if err := Up(ctx, pool); err != nil {
		t.Fatalf("Up 실패: %v", err)
	}

	if err := AssertUpToDate(ctx, pool); err != nil {
		t.Errorf("Up 후 AssertUpToDate 통과 기대, 실제 %v", err)
	}
}
```

import에 `pgxpool` 추가 필요 — 위 코드에 포함시킴. 정확한 import 라인:
```go
import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
	"github.com/Claude-su-Factory/quant-bot/go/internal/db"
)
```

(편집 시 위 코드에서 `*pgxpool.Pool` 사용하므로 pgxpool import 필수.)

- [ ] **Step 8: 통합 테스트 실행 (Postgres 필요)**

```bash
cd /Users/yuhojin/Desktop/quant-bot && make up && sleep 8
cd go && RUN_INTEGRATION=1 go test -count=1 -tags=integration ./internal/migrate/... -v
cd /Users/yuhojin/Desktop/quant-bot && make down
```

Expected: 3개 통합 테스트 PASS (Up, AssertUpToDate Fresh, AssertUpToDate AfterUp). 단위 테스트 2개도 함께 PASS.

- [ ] **Step 9: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/migrate/ go/go.mod go/go.sum
git commit -m "feat(migrate): goose-based migration wrapper with embed.FS

- MigrationsFS: embed.FS for SQL files (synced from shared/schema/ via prepare-migrations)
- Up/Status/AssertUpToDate functions
- ErrMigrationsPending sentinel for fail-fast Bootstrap (R12)
- Unit tests: embedded FS structure
- Integration tests: actual goose Up/AssertUpToDate cycle"
```

---

## Task 3: config 확장 ([retry], [ingest]) + 검증 (TDD code task)

**Files:**
- Modify: `go/internal/config/config.go`, `config_test.go`, `testdata/valid.toml`, `config/config.toml`, `config/config.example.toml`

**구현자에게**: TDD 사이클 강제.

- [ ] **Step 1: 실패 테스트 추가 (RED)**

`go/internal/config/config_test.go`에 다음 테스트들 추가:
```go
func TestLoad_HasRetryDefaults(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("Load 실패: %v", err)
	}
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("retry.max_attempts: 기대 3, 실제 %d", cfg.Retry.MaxAttempts)
	}
	if cfg.Retry.BackoffInitialMs != 1000 {
		t.Errorf("retry.backoff_initial_ms: 기대 1000, 실제 %d", cfg.Retry.BackoffInitialMs)
	}
	if cfg.Retry.BackoffMultiplier != 2.0 {
		t.Errorf("retry.backoff_multiplier: 기대 2.0, 실제 %f", cfg.Retry.BackoffMultiplier)
	}
}

func TestLoad_HasIngestSection(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("Load 실패: %v", err)
	}
	if cfg.Ingest.BackfillStartDate != "2006-01-01" {
		t.Errorf("ingest.backfill_start_date: 기대 2006-01-01, 실제 %q", cfg.Ingest.BackfillStartDate)
	}
	if len(cfg.Ingest.FREDSeries) != 4 {
		t.Errorf("ingest.fred_series: 기대 4개, 실제 %d", len(cfg.Ingest.FREDSeries))
	}
}

func TestValidate_InvalidRetry(t *testing.T) {
	_, err := Load("testdata/invalid_retry.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (retry 잘못), 실제 %v", err)
	}
}

func TestValidate_InvalidBackfillDate(t *testing.T) {
	_, err := Load("testdata/invalid_backfill_date.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (날짜 형식 X), 실제 %v", err)
	}
}

func TestValidate_EmptyFREDSeries(t *testing.T) {
	_, err := Load("testdata/empty_fred_series.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (빈 시리즈 목록), 실제 %v", err)
	}
}
```

- [ ] **Step 2: testdata fixture 갱신 + 추가**

`go/internal/config/testdata/valid.toml` — 끝에 추가 (기존 섹션 위에 X, 끝에 append):
```toml

[retry]
max_attempts = 3
backoff_initial_ms = 1000
backoff_multiplier = 2.0

[ingest]
backfill_start_date = "2006-01-01"
fred_series = ["T10Y2Y", "VIXCLS", "BAMLH0A0HYM2", "DFF"]
```

Create `go/internal/config/testdata/invalid_retry.toml` — valid.toml 복사 후 다음 변경:
```toml
[retry]
max_attempts = 0     # 0은 ≥1 위반
backoff_initial_ms = 1000
backoff_multiplier = 2.0
```

Create `go/internal/config/testdata/invalid_backfill_date.toml` — valid.toml 복사 후:
```toml
[ingest]
backfill_start_date = "not a date"
fred_series = ["T10Y2Y"]
```

Create `go/internal/config/testdata/empty_fred_series.toml` — valid.toml 복사 후:
```toml
[ingest]
backfill_start_date = "2006-01-01"
fred_series = []
```

- [ ] **Step 3: 테스트 실행해 실패 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/config/... -v
```

Expected: 새 테스트 5개 모두 컴파일 에러 (`Retry`, `Ingest` 필드 미정의).

- [ ] **Step 4: Config 구조체 확장 + 검증 추가 (GREEN)**

`go/internal/config/config.go`의 `Config` 구조체와 그 아래 타입들에 추가:

OLD:
```go
type Config struct {
	General  GeneralConfig  `toml:"general"`
	Database DatabaseConfig `toml:"database"`
	Alpaca   AlpacaConfig   `toml:"alpaca"`
	FRED     FREDConfig     `toml:"fred"`
	Logging  LoggingConfig  `toml:"logging"`
}
```

NEW:
```go
type Config struct {
	General  GeneralConfig  `toml:"general"`
	Database DatabaseConfig `toml:"database"`
	Alpaca   AlpacaConfig   `toml:"alpaca"`
	FRED     FREDConfig     `toml:"fred"`
	Logging  LoggingConfig  `toml:"logging"`
	Retry    RetryConfig    `toml:"retry"`
	Ingest   IngestConfig   `toml:"ingest"`
}
```

`LoggingConfig` 타입 정의 다음에 두 새 타입 추가:
```go
type RetryConfig struct {
	MaxAttempts       int     `toml:"max_attempts"`
	BackoffInitialMs  int     `toml:"backoff_initial_ms"`
	BackoffMultiplier float64 `toml:"backoff_multiplier"`
}

type IngestConfig struct {
	BackfillStartDate string   `toml:"backfill_start_date"`  // ISO 8601 (R13)
	FREDSeries        []string `toml:"fred_series"`
}
```

`validate` 함수 끝(return nil 직전)에 추가:
```go
	// retry
	if cfg.Retry.MaxAttempts < 1 {
		return fmt.Errorf("retry.max_attempts: 1 이상이어야 함: %d", cfg.Retry.MaxAttempts)
	}
	if cfg.Retry.BackoffInitialMs < 1 {
		return fmt.Errorf("retry.backoff_initial_ms: 1 이상이어야 함: %d", cfg.Retry.BackoffInitialMs)
	}
	if cfg.Retry.BackoffMultiplier < 1.0 {
		return fmt.Errorf("retry.backoff_multiplier: 1.0 이상이어야 함: %f", cfg.Retry.BackoffMultiplier)
	}

	// ingest
	if _, err := time.Parse("2006-01-02", cfg.Ingest.BackfillStartDate); err != nil {
		return fmt.Errorf("ingest.backfill_start_date: ISO 8601(YYYY-MM-DD) 형식 X: %q", cfg.Ingest.BackfillStartDate)
	}
	if len(cfg.Ingest.FREDSeries) == 0 {
		return fmt.Errorf("ingest.fred_series: 비어있을 수 없음")
	}
```

import에 `"time"` 추가.

- [ ] **Step 5: 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/config/... -v
```

Expected: 12개 테스트 모두 PASS (기존 7개 + 신규 5개).

- [ ] **Step 6: 실제 config 파일에 섹션 추가**

`config/config.example.toml` 끝에 추가:
```toml

[retry]
max_attempts = 3
backoff_initial_ms = 1000
backoff_multiplier = 2.0

[ingest]
backfill_start_date = "2006-01-01"
fred_series = ["T10Y2Y", "VIXCLS", "BAMLH0A0HYM2", "DFF"]
```

`config/config.toml` 끝에도 똑같이 추가 (실제 사용 값 = example과 동일).

- [ ] **Step 7: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/config/ config/
git commit -m "feat(config): add [retry] and [ingest] sections with validation

- RetryConfig: max_attempts, backoff_initial_ms, backoff_multiplier
- IngestConfig: backfill_start_date (ISO 8601), fred_series ([]string)
- validate(): 4 new rules (retry counts, ISO date parse, non-empty fred_series)
- 5 new tests + 3 new fixture TOMLs
- config.toml + config.example.toml synced (R11)"
```

---

## Task 4: retry 패키지 (TDD code task)

**Files:**
- Create: `go/internal/retry/retry.go`, `retry_test.go`

**구현자에게**: TDD 강제. `superpowers:test-driven-development` 호출 후 작업.

- [ ] **Step 1: 디렉터리 생성 + 첫 실패 테스트 (RED)**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/go/internal/retry
```

Create `go/internal/retry/retry_test.go`:
```go
package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

var defaultCfg = Config{
	MaxAttempts:       3,
	BackoffInitialMs:  10,   // 테스트 빠르게 — 10ms
	BackoffMultiplier: 2.0,
}

func TestDo_FirstTrySuccess(t *testing.T) {
	calls := 0
	retries, err := Do(context.Background(), defaultCfg, nil, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("기대 nil, 실제 %v", err)
	}
	if calls != 1 {
		t.Errorf("calls 기대 1, 실제 %d", calls)
	}
	if retries != 0 {
		t.Errorf("retries 기대 0, 실제 %d", retries)
	}
}

func TestDo_RetryThenSuccess(t *testing.T) {
	calls := 0
	retries, err := Do(context.Background(), defaultCfg, nil, func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Errorf("기대 nil, 실제 %v", err)
	}
	if calls != 3 {
		t.Errorf("calls 기대 3, 실제 %d", calls)
	}
	if retries != 2 {
		t.Errorf("retries 기대 2, 실제 %d", retries)
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	want := errors.New("permanent")
	calls := 0
	retries, err := Do(context.Background(), defaultCfg, nil, func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("마지막 에러 기대 %v, 실제 %v", want, err)
	}
	if calls != 3 {
		t.Errorf("calls 기대 3, 실제 %d", calls)
	}
	if retries != 2 {
		t.Errorf("retries 기대 2, 실제 %d", retries)
	}
}

func TestDo_NonRetryableErrorStopsImmediately(t *testing.T) {
	want := errors.New("non-retryable")
	calls := 0
	isRetryable := func(err error) bool {
		return false  // 모두 비재시도
	}
	retries, err := Do(context.Background(), defaultCfg, isRetryable, func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("에러 기대 %v, 실제 %v", want, err)
	}
	if calls != 1 {
		t.Errorf("calls 기대 1, 실제 %d", calls)
	}
	if retries != 0 {
		t.Errorf("retries 기대 0, 실제 %d", retries)
	}
}

func TestDo_ContextCancelStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()  // 즉시 취소

	calls := 0
	_, err := Do(ctx, defaultCfg, nil, func() error {
		calls++
		return errors.New("would retry")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("context.Canceled 기대, 실제 %v", err)
	}
	if calls > 1 {
		t.Errorf("최대 1회 호출 기대, 실제 %d", calls)
	}
}

func TestDo_BackoffIncreases(t *testing.T) {
	// 백오프가 증가하는지 — 시간 측정으로 정성적 검증
	cfg := Config{
		MaxAttempts:       3,
		BackoffInitialMs:  50,
		BackoffMultiplier: 2.0,
	}
	start := time.Now()
	Do(context.Background(), cfg, nil, func() error {
		return errors.New("always fail")
	})
	elapsed := time.Since(start)
	// 최소 50ms (1차 실패→대기) + 100ms (2차 실패→대기) = 150ms
	// 3차도 실패하지만 대기 후 추가 호출 X
	if elapsed < 130*time.Millisecond {
		t.Errorf("백오프 너무 짧음: %v", elapsed)
	}
}
```

- [ ] **Step 2: 컴파일 실패 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/retry/... -v
```

Expected: `Do`, `Config`, `IsRetryable` 미정의.

- [ ] **Step 3: 최소 구현 (GREEN)**

Create `go/internal/retry/retry.go`:
```go
// Package retry는 외부 호출 (HTTP API 등) 재시도 helper를 제공한다.
// exponential backoff + ctx 취소 + 사용자 정의 IsRetryable.
package retry

import (
	"context"
	"time"
)

// Config는 재시도 정책. config.RetryConfig에서 옴.
type Config struct {
	MaxAttempts       int
	BackoffInitialMs  int
	BackoffMultiplier float64
}

// IsRetryable는 op이 반환한 에러가 재시도 대상인지 판단.
// nil이면 모든 에러 재시도 (단순).
type IsRetryable func(err error) bool

// Do는 op를 재시도와 함께 실행.
// 마지막 시도까지 실패 시 마지막 에러 반환. ctx 취소 즉시 ctx.Err() 반환.
func Do(ctx context.Context, cfg Config, isRetryable IsRetryable, op func() error) (retries int, err error) {
	delay := time.Duration(cfg.BackoffInitialMs) * time.Millisecond
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		if err = ctx.Err(); err != nil {
			return retries, err
		}
		err = op()
		if err == nil {
			return retries, nil
		}
		// non-retryable이면 즉시 종료
		if isRetryable != nil && !isRetryable(err) {
			return retries, err
		}
		// 마지막 시도였으면 더 안 잠
		if attempt == cfg.MaxAttempts {
			return retries, err
		}
		// 백오프 대기 (ctx 취소 존중)
		select {
		case <-ctx.Done():
			return retries, ctx.Err()
		case <-time.After(delay):
		}
		retries++
		delay = time.Duration(float64(delay) * cfg.BackoffMultiplier)
	}
	return retries, err
}
```

- [ ] **Step 4: 6개 테스트 모두 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test -count=1 ./internal/retry/... -v
```

Expected: 6개 PASS.

```bash
go vet ./internal/retry/...
```

Expected: 0 issues.

- [ ] **Step 5: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/retry/
git commit -m "feat(retry): exponential backoff helper with ctx cancellation + IsRetryable

- Do(ctx, cfg, isRetryable, op) returns (retries, err)
- 6 unit tests: first-try success, retry-then-success, all-fail,
  non-retryable stops immediately, ctx cancel stops, backoff increases"
```

---

## Task 5: repo 패키지 (macro_series + runs CRUD) (TDD code task, 통합 테스트 위주)

**Files:**
- Create: `go/internal/repo/macro_series.go`, `macro_series_integration_test.go`, `runs.go`, `runs_integration_test.go`

**구현자에게**: TDD. DB 테이블 의존이라 단위 테스트 어려움 → 통합 테스트 중심. mock 시도 X (pgxpool 통째 mock 어려움 — Phase 1a Task 4의 결정).

- [ ] **Step 1: 디렉터리 + macro_series 통합 테스트 (RED)**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/go/internal/repo
```

Create `go/internal/repo/macro_series_integration_test.go`:
```go
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
	pool.Exec(bg, "DROP TABLE IF EXISTS runs, macro_series, goose_db_version CASCADE")
	pool.Exec(bg, "DROP EXTENSION IF EXISTS timescaledb CASCADE")
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
		{SeriesID: "T10Y2Y", ObservedAt: mustDate("2026-04-03"), Value: nil}, // 휴장일
	}
	n, err := InsertObservations(ctx, pool, obs)
	if err != nil {
		t.Fatalf("InsertObservations 실패: %v", err)
	}
	if n != 3 {
		t.Errorf("inserted 기대 3, 실제 %d", n)
	}

	// SELECT으로 확인
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

	// 같은 데이터 다시 insert
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
```

- [ ] **Step 2: 컴파일 실패 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test -tags=integration ./internal/repo/... -v
```

Expected: `Observation`, `InsertObservations`, `LastObservedAt` 미정의.

- [ ] **Step 3: 최소 구현 (GREEN)**

Create `go/internal/repo/macro_series.go`:
```go
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

	rows := make([][]any, len(obs))
	for i, o := range obs {
		rows[i] = []any{o.SeriesID, o.ObservedAt, o.Value}
	}
	// COPY는 ON CONFLICT 미지원. INSERT 배치로 진행.
	// 100행 단위 묶기 (Postgres 파라미터 한도 65535 / 컬럼당 3 = 21845 가능, 100이면 안전)
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
	// $1,$2,$3 / $4,$5,$6 / ... 형태 placeholder 동적 생성
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
```

- [ ] **Step 4: 통합 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot && make up && sleep 8
cd go && RUN_INTEGRATION=1 go test -count=1 -tags=integration ./internal/repo/... -v
```

Expected: 4개 통합 테스트 PASS.

- [ ] **Step 5: runs 테이블 통합 테스트 (RED)**

Create `go/internal/repo/runs_integration_test.go`:
```go
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
		time.Sleep(5 * time.Millisecond) // 시간 분리
	}

	runs, err := RecentRuns(ctx, pool, 10)
	if err != nil {
		t.Fatalf("RecentRuns 실패: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("3개 기대, 실제 %d", len(runs))
	}
	// 최신부터 (rows=2가 첫 번째)
	if runs[0].RowsProcessed != 2 {
		t.Errorf("최신 우선 정렬 X, 첫 row=%d", runs[0].RowsProcessed)
	}
}

func TestStaleRuns_FindsAbnormallyOpen(t *testing.T) {
	pool := setupDB(t)
	ctx := context.Background()

	// 일부러 finished_at NULL + 오래된 row 삽입
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
```

- [ ] **Step 6: runs.go 구현**

Create `go/internal/repo/runs.go`:
```go
package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run은 runs 테이블의 한 행.
type Run struct {
	ID             int64
	JobName        string
	Instance       string
	StartedAt      time.Time
	FinishedAt     *time.Time
	Status         string
	RowsProcessed  int
	RetryCount     int
	ErrorMessage   string
}

// RunResult는 FinishRun에 넘길 결과 정보.
type RunResult struct {
	Status        string // "success" / "failed"
	RowsProcessed int
	RetryCount    int
	Error         error  // failed일 때만 사용
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
	_, err := pool.Exec(ctx,
		`UPDATE runs SET finished_at = NOW(), status = $1, rows_processed = $2,
		                  retry_count = $3, error_message = $4
		 WHERE id = $5`,
		r.Status, r.RowsProcessed, r.RetryCount, errMsg, id,
	)
	if err != nil {
		return fmt.Errorf("FinishRun UPDATE: %w", err)
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
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
```

- [ ] **Step 7: 통합 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && RUN_INTEGRATION=1 go test -count=1 -tags=integration ./internal/repo/... -v
```

Expected: 8개 통합 테스트 모두 PASS (macro_series 4 + runs 4).

```bash
cd /Users/yuhojin/Desktop/quant-bot && make down
```

- [ ] **Step 8: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/repo/
git commit -m "feat(repo): macro_series + runs CRUD with idempotent INSERT

macro_series.go:
- Observation type
- InsertObservations: batched ON CONFLICT DO NOTHING, returns rows actually inserted
- LastObservedAt: MAX query, zero time on empty table

runs.go:
- Run + RunResult types
- StartRun: INSERT + RETURNING id
- FinishRun: UPDATE status/finished_at/stats
- RecentRuns: SELECT ORDER BY started_at DESC LIMIT N
- StaleRuns: finished_at IS NULL AND started_at < NOW - 1h (정상 진단용)

8 integration tests (RUN_INTEGRATION=1, build tag integration)"
```

---

## Task 6: ingest/fred 패키지 (HTTP client + ingester) (TDD code task)

**Files:**
- Create: `go/internal/ingest/fred/client.go`, `client_test.go`, `ingester.go`, `ingester_integration_test.go`

**구현자에게**: TDD. client는 httptest로 단위 테스트, ingester는 실제 DB로 통합 테스트.

- [ ] **Step 1: 디렉터리 + client_test.go (RED)**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/go/internal/ingest/fred
```

Create `go/internal/ingest/fred/client_test.go`:
```go
package fred

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSeries_ParsesObservations(t *testing.T) {
	// FRED API 응답 모의
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"observations": [
				{"date": "2026-04-01", "value": "0.25"},
				{"date": "2026-04-02", "value": "."},
				{"date": "2026-04-03", "value": "0.30"}
			]
		}`))
	}))
	defer server.Close()

	client := New(server.URL, "test_api_key")
	obs, err := client.FetchSeries(context.Background(), "T10Y2Y", mustDate("2026-04-01"), mustDate("2026-04-03"))
	if err != nil {
		t.Fatalf("FetchSeries 실패: %v", err)
	}
	if len(obs) != 3 {
		t.Fatalf("3개 기대, 실제 %d", len(obs))
	}
	if obs[0].Date != mustDate("2026-04-01") {
		t.Errorf("date 파싱 실패: %v", obs[0].Date)
	}
	if obs[0].Value == nil || *obs[0].Value != 0.25 {
		t.Errorf("value 파싱 실패: %v", obs[0].Value)
	}
	if obs[1].Value != nil {
		t.Errorf("'.' value는 nil 기대, 실제 %v", *obs[1].Value)
	}
}

func TestFetchSeries_5xxReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", 503)
	}))
	defer server.Close()

	client := New(server.URL, "test")
	_, err := client.FetchSeries(context.Background(), "T10Y2Y", mustDate("2026-04-01"), mustDate("2026-04-01"))
	if err == nil {
		t.Fatalf("503에 에러 기대")
	}
	var hErr *HTTPError
	if !errors.As(err, &hErr) {
		t.Fatalf("HTTPError 기대, 실제 %T", err)
	}
	if hErr.StatusCode != 503 {
		t.Errorf("StatusCode 기대 503, 실제 %d", hErr.StatusCode)
	}
}

func TestIsRetryable_4xxNotRetryable(t *testing.T) {
	err := &HTTPError{StatusCode: 400, Body: "bad request"}
	if IsRetryable(err) {
		t.Errorf("4xx는 비재시도 기대")
	}
}

func TestIsRetryable_5xxRetryable(t *testing.T) {
	err := &HTTPError{StatusCode: 503, Body: "x"}
	if !IsRetryable(err) {
		t.Errorf("5xx는 재시도 기대")
	}
}

func TestIsRetryable_429Retryable(t *testing.T) {
	err := &HTTPError{StatusCode: 429, Body: "rate limit"}
	if !IsRetryable(err) {
		t.Errorf("429는 재시도 기대")
	}
}

func TestIsRetryable_NetworkErrorRetryable(t *testing.T) {
	err := errors.New("connection reset")
	if !IsRetryable(err) {
		t.Errorf("일반 에러는 재시도 기대 (네트워크 가정)")
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}
```

- [ ] **Step 2: 컴파일 실패 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/ingest/fred/... -v
```

Expected: `New`, `Client`, `FetchSeries`, `HTTPError`, `IsRetryable`, `Observation` 미정의.

- [ ] **Step 3: client.go 구현 (GREEN)**

Create `go/internal/ingest/fred/client.go`:
```go
// Package fred는 FRED API 클라이언트 + IsRetryable 정책.
package fred

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Observation은 FRED API의 한 관측 (date + value).
// Value는 NULL 가능 (FRED가 휴장일 등에 "." 반환).
type Observation struct {
	Date  time.Time
	Value *float64
}

// HTTPError는 FRED API에서 4xx/5xx 받았을 때 반환된다.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("FRED HTTP %d: %s", e.StatusCode, e.Body)
}

// Client는 FRED API 호출자.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New는 baseURL(예: "https://api.stlouisfed.org/fred")과 API key로 클라이언트 생성.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchSeries는 지정한 기간의 시리즈 관측을 가져온다.
func (c *Client) FetchSeries(ctx context.Context, seriesID string, start, end time.Time) ([]Observation, error) {
	q := url.Values{}
	q.Set("series_id", seriesID)
	q.Set("api_key", c.apiKey)
	q.Set("file_type", "json")
	q.Set("observation_start", start.Format("2006-01-02"))
	q.Set("observation_end", end.Format("2006-01-02"))

	endpoint := c.baseURL + "/series/observations?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("FRED req 생성: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FRED HTTP: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var raw struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("FRED JSON 파싱: %w", err)
	}

	out := make([]Observation, 0, len(raw.Observations))
	for _, o := range raw.Observations {
		date, err := time.Parse("2006-01-02", o.Date)
		if err != nil {
			return nil, fmt.Errorf("FRED date 파싱 %q: %w", o.Date, err)
		}
		var v *float64
		if o.Value != "." && o.Value != "" {
			f, err := strconv.ParseFloat(o.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("FRED value 파싱 %q: %w", o.Value, err)
			}
			v = &f
		}
		out = append(out, Observation{Date: date.UTC(), Value: v})
	}
	return out, nil
}

// IsRetryable는 FRED API 결과가 재시도 대상인지 판단.
// HTTPError 4xx → false, 5xx/429 → true, 나머지 → true (네트워크 에러 가정).
func IsRetryable(err error) bool {
	var hErr *HTTPError
	if errAs(err, &hErr) {
		if hErr.StatusCode >= 400 && hErr.StatusCode < 500 {
			// 단, 429는 rate limit이라 재시도
			if hErr.StatusCode == 429 {
				return true
			}
			return false
		}
		// 5xx 재시도
		return true
	}
	// 네트워크 에러 등 → 재시도
	return true
}

// errAs는 errors.As 래퍼 (import 단순화).
func errAs(err error, target any) bool {
	type asInterface interface {
		As(any) bool
	}
	// stdlib errors.As 사용 — 별도 import 필요. 위에 errors import 추가.
	return errorsAs(err, target)
}
```

위 코드의 `errorsAs` 부분이 어색함. 단순화 — `errors` 패키지 import 후 `errors.As` 직접 사용:

`client.go`의 import 블록 갱신:
```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)
```

`IsRetryable` 함수의 마지막 부분 교체 — `errAs`/`errorsAs` 제거하고 `errors.As` 직접 사용:
```go
func IsRetryable(err error) bool {
	var hErr *HTTPError
	if errors.As(err, &hErr) {
		if hErr.StatusCode == 429 {
			return true
		}
		if hErr.StatusCode >= 400 && hErr.StatusCode < 500 {
			return false
		}
		return true
	}
	return true
}
```

`errAs`, `errorsAs` 함수 삭제.

- [ ] **Step 4: client 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test -count=1 ./internal/ingest/fred/... -v
```

Expected: 6개 PASS.

- [ ] **Step 5: ingester_integration_test.go 작성 (RED)**

Create `go/internal/ingest/fred/ingester_integration_test.go`:
```go
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
	pool.Exec(bg, "DROP TABLE IF EXISTS runs, macro_series, goose_db_version CASCADE")
	pool.Exec(bg, "DROP EXTENSION IF EXISTS timescaledb CASCADE")
	migrate.Up(bg, pool)
	return pool
}

func TestRun_IngestsFromMockFRED(t *testing.T) {
	pool := freshDB(t)

	// 모의 FRED 서버 — 두 시리즈에 대해 다른 응답
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
		Series:              []string{"T10Y2Y", "VIXCLS"},
		BackfillStartDate:   mustDate("2026-04-01"),
		Retry:               retryCfg(),
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

	// 첫 실행
	r1, _ := ing.Run(ctx)
	if r1.RowsProcessed != 1 {
		t.Errorf("첫 실행 rows 기대 1, 실제 %d", r1.RowsProcessed)
	}
	// 두 번째 실행 — 같은 데이터 → ON CONFLICT로 0 rows
	r2, _ := ing.Run(ctx)
	if r2.RowsProcessed != 0 {
		t.Errorf("idempotent 두번째 rows 기대 0, 실제 %d", r2.RowsProcessed)
	}
}

func retryCfg() retryConfig {
	return retryConfig{
		MaxAttempts:       3,
		BackoffInitialMs:  10,
		BackoffMultiplier: 2.0,
	}
}
```

- [ ] **Step 6: ingester.go 구현 (GREEN)**

Create `go/internal/ingest/fred/ingester.go`:
```go
package fred

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Claude-su-Factory/quant-bot/go/internal/repo"
	"github.com/Claude-su-Factory/quant-bot/go/internal/retry"
)

// Config는 ingester 동작 설정. config.IngestConfig + config.RetryConfig를 받아 초기화.
type Config struct {
	Series              []string
	BackfillStartDate   time.Time
	Retry               retryConfig
}

type retryConfig = retry.Config

// Result는 Run의 결과 통계.
type Result struct {
	RowsProcessed int
	RetryCount    int
}

// Ingester는 FRED 시리즈를 DB에 수집하는 작업.
type Ingester struct {
	client   *Client
	pool     *pgxpool.Pool
	cfg      Config
	instance string
}

// NewIngester는 의존성 주입으로 Ingester 생성.
func NewIngester(client *Client, pool *pgxpool.Pool, cfg Config, instance string) *Ingester {
	return &Ingester{client: client, pool: pool, cfg: cfg, instance: instance}
}

// Run은 모든 시리즈를 수집한다 (증분 + 첫 실행 시 백필).
// 실패 시 마지막 에러 반환. 부분 성공 시 누적 통계 반환.
func (i *Ingester) Run(ctx context.Context) (Result, error) {
	var res Result
	for _, seriesID := range i.cfg.Series {
		last, err := repo.LastObservedAt(ctx, i.pool, seriesID)
		if err != nil {
			return res, fmt.Errorf("LastObservedAt %s: %w", seriesID, err)
		}
		var start time.Time
		if last.IsZero() {
			start = i.cfg.BackfillStartDate
		} else {
			start = last.Add(24 * time.Hour) // 다음날부터
		}
		end := time.Now().UTC()
		if !start.Before(end) {
			continue // 이미 최신
		}

		var obs []Observation
		retries, err := retry.Do(ctx, i.cfg.Retry, IsRetryable, func() error {
			var inner error
			obs, inner = i.client.FetchSeries(ctx, seriesID, start, end)
			return inner
		})
		res.RetryCount += retries
		if err != nil {
			return res, fmt.Errorf("FetchSeries %s: %w", seriesID, err)
		}

		// FRED Observation → repo Observation 변환
		repoObs := make([]repo.Observation, len(obs))
		for k, o := range obs {
			repoObs[k] = repo.Observation{
				SeriesID:   seriesID,
				ObservedAt: o.Date,
				Value:      o.Value,
			}
		}
		n, err := repo.InsertObservations(ctx, i.pool, repoObs)
		if err != nil {
			return res, fmt.Errorf("InsertObservations %s: %w", seriesID, err)
		}
		res.RowsProcessed += n
	}
	return res, nil
}
```

- [ ] **Step 7: 통합 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot && make up && sleep 8
cd go && RUN_INTEGRATION=1 go test -count=1 -tags=integration ./internal/ingest/fred/... -v
cd /Users/yuhojin/Desktop/quant-bot && make down
```

Expected: 6 client unit + 2 ingester integration = 8개 PASS.

- [ ] **Step 8: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/ingest/
git commit -m "feat(ingest/fred): HTTP client + ingester for FRED macro data

client.go:
- Client.FetchSeries(ctx, seriesID, start, end) → []Observation
- HTTPError type with StatusCode, Body
- IsRetryable: 4xx (except 429) non-retryable, 5xx + network retryable
- 6 unit tests with httptest mock FRED server

ingester.go:
- Ingester.Run: per-series incremental fetch + ON CONFLICT INSERT
- Uses retry.Do with IsRetryable, repo.InsertObservations
- 2 integration tests (mock FRED server + real DB)"
```

---

## Task 7: cli/bootstrap + main router + version (TDD code task)

**Files:**
- Create: `go/internal/cli/bootstrap.go`, `bootstrap_test.go`, `version.go`, `go/cmd/quantbot/main.go`

**구현자에게**: TDD. Bootstrap의 단위 테스트는 의존성 주입으로 가능 (config 로더 분리).

- [ ] **Step 1: 디렉터리**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/go/internal/cli /Users/yuhojin/Desktop/quant-bot/go/cmd/quantbot
```

- [ ] **Step 2: bootstrap_test.go 먼저 (RED)**

Create `go/internal/cli/bootstrap_test.go`:
```go
package cli

import (
	"context"
	"errors"
	"testing"
)

func TestBootstrap_InvalidConfigPath(t *testing.T) {
	_, err := Bootstrap(context.Background(), "testdata/does_not_exist.toml", false)
	if err == nil {
		t.Fatalf("존재하지 않는 path에 에러 기대")
	}
	// config.ErrConfigMissing이 wrap되어 있어야 함
	if !errors.Is(err, ErrBootstrap) {
		t.Errorf("ErrBootstrap 기대, 실제 %v", err)
	}
}
```

(Bootstrap의 다른 경로는 DB 의존이라 통합 테스트 영역 — 본 단위 테스트는 fail-fast 진입 검증만.)

- [ ] **Step 3: 컴파일 실패 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test ./internal/cli/... -v
```

Expected: `Bootstrap`, `ErrBootstrap` 미정의.

- [ ] **Step 4: bootstrap.go 구현 (GREEN)**

Create `go/internal/cli/bootstrap.go`:
```go
// Package cli는 quantbot CLI subcommand 구현.
// Bootstrap은 모든 명령이 공유하는 셋업 (config + logger + DB pool + 마이그레이션 검증).
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
	"github.com/Claude-su-Factory/quant-bot/go/internal/db"
	"github.com/Claude-su-Factory/quant-bot/go/internal/logging"
	"github.com/Claude-su-Factory/quant-bot/go/internal/migrate"
)

// ErrBootstrap는 Bootstrap의 어떤 단계든 실패 시 wrap된다.
var ErrBootstrap = errors.New("bootstrap 실패")

// App은 Bootstrap의 결과 — 모든 subcommand가 받음.
type App struct {
	Cfg    *config.Config
	Logger *logging.Logger
	Pool   *pgxpool.Pool
	Close  func() error
}

// Bootstrap은 fail-fast 시퀀스로 앱 컨텍스트를 만든다 (R12).
// requireMigrated가 true면 미적용 마이그레이션 감지 시 ErrMigrationsPending 반환.
func Bootstrap(ctx context.Context, configPath string, requireMigrated bool) (*App, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("%w: config: %v", ErrBootstrap, err)
	}

	logger, closeLogger, err := logging.New(cfg.Logging.FileDir, cfg.General.LogLevel,
		cfg.General.Environment, cfg.Logging.IncludeCaller)
	if err != nil {
		return nil, fmt.Errorf("%w: logger: %v", ErrBootstrap, err)
	}

	pool, err := db.NewPool(ctx, cfg.Database)
	if err != nil {
		closeLogger()
		return nil, fmt.Errorf("%w: db: %v", ErrBootstrap, err)
	}

	if requireMigrated {
		if err := migrate.AssertUpToDate(ctx, pool); err != nil {
			pool.Close()
			closeLogger()
			return nil, fmt.Errorf("%w: %v\n실행: quantbot migrate up (또는 make install)", ErrBootstrap, err)
		}
	}

	return &App{
		Cfg:    cfg,
		Logger: logger,
		Pool:   pool,
		Close: func() error {
			pool.Close()
			return closeLogger()
		},
	}, nil
}
```

- [ ] **Step 5: 단위 테스트 통과 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go test -count=1 ./internal/cli/... -v
```

Expected: TestBootstrap_InvalidConfigPath PASS.

- [ ] **Step 6: version.go 작성**

Create `go/internal/cli/version.go`:
```go
package cli

import "fmt"

// Version은 main에서 ldflags로 주입된다 (`-X main.Version=...`).
// main.go가 RunVersion 호출 시 자체 변수를 인자로 넘김.
func RunVersion(version string) {
	fmt.Printf("quantbot %s\n", version)
}
```

- [ ] **Step 7: main.go 작성**

Create `go/cmd/quantbot/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/Claude-su-Factory/quant-bot/go/internal/cli"
)

// Version은 -ldflags="-X main.Version=$(git describe ...)"로 빌드 타임 주입.
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "ingest":
		cli.RunIngest(os.Args[2:])
	case "status":
		cli.RunStatus(os.Args[2:])
	case "migrate":
		cli.RunMigrate(os.Args[2:])
	case "version", "--version", "-v":
		cli.RunVersion(Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 명령: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println(`quantbot — 미국 주식 스윙 트레이딩 봇

사용법:
  quantbot <command> [args]

명령:
  ingest fred       FRED 거시 데이터 수집
  status            최근 운영 상태 표시
  migrate up        DB 마이그레이션 적용
  migrate status    마이그레이션 적용 상태
  version           버전 정보
  help              본 도움말`)
}
```

- [ ] **Step 8: cli.RunIngest, RunStatus, RunMigrate stub (Task 8에서 채움)**

Task 8을 먼저 시작하면 main.go가 컴파일 안 됨. 임시 stub으로 build 가능하게:

Create `go/internal/cli/ingest.go` (stub — Task 8에서 진짜 구현):
```go
package cli

import "fmt"

func RunIngest(args []string) {
	fmt.Println("Task 8에서 구현됨")
}
```

Create `go/internal/cli/status.go` (stub):
```go
package cli

import "fmt"

func RunStatus(args []string) {
	fmt.Println("Task 8에서 구현됨")
}
```

Create `go/internal/cli/migrate.go` (stub):
```go
package cli

import "fmt"

func RunMigrate(args []string) {
	fmt.Println("Task 8에서 구현됨")
}
```

- [ ] **Step 9: 빌드 + version 동작 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go build -o quantbot -ldflags="-X main.Version=v0.1.0-dev" ./cmd/quantbot
./quantbot version
./quantbot help
./quantbot bogus 2>&1 | head -3
echo "exit: $?"
rm quantbot
```

Expected:
- `quantbot v0.1.0-dev`
- 도움말 출력
- "알 수 없는 명령" + exit 2

- [ ] **Step 10: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/cmd/ go/internal/cli/
git commit -m "feat(cli): bootstrap + main router + version + stubs

- Bootstrap: fail-fast (config → logger → pool → migrate check) (R12)
- ErrBootstrap sentinel for unified error path
- App.Close: pool.Close() + closeLogger() in one call
- main.go: subcommand router with help/version
- ldflags Version injection
- RunIngest/RunStatus/RunMigrate stubs (Task 8 채움)
- 1 unit test (invalid config path → ErrBootstrap)"
```

---

## Task 8: cli ingest + migrate + status subcommand (TDD code task)

**Files:**
- Modify: `go/internal/cli/ingest.go`, `migrate.go`, `status.go`
- Create: `go/internal/cli/ingest_test.go`, `status_test.go`

**구현자에게**: TDD. CLI 명령 자체는 통합 테스트 위주. 함수 단위로 시그니처·flag 파싱만 단위 테스트.

- [ ] **Step 1: cli/migrate.go 진짜 구현**

`go/internal/cli/migrate.go` 전체 교체:
```go
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Claude-su-Factory/quant-bot/go/internal/migrate"
)

func RunMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	configPath := fs.String("config", "config/config.toml", "config 파일 경로")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "사용법: quantbot migrate <up|status>")
		os.Exit(2)
	}

	ctx := context.Background()
	app, err := Bootstrap(ctx, *configPath, false) // requireMigrated=false (이게 마이그레이션 자체임)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	defer app.Close()

	switch fs.Arg(0) {
	case "up":
		if err := migrate.Up(ctx, app.Pool); err != nil {
			fmt.Fprintln(os.Stderr, "마이그레이션 실패:", err)
			os.Exit(1)
		}
		fmt.Println("✅ 마이그레이션 적용 완료")
	case "status":
		if err := migrate.Status(ctx, app.Pool); err != nil {
			fmt.Fprintln(os.Stderr, "status 실패:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 migrate 하위 명령: %s\n", fs.Arg(0))
		os.Exit(2)
	}
}
```

- [ ] **Step 2: cli/ingest.go 진짜 구현**

`go/internal/cli/ingest.go` 전체 교체:
```go
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Claude-su-Factory/quant-bot/go/internal/ingest/fred"
	"github.com/Claude-su-Factory/quant-bot/go/internal/repo"
)

const fredDefaultBaseURL = "https://api.stlouisfed.org/fred"

func RunIngest(args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	configPath := fs.String("config", "config/config.toml", "config 파일 경로")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "사용법: quantbot ingest <fred>")
		os.Exit(2)
	}

	ctx := context.Background()
	app, err := Bootstrap(ctx, *configPath, true) // requireMigrated=true
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	defer app.Close()

	switch fs.Arg(0) {
	case "fred":
		runFRED(ctx, app)
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 ingest source: %s\n", fs.Arg(0))
		os.Exit(2)
	}
}

func runFRED(ctx context.Context, app *App) {
	jobName := "ingest_fred"
	runID, err := repo.StartRun(ctx, app.Pool, jobName, app.Cfg.General.Environment)
	if err != nil {
		app.Logger.Error("StartRun 실패", "err", err)
		os.Exit(1)
	}

	backfillStart, _ := time.Parse("2006-01-02", app.Cfg.Ingest.BackfillStartDate)
	client := fred.New(fredDefaultBaseURL, app.Cfg.FRED.APIKey)
	ing := fred.NewIngester(client, app.Pool, fred.Config{
		Series:            app.Cfg.Ingest.FREDSeries,
		BackfillStartDate: backfillStart,
		Retry: retry.Config{
			MaxAttempts:       app.Cfg.Retry.MaxAttempts,
			BackoffInitialMs:  app.Cfg.Retry.BackoffInitialMs,
			BackoffMultiplier: app.Cfg.Retry.BackoffMultiplier,
		},
	}, app.Cfg.General.Environment)

	res, err := ing.Run(ctx)
	if err != nil {
		repo.FinishRun(ctx, app.Pool, runID, repo.RunResult{
			Status: "failed", Error: err, RowsProcessed: res.RowsProcessed, RetryCount: res.RetryCount,
		})
		app.Logger.Error("FRED 인제스트 실패", "err", err, "rows", res.RowsProcessed)
		os.Exit(1)
	}

	repo.FinishRun(ctx, app.Pool, runID, repo.RunResult{
		Status: "success", RowsProcessed: res.RowsProcessed, RetryCount: res.RetryCount,
	})
	app.Logger.Info("FRED 인제스트 완료", "rows", res.RowsProcessed, "retries", res.RetryCount)
	fmt.Printf("✅ FRED 수집 완료: %d행 (재시도 %d회)\n", res.RowsProcessed, res.RetryCount)
}
```

- [ ] **Step 3: cli/status.go 진짜 구현**

`go/internal/cli/status.go` 전체 교체:
```go
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Claude-su-Factory/quant-bot/go/internal/repo"
)

func RunStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "config/config.toml", "config 파일 경로")
	fs.Parse(args)

	ctx := context.Background()
	app, err := Bootstrap(ctx, *configPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	defer app.Close()

	fmt.Println("=== quant-bot 운영 상태 ===")
	fmt.Println("환경:", app.Cfg.General.Environment)
	fmt.Println()

	runs, err := repo.RecentRuns(ctx, app.Pool, 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, "RecentRuns 실패:", err)
		os.Exit(1)
	}
	fmt.Println("최근 실행 (10건):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "시작\t작업\t상태\t행수\t재시도\t소요\t에러")
	for _, r := range runs {
		dur := ""
		if r.FinishedAt != nil {
			dur = r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond).String()
		} else {
			dur = "(진행중/비정상)"
		}
		emoji := statusEmoji(r.Status)
		errMsg := r.ErrorMessage
		if len(errMsg) > 40 {
			errMsg = errMsg[:40] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s %s\t%d\t%d\t%s\t%s\n",
			r.StartedAt.Local().Format("2006-01-02 15:04:05"),
			r.JobName, emoji, r.Status, r.RowsProcessed, r.RetryCount, dur, errMsg)
	}
	w.Flush()
	fmt.Println()

	// 시리즈 현황
	fmt.Println("거시 시리즈 현황:")
	rows, err := app.Pool.Query(ctx,
		`SELECT series_id, MAX(observed_at), COUNT(*), MAX(ingested_at)
		 FROM macro_series GROUP BY series_id ORDER BY series_id`)
	if err == nil {
		ws := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(ws, "시리즈\t마지막 관측\t총 행수\tDB 입력")
		for rows.Next() {
			var sid string
			var observed, ingested time.Time
			var n int
			rows.Scan(&sid, &observed, &n, &ingested)
			fmt.Fprintf(ws, "%s\t%s\t%d\t%s\n", sid,
				observed.Local().Format("2006-01-02"), n,
				ingested.Local().Format("2006-01-02 15:04:05"))
		}
		rows.Close()
		ws.Flush()
	}
	fmt.Println()

	// 비정상 종료
	stale, err := repo.StaleRuns(ctx, app.Pool)
	if err == nil {
		if len(stale) == 0 {
			fmt.Println("비정상 종료: 0건")
		} else {
			fmt.Printf("⚠️  비정상 종료 (started_at + 1h 넘게 finished_at NULL): %d건\n", len(stale))
			for _, r := range stale {
				fmt.Printf("   id=%d %s 시작 %s\n", r.ID, r.JobName,
					r.StartedAt.Local().Format("2006-01-02 15:04:05"))
			}
		}
	}
	fmt.Println()
	fmt.Println("LaunchAgent: launchctl list | grep com.quantbot 로 활성 여부 확인")
	fmt.Println("다음 예정: 매일 22:00 (시스템 로컬 타임존, plist 기준)")
}

func statusEmoji(s string) string {
	switch s {
	case "success":
		return "✅"
	case "failed":
		return "❌"
	case "running":
		return "⏳"
	default:
		return "❓"
	}
}
```

- [ ] **Step 4: 빌드 + go vet**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && go build -o quantbot ./cmd/quantbot && go vet ./...
```

Expected: 빌드 성공, vet 0 issues.

- [ ] **Step 5: 통합 시나리오 검증 (수동)**

```bash
cd /Users/yuhojin/Desktop/quant-bot && make up && sleep 8
cd /Users/yuhojin/Desktop/quant-bot/go
./quantbot migrate up
./quantbot status        # runs 0건 + 시리즈 0건 보여야 함
cd /Users/yuhojin/Desktop/quant-bot && make down
```

Expected: migrate up 완료. status가 빈 표 출력 (에러 없이).

- [ ] **Step 6: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/cli/
git commit -m "feat(cli): implement ingest/migrate/status subcommands

- migrate up/status: goose wrapper, requireMigrated=false
- ingest fred: StartRun → fred.Ingester → FinishRun + logger output
- status: RecentRuns + macro_series summary + StaleRuns warning
  + tabwriter for aligned tables + emoji status indicators
- All commands use Bootstrap (R12 fail-fast)"
```

---

## Task 9: LaunchAgent + make install/uninstall/logs/build (infra task)

**Files:**
- Create: `deploy/launchd/com.quantbot.ingest-fred.plist`
- Modify: `Makefile` (root), `go/Makefile`

- [ ] **Step 1: deploy/launchd 디렉터리 + plist**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/deploy/launchd
```

Create `deploy/launchd/com.quantbot.ingest-fred.plist`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.quantbot.ingest-fred</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{QUANTBOT_BINARY_PATH}}</string>
        <string>ingest</string>
        <string>fred</string>
    </array>
    <key>WorkingDirectory</key>
    <string>{{QUANTBOT_PROJECT_PATH}}</string>
    <key>StartCalendarInterval</key>
    <dict>
        <key>Hour</key><integer>22</integer>
        <key>Minute</key><integer>0</integer>
    </dict>
    <key>StandardOutPath</key>
    <string>{{QUANTBOT_PROJECT_PATH}}/logs/launchd-ingest-fred.out.log</string>
    <key>StandardErrorPath</key>
    <string>{{QUANTBOT_PROJECT_PATH}}/logs/launchd-ingest-fred.err.log</string>
    <key>RunAtLoad</key>
    <false/>
    <key>EnvironmentVariables</key>
    <dict>
        <key>QUANTBOT_CONFIG</key>
        <string>{{QUANTBOT_PROJECT_PATH}}/config/config.toml</string>
    </dict>
</dict>
</plist>
```

- [ ] **Step 2: go/Makefile build 갱신 (-ldflags + -o)**

`go/Makefile`의 `build:` target 교체. OLD:
```makefile
build:  ## 모든 cmd 빌드
	go build ./...
```

NEW:
```makefile
build:  ## quantbot binary 빌드 (-o quantbot, ldflags Version 임베드)
	go build -o quantbot -ldflags="-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" ./cmd/quantbot
```

- [ ] **Step 3: 루트 Makefile에 install/uninstall/logs/build 추가**

루트 `Makefile`의 `.PHONY` 갱신. OLD:
```
.PHONY: help up down db-check test test-integration fmt lint prepare-migrations
```
NEW:
```
.PHONY: help up down db-check test test-integration fmt lint prepare-migrations build install uninstall logs
```

`prepare-migrations` target 다음에 추가:
```makefile
build: prepare-migrations  ## quantbot binary 빌드 (마이그레이션 임베드 포함)
	$(MAKE) -C go build

install: ## quantbot 빌드 + 마이그레이션 적용 + LaunchAgent 등록 (1회 셋업)
	docker compose -f docker/docker-compose.yml up -d --wait
	$(MAKE) build
	./go/quantbot migrate up
	@mkdir -p ~/Library/LaunchAgents logs
	@sed -e "s|{{QUANTBOT_BINARY_PATH}}|$(CURDIR)/go/quantbot|g" \
	     -e "s|{{QUANTBOT_PROJECT_PATH}}|$(CURDIR)|g" \
	     deploy/launchd/com.quantbot.ingest-fred.plist \
	     > ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@launchctl unload ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist 2>/dev/null || true
	@launchctl load ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@echo ""
	@echo "✅ 셋업 완료."
	@echo ""
	@echo "📅 매일 22:00 (시스템 로컬 타임존)에 자동 실행됨."
	@echo "    install 직후엔 즉시 실행되지 않습니다 (다음 정시 대기)."
	@echo "    지금 한 번 테스트하려면: launchctl start com.quantbot.ingest-fred"
	@echo ""
	@echo "🔍 상태 확인:    ./go/quantbot status"
	@echo "📜 로그 tail:    make logs"
	@echo "🗑  제거:         make uninstall"

uninstall: ## LaunchAgent 제거 (DB·로그·binary 보존)
	@launchctl unload ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@echo "✅ LaunchAgent 제거됨. (DB·로그·binary는 그대로 — 데이터 보존)"

logs: ## 오늘 봇 로그 tail (Ctrl+C로 종료)
	@tail -f logs/app-$$(date +%Y-%m-%d).log | jq -C .
```

- [ ] **Step 4: install 동작 검증 (사용자 환경 1회)**

```bash
cd /Users/yuhojin/Desktop/quant-bot
make install
launchctl list | grep com.quantbot
```

Expected: `make install` 성공 + LaunchAgent 등록 확인.

- [ ] **Step 5: 즉시 1회 실행 테스트**

```bash
launchctl start com.quantbot.ingest-fred
sleep 5
./go/quantbot status
```

Expected: status에 새 run 1건 보임. (FRED API key 비어있으면 dev 환경에선 경고만 — 실제 fetch는 실패할 수 있음. paper 환경에선 종료. dev 사용 권장.)

- [ ] **Step 6: 정리 (테스트 후)**

```bash
make uninstall
make down
```

- [ ] **Step 7: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add deploy/ Makefile go/Makefile
git commit -m "feat(deploy): LaunchAgent plist + make install/uninstall/logs/build

- com.quantbot.ingest-fred.plist: daily 22:00 trigger, sed-templated paths
- root Makefile install: up + build + migrate up + plist install (one command)
- root Makefile uninstall: launchctl unload + plist remove (preserves data)
- root Makefile logs: tail today's app log with jq color
- root Makefile build: prepare-migrations + delegate to go/Makefile
- go/Makefile build: -o quantbot + ldflags Version injection from git describe"
```

---

## Task 10: R14 + 문서 동기화 + Phase 1b-A 완료 태그 (doc-only task)

**Files:**
- Modify: `docs/ARCHITECTURE.md`, `CLAUDE.md`, `docs/STATUS.md`, `docs/ROADMAP.md`

- [ ] **Step 1: ARCHITECTURE.md R 표에 R14 추가**

`docs/ARCHITECTURE.md`에서 R13 줄 다음에 R14 추가:
```
| R14 | 운영 작업은 stateless CLI + macOS launchd (사용자 매일 부담 0) | [phase1b-a §4](superpowers/specs/2026-05-03-phase1b-a-ingest-infra-fred-design.md) |
```

- [ ] **Step 2: CLAUDE.md R 표에 R14 추가**

`CLAUDE.md`의 R13 줄 다음에 추가:
```
| R14 | 운영 작업은 stateless CLI + macOS launchd (사용자 매일 부담 0) | [§4](docs/superpowers/specs/2026-05-03-phase1b-a-ingest-infra-fred-design.md) |
```

- [ ] **Step 3: STATUS.md 갱신**

(a) 헤더:

OLD: `**현재 Phase**: Phase 1a 완료. 다음: Phase 1b — 데이터 인제스트 (Go)`
NEW: `**현재 Phase**: Phase 1b-A 완료. 다음: Phase 1b-B — Alpaca 가격 + EDGAR 재무제표 수집기`

(b) 마지막 업데이트: `2026-05-03` (변경 없음 — 같은 날)

(c) Phase 진행 상황:

OLD:
```
- [ ] Phase 1b — 데이터 인제스트 (Go)
```

NEW:
```
- [x] Phase 1b-A — 인프라 + FRED 수집기 (2026-05-03 완료)
- [ ] Phase 1b-B — Alpaca + EDGAR 수집기
```

(d) 최근 변경 이력 맨 위 추가:
```
- **2026-05-03** Phase 1b-A 완료 — TimescaleDB 마이그레이션 (goose), retry helper, repo 패턴, FRED 인제스터 (4 시리즈 × 20년 백필), CLI (ingest/status/migrate/version), LaunchAgent 자동 셋업 (사용자 매일 부담 0), R14 도입, v0.2.0-phase1b-a 태그
```

- [ ] **Step 4: ROADMAP.md 갱신**

(a) 현재 추천:

OLD: `**현재 추천 다음 작업**: Phase 1b — 데이터 인제스트 (Go)`
NEW: `**현재 추천 다음 작업**: Phase 1b-B — Alpaca 가격 + EDGAR 재무제표 수집기`

(b) `### Phase 1b — 데이터 인제스트 (Go)` 섹션을 다음 두 섹션으로 교체:

```markdown
### Phase 1b-B — Alpaca 가격 + EDGAR 재무제표 수집기 [Tier 1 필수]

Phase 1b-A에서 정립한 인제스터 패턴(retry · repo · CLI · LaunchAgent)을 두 데이터 소스에 확장:
- Alpaca 일봉 가격 수집기 (S&P 500 종목군 — 미국 대형주 500개) — `prices_daily` hypertable
- **EDGAR 재무제표 수집기** — SEC 공시 분기·연간 재무제표 (10-Q, 10-K) — `fundamentals` 테이블
- 새 LaunchAgent 2개 (com.quantbot.ingest-alpaca, com.quantbot.ingest-edgar)
```

(c) Tier 분류:

OLD: `- **Tier 1 (필수)**: Phase 1b, 2, 3, 4, 4.5, 5, 6, 7`
NEW: `- **Tier 1 (필수)**: Phase 1b-B, 2, 3, 4, 4.5, 5, 6, 7`

- [ ] **Step 5: 검증**

```bash
cd /Users/yuhojin/Desktop/quant-bot
grep -q "R14" docs/ARCHITECTURE.md && grep -q "R14" CLAUDE.md && echo "R14 OK"
grep -q "Phase 1b-A 완료" docs/STATUS.md && echo "STATUS OK"
grep -q "Phase 1b-B" docs/ROADMAP.md && echo "ROADMAP OK"
```

Expected: 3줄 모두 OK.

- [ ] **Step 6: 커밋 + 태그**

```bash
git add docs/ARCHITECTURE.md CLAUDE.md docs/STATUS.md docs/ROADMAP.md
git commit -m "$(cat <<'EOF'
docs: sync R14 + mark Phase 1b-A complete

- ARCHITECTURE.md, CLAUDE.md: R14 한 줄 요약 추가 (CLI + LaunchAgent)
- STATUS.md: Phase 1b-A 완료 표시, 변경 이력 갱신
- ROADMAP.md: Phase 1b 섹션을 1b-B(Alpaca + EDGAR)로 변경
EOF
)"

git tag -a v0.2.0-phase1b-a -m "Phase 1b-A complete: ingest infrastructure + FRED collector (R14: stateless CLI + LaunchAgent)"
```

- [ ] **Step 7: GitHub 푸시 (사용자 승인 후)**

오너 결정 후 푸시:
```bash
git push origin main
git push --tags
```

(`v0.0.1-phase0`, `v0.1.0-phase1a` 이미 GitHub에 있고 `v0.2.0-phase1b-a` 신규.)
