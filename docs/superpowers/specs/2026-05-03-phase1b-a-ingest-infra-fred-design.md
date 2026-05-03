---
title: Phase 1b-A — 데이터 인제스트 인프라 + FRED 수집기
date: 2026-05-03
status: Brainstorming approved (pending user spec review)
phase: Phase 1b-A — Ingest Infrastructure + FRED Collector
scope_note: 인제스트 패턴 정립용 vehicle = FRED. Alpaca·EDGAR는 Phase 1b-B에서 동일 패턴 적용.
authors: yuhojin, Claude
---

# Phase 1b-A — 데이터 인제스트 인프라 + FRED 수집기

## 1. 목표

Phase 1a 인프라(설정·로거·DB 풀·에러 패턴) 위에 **데이터 인제스트 패턴**을 정립하고, **FRED 거시 데이터 4종**을 매일 자동 수집한다.

본 phase 끝나면:
- DB에 `macro_series` hypertable + `runs` 모니터링 테이블 살아있음
- `quantbot ingest fred` 명령으로 FRED 수집 1회 실행 가능
- macOS LaunchAgent로 매일 22:00 KST 자동 호출
- `quantbot status` 명령으로 운영 상태 확인 가능
- 사용자 매일 운영 부담 = 0건 (`make install` 1회 셋업만)

이게 정립되면 Phase 1b-B에서 Alpaca·EDGAR가 같은 패턴 따라가기만 하면 됨.

## 2. 범위 외 (Phase 1b-A에 포함하지 않음)

- Alpaca 가격 수집기 — Phase 1b-B
- EDGAR 재무제표 수집기 — Phase 1b-B
- `prices_daily` 테이블 (Alpaca와 함께 도입) — Phase 1b-B
- `fundamentals` 테이블 (EDGAR와 함께) — Phase 1b-B
- 외부 알림 채널 (Slack/Discord/Telegram) — Phase 7
- Python 인프라 (config/db/log/errors) — Phase 2 (Phase 1a에서 이연됨)
- `[ingest]` config의 다른 데이터 소스 키 — Phase 1b-B에서 추가

## 3. 컨텍스트 (브레인스토밍 결정)

| # | 항목 | 결정 |
|---|------|------|
| 1 | 분해 | Phase 1b를 1b-A(인프라+FRED) + 1b-B(Alpaca+EDGAR)로 분리 |
| 2 | DB 마이그레이션 도구 | `pressly/goose` (embed.FS 친화, up/down 단일 파일) |
| 3 | 백필 기간 | 20년 (2006~현재) — 2008 위기 포함 |
| 4 | FRED 시리즈 | T10Y2Y, VIXCLS, BAMLH0A0HYM2, DFF (4개) |
| 5 | 실행 방식 | CLI + macOS LaunchAgent (옵션 D) |
| 6 | 모니터링 | `runs` 테이블 + `quantbot status` 명령 |

## 4. 핵심 설계 결정 (R14 신규)

기존 R1~R13에 이어 추가.

### R14. 운영 작업은 stateless CLI + 외부 스케줄러 (macOS launchd)

- **Why**:
  - 사용자가 "컴퓨터 잠시 꺼질 수 있음" 명시 → 데몬은 죽음·부활 상태 관리 복잡.
  - R2(상태는 DB)와 정합 — fresh process마다 DB에서 상태 복구.
  - long-running 데몬의 메모리·goroutine 누수 영역 자체가 없음.
  - 노트북 환경 자원 부담 ↓ (24/7 대신 매일 수 분).
- **How**:
  - 모든 봇 작업은 단발 CLI 명령 (`quantbot ingest fred`, `quantbot status` 등).
  - 매일 자동 실행은 macOS launchd LaunchAgent 위임.
  - 봇 코드 안에 자체 스케줄러·crontab 파싱 X.
  - `make install`이 LaunchAgent plist를 사용자 홈에 자동 설치 → 1회 셋업.
- **예외**: 만약 향후 (Phase 6+) intraday 거래로 갈 경우 long-running 필요 → R14 재논의. Phase 1b~Phase 9 동안은 swing 빈도라 R14 유효.

## 5. 데이터 스키마 (4개 테이블)

### 5.1 마이그레이션 추적 (goose 자동 관리)

```sql
-- goose가 첫 실행 시 자동 생성 — 우리가 직접 만들지 않음
CREATE TABLE goose_db_version (
    id SERIAL PRIMARY KEY,
    version_id BIGINT NOT NULL,
    is_applied BOOLEAN NOT NULL,
    tstamp TIMESTAMP DEFAULT NOW()
);
```

### 5.2 거시 시계열 (TimescaleDB hypertable)

`CREATE EXTENSION`은 트랜잭션 회피가 안전하므로 **두 마이그레이션 파일로 분리**.

`shared/schema/migrations/20260503000001_enable_timescaledb.sql`:
```sql
-- +goose Up
-- +goose NO TRANSACTION
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- +goose Down
-- +goose NO TRANSACTION
DROP EXTENSION IF EXISTS timescaledb;
```

`shared/schema/migrations/20260503000002_create_macro_series.sql`:
```sql
-- +goose Up
CREATE TABLE macro_series (
    series_id   TEXT NOT NULL,                          -- 'T10Y2Y', 'VIXCLS', etc.
    observed_at TIMESTAMPTZ NOT NULL,                   -- 시리즈 관측 일자
    value       NUMERIC(20, 8),                         -- FRED 8 decimal precision. NULL 허용 (휴장일)
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),     -- DB 입력 시각 (R13 point-in-time)
    source      TEXT NOT NULL DEFAULT 'fred',
    PRIMARY KEY (series_id, observed_at)
);
SELECT create_hypertable('macro_series', 'observed_at');
-- Note: 별도 macro_series_series_idx CREATE INDEX 없음. PK (series_id, observed_at)가
-- backward scan 가능하므로 DESC 인덱스 중복. 향후 query 프로파일링 결과 필요 시 추가.

-- +goose Down
DROP TABLE macro_series;
```

**키 결정**:
- `(series_id, observed_at)` PK → idempotent insert 가능 (`ON CONFLICT DO NOTHING`)
- `observed_at`이 hypertable time 컬럼
- `value` NULLABLE — FRED는 휴장일·발표 전일을 NULL로 반환
- `ingested_at` 보존으로 R13 point-in-time 추적 (언제 우리가 그 값을 알게 됐는가)

### 5.3 운영 실행 기록 (모니터링용)

```sql
-- shared/schema/migrations/20260503000003_create_runs.sql
-- +goose Up
CREATE TABLE runs (
    id              BIGSERIAL PRIMARY KEY,
    job_name        TEXT NOT NULL,                        -- 'ingest_fred', 'ingest_alpaca' (Phase 1b-B), ...
    instance        TEXT NOT NULL DEFAULT 'paper',        -- R3 instance 토큰 (paper/live/dev/test)
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,                          -- NULL = 진행 중 또는 비정상 종료
    status          TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),  -- DB-level enum
    rows_processed  INTEGER NOT NULL DEFAULT 0,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT
);
CREATE INDEX runs_recent_idx ON runs (started_at DESC);
CREATE INDEX runs_job_status_idx ON runs (job_name, status, started_at DESC);

-- +goose Down
DROP TABLE runs;
```

**키 결정**:
- hypertable 아님 (작은 메타데이터, 시계열 분석 아님)
- `id` BIGSERIAL — 인스턴스 간 충돌 가능성 X (단일 봇 가정 R3)
- `finished_at IS NULL`이면 비정상 종료 흔적 → status 명령에서 경고로 표시
- `instance` 컬럼 — paper/live 데이터 섞이지 않게 분리

### 5.4 (Phase 1b-B에서 추가될 것 — 본 phase 범위 외)

- `prices_daily` (Alpaca OHLCV)
- `fundamentals` (EDGAR 재무제표)

## 6. CLI 구조

### 6.1 진입점

`go/cmd/quantbot/main.go` — 단일 binary, subcommand 라우팅.

### 6.2 명령 목록 (Phase 1b-A 기준)

| 명령 | 동작 |
|------|------|
| `quantbot ingest fred` | FRED 4개 시리즈 증분 수집 (백필 포함) |
| `quantbot status` | 최근 runs + 시리즈 현황 표 출력 |
| `quantbot migrate up` | goose 마이그레이션 적용 |
| `quantbot migrate status` | 마이그레이션 적용 상태 |
| `quantbot version` | 봇 버전 (build info) |

(Phase 1b-B에서 `ingest alpaca`, `ingest edgar` 추가)

### 6.3 라우팅 구현 — stdlib `flag` 사용

cobra·urfave/cli 같은 라이브러리 도입 X. 명령 5개 미만이라 stdlib `flag.NewFlagSet` + switch면 충분.

```go
// go/cmd/quantbot/main.go (구조 예시)
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
    case "version":
        cli.RunVersion()
    default:
        printUsage()
        os.Exit(2)
    }
}
```

각 subcommand는 `go/internal/cli/{ingest,status,migrate,version}.go`에 자체 함수.

### 6.4 공통 부트스트랩 (모든 명령이 공유)

```go
// go/internal/cli/bootstrap.go
type App struct {
    Cfg    *config.Config
    Logger *logging.Logger
    Pool   *pgxpool.Pool
    Close  func() error  // 풀 + 로그 파일 모두 닫음
}

// Bootstrap: config 로드 → 검증 → 로거 → DB 풀 → 헬스체크 → 마이그레이션 상태 확인 (R12 fail-fast)
func Bootstrap(ctx context.Context, configPath string, requireMigrated bool) (*App, error) {
    // 1. config.Load(configPath) — 실패 시 ErrConfigMissing/ErrConfigInvalid
    // 2. logging.New(...) → logger + closeLogger
    // 3. db.NewPool(ctx, cfg.Database) — 1초 ping (R12)
    // 4. requireMigrated이면 migrate.AssertUpToDate(ctx, pool) — 미적용 마이그레이션 있으면 ErrMigrationsPending
    // 5. App.Close = func() { pool.Close(); closeLogger() }  ← 풀 + 로그 둘 다
}
```

**`requireMigrated` 플래그**:
- `migrate up`/`migrate status`/`version` → false (마이그레이션 자체가 대상이라)
- `ingest fred`/`status` → true (DB 테이블 의존)

미적용 마이그레이션 감지 시 명확한 에러:
```
ERROR: 미적용 마이그레이션 N개 발견. 'quantbot migrate up' 먼저 실행하세요.
       (또는 'make install'이 자동 적용)
```

**Close 책임**: DB 풀(`pool.Close()`) + 로그 파일(`closeLogger()`) 모두. 모든 subcommand는 `defer app.Close()` 호출.

## 7. FRED 인제스터

### 7.1 FRED API 사양

- Endpoint: `https://api.stlouisfed.org/fred/series/observations`
- 인증: query param `api_key=<key>`
- Rate limit: 120 req/분 (실효 충분)
- 주요 query params:
  - `series_id` (예: `T10Y2Y`)
  - `observation_start` (ISO 8601 date, 예: `2006-01-01`)
  - `observation_end` (생략 시 현재까지)
  - `file_type=json`
- 응답 형식 (관심 부분만):
  ```json
  {
    "observations": [
      {"date": "2006-01-03", "value": "0.13"},
      {"date": "2006-01-04", "value": "0.15"},
      {"date": "2006-01-05", "value": "."}
    ]
  }
  ```
- `value`가 `"."`이면 휴장일 → DB에 NULL 저장.

### 7.2 인제스트 흐름 (`quantbot ingest fred` 1회 실행)

```
Bootstrap (config + logger + DB pool + ping)  ← R12 fail-fast
  ↓
runs INSERT (started_at=NOW, status='running')  ← run_id 반환
  ↓
for each series in cfg.Ingest.FredSeries:
    a. SELECT MAX(observed_at) FROM macro_series WHERE series_id=$1
       → 마지막 관측일 (없으면 cfg.Ingest.BackfillStartDate 사용)
    b. retry.Do(fetchFREDSeries(series, lastDate+1, today)):
       - HTTP GET, JSON parse, 백오프 재시도 (cfg.Retry)
    c. INSERT INTO macro_series ... ON CONFLICT DO NOTHING
       (idempotent — 같은 (series, date) 재삽입 안 됨)
    d. rows_processed += 새로 들어간 행 수
    e. retry_count += 재시도 횟수
  ↓
모두 성공 → runs UPDATE (status='success', finished_at=NOW, 통계)
실패 → runs UPDATE (status='failed', error_message=err.Error()), exit 1
defer pool.Close()
```

### 7.3 FRED HTTP 클라이언트 + 재시도

`go/internal/ingest/fred/client.go`:
- `FetchSeries(ctx, seriesID, start, end) ([]Observation, error)`
- 5xx, 네트워크 에러 → retry 대상
- 4xx (잘못된 series_id, 잘못된 API key) → 즉시 종료 (재시도 무의미)
- 429 (rate limit) → 백오프 재시도

### 7.4 Idempotency (중요)

같은 날 두 번 `quantbot ingest fred` 실행해도:
- `MAX(observed_at)` 쿼리 → 어제까지 받은 게 잡힘
- 오늘 데이터만 새로 가져옴
- `ON CONFLICT DO NOTHING`으로 중복 insert 방지
- 결과: rows_processed=0, status='success' (이미 다 있음)

LaunchAgent 누락 보충 실행도 안전.

### 7.5 INSERT 구현 디테일

```go
// repo/macro_series.go
func InsertObservations(ctx context.Context, pool *pgxpool.Pool, obs []Observation) (rowsInserted int, err error) {
    // 배치 INSERT with ON CONFLICT DO NOTHING
    // 반환값 rowsInserted = pgx CommandTag.RowsAffected()
    // (PostgreSQL은 ON CONFLICT DO NOTHING 시 실제 insert된 행 수만 카운트)
}
```

핵심:
- 단일 트랜잭션으로 한 series의 모든 observation 처리 (부분 실패 방지)
- `ON CONFLICT (series_id, observed_at) DO NOTHING` (PK 정의로 자동)
- `RowsAffected()`가 새로 들어간 행 수 (이미 있던 건 카운트 안 됨) — `runs.rows_processed` 누적에 사용

### 7.6 동일 시점 다중 실행 시나리오

LaunchAgent가 22:00에 호출했는데 실패 → launchd가 자동 재시도 → 같은 시각 두 번째 process. 이런 경우:
- 두 process가 각자 새 `runs` row 생성 (id 별개)
- `macro_series` insert는 idempotent → 데이터 중복 X
- 사용자가 `quantbot status`에서 보면 같은 작업이 두 번 보일 수 있음 — 정상. 의도된 추적.
- 단순화 위해 process-level mutex 도입 X (launchd가 거의 동시 호출 안 함, 충돌 시 ON CONFLICT가 데이터 보호).

## 8. Retry helper

### 8.1 위치

`go/internal/retry/retry.go` 신규 패키지.

### 8.2 API

```go
package retry

import (
    "context"
    "time"
)

type Config struct {
    MaxAttempts       int     // ≥1
    BackoffInitialMs  int     // ≥1
    BackoffMultiplier float64 // ≥1.0
}

// IsRetryable는 op이 반환한 에러를 재시도해야 하는지 판단.
// nil이면 모든 에러 재시도 (단순).
type IsRetryable func(err error) bool

// Do는 op를 재시도와 함께 실행. 마지막 시도까지 실패 시 마지막 에러 반환.
// ctx 취소·deadline 존중. 재시도 횟수는 returned int로 반환.
func Do(ctx context.Context, cfg Config, isRetryable IsRetryable, op func() error) (retries int, err error)
```

- exponential backoff: 1초 → 2초 → 4초 (cfg.BackoffMultiplier=2.0 기준)
- ctx done 즉시 반환 (cancellation 우선)
- isRetryable=nil → 모든 에러 재시도 (단순 사용)

### 8.3 IsRetryable 정책 (FRED 인제스터 적용)

`go/internal/ingest/fred/client.go`에서 정의:

```go
// FRED API 호출 결과 분류
func isRetryableFRED(err error) bool {
    var httpErr *HTTPError
    if errors.As(err, &httpErr) {
        // 4xx (잘못된 series_id, 잘못된 API key) → 재시도 무의미
        if httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
            return false
        }
        // 5xx (서버 오류), 429 (rate limit) → 재시도 가능
        return true
    }
    // 네트워크 에러 (connection reset, timeout) → 재시도 가능
    return true
}
```

이 분류로 retry.Do 호출:
```go
retries, err := retry.Do(ctx, cfg.Retry, isRetryableFRED, func() error {
    return fetchSeriesOnce(ctx, seriesID, start, end)
})
```

### 8.3 config 추가 (`config/config.toml` + example)

```toml
[retry]
max_attempts = 3
backoff_initial_ms = 1000
backoff_multiplier = 2.0

[ingest]
backfill_start_date = "2006-01-01"   # ISO 8601 (R13 — 사람이 손으로 채우는 자리)
fred_series = ["T10Y2Y", "VIXCLS", "BAMLH0A0HYM2", "DFF"]
```

검증 규칙 추가 (`config.validate`):
- `retry.max_attempts ≥ 1`
- `retry.backoff_initial_ms ≥ 1`
- `retry.backoff_multiplier ≥ 1.0`
- `ingest.backfill_start_date` 파싱 가능 ISO 8601 (`time.Parse("2006-01-02", ...)`)
- `ingest.fred_series` 비어있지 않음

## 9. 마이그레이션 (goose)

### 9.1 의존성

```bash
go get github.com/pressly/goose/v3@v3.21.0
```

### 9.2 SQL 파일 위치

`shared/schema/migrations/`:
- `20260503000001_create_macro_series.sql`
- `20260503000002_create_runs.sql`

(파일명 `YYYYMMDDHHmmss_<name>.sql` — goose 표준)

### 9.3 임베드

`go/internal/migrate/migrate.go`:
```go
package migrate

import (
    "embed"
    "github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS  // 빌드 타임에 SQL 파일 binary에 포함

// Up applies all pending migrations.
func Up(ctx context.Context, pool *pgxpool.Pool) error { ... }

// Status prints applied/pending migrations.
func Status(ctx context.Context, pool *pgxpool.Pool) error { ... }
```

**중요**: `//go:embed migrations/*.sql`이 동작하려면 SQL 파일이 `go/internal/migrate/migrations/`에 있어야 함. R9는 `shared/schema/`가 단일 진실 원천이라 했음. **해결**: build 시 `shared/schema/migrations/`를 `go/internal/migrate/migrations/`에 symlink 또는 복사.

→ **오너 결정**: `Makefile` `prepare-migrations` target이 `cp -r shared/schema/migrations/ go/internal/migrate/migrations/`로 동기화. `make build` / `make test`가 자동 호출. `go/internal/migrate/migrations/`는 `.gitignore` (생성물).

이렇게 하면 R9 (단일 진실 원천 = `shared/schema/`) 유지하면서 goose embed 가능.

### 9.4 CLI 호출

```bash
quantbot migrate up      # 미적용 마이그레이션 모두 적용
quantbot migrate status  # 적용/미적용 목록
```

### 9.5 트랜잭션 보장

goose는 default로 각 마이그레이션을 트랜잭션 안에서 실행. SQL 파일이 부분 실패하면 자동 rollback. CREATE EXTENSION 같은 트랜잭션 불가능 명령은 SQL 파일 안에서 `-- +goose NO TRANSACTION` annotation으로 분리 가능.

본 phase의 두 마이그레이션:
- `create_macro_series.sql`: `CREATE EXTENSION` + `CREATE TABLE` + `create_hypertable()`. CREATE EXTENSION만 트랜잭션 회피 필요할 수 있음 — 실패 시 별도 SQL 파일로 분리.
- `create_runs.sql`: 단순 CREATE TABLE — 트랜잭션 안전.

### 9.6 단위 테스트의 fixture 마이그레이션

`go/internal/migrate/migrations/`는 빌드 시 `shared/schema/migrations/`에서 복사되어 gitignore. 단위 테스트가 마이그레이션 SQL 직접 사용해야 하면 두 옵션:

- (a) 단위 테스트는 마이그레이션 동작이 아닌 wrapper 함수 동작만 검증 (예: `goose.SetBaseFS` 호출 확인). 실제 SQL은 통합 테스트.
- (b) `migrate_test.go`에 hardcoded 작은 SQL fixture (`CREATE TABLE _test_dummy ...`) 사용.

→ **결정**: (a) 채택. migrate 단위 테스트는 wrapper 함수 동작만. 실제 SQL 적용은 `_integration_test.go`에서 build tag로 분리, 실제 Postgres에 적용해 schema_migrations 테이블 검증.

## 10. `quantbot status` 출력

### 10.1 명령 동작

DB의 `runs` + `macro_series` 조회 후 표 출력.

```bash
$ quantbot status
=== quant-bot 운영 상태 ===
환경: paper
빌드: v0.1.0-phase1a-1-gXXXXXXX (또는 git describe)

최근 10건 실행 (runs 테이블):
시작 시각              작업           상태     행수    재시도  소요
2026-05-03 22:00:01   ingest_fred    ✅       2,184   0      1.2s
2026-05-02 22:00:01   ingest_fred    ✅       2,180   0      1.1s
2026-05-01 22:00:01   ingest_fred    ⚠️ 재시도 2,180   1      3.4s
2026-04-30 22:00:01   ingest_fred    ❌ 실패   0       3      1.0s
   에러: FRED API 503 Service Unavailable

데이터 시리즈 현황:
시리즈              마지막 관측        총 행수    DB 입력 시각
T10Y2Y              2026-05-02         5,243      2026-05-03 22:00:02
VIXCLS              2026-05-02         5,243      2026-05-03 22:00:03
BAMLH0A0HYM2        2026-05-02         5,243      2026-05-03 22:00:03
DFF                 2026-05-03         5,244      2026-05-03 22:00:04

비정상 종료 (finished_at IS NULL): 0건  ← 1건 이상 시 ⚠️ 빨간 경고 + 해당 run id 표시
LaunchAgent 등록 상태: ✅ 활성 (com.quantbot.ingest-fred)
다음 예정 실행: 매일 22:00 (시스템 로컬 타임존)
```

### 10.3 "다음 예정 실행" 출처

LaunchAgent의 `StartCalendarInterval`은 plist에 하드코딩된 시각. status 명령은 plist 파싱하지 않고 단순 표기 ("매일 22:00 시스템 로컬 타임존") + `launchctl list com.quantbot.ingest-fred` 결과 라인 표시 (활성/비활성). plist 파싱 도입은 over-engineering.

### 10.4 비정상 종료 표시

`SELECT COUNT(*) FROM runs WHERE finished_at IS NULL AND started_at < NOW() - INTERVAL '1 hour'` (실행 중 1시간 넘음 = 비정상 의심).

- 0건: 초록색 ✅ 또는 그냥 "0건"
- 1건 이상: 빨간색 ⚠️ + run id 목록 + 시작 시각

### 10.2 위치

`go/internal/cli/status.go` — SQL 두 개 + `text/tabwriter`로 정렬 출력.

## 11. LaunchAgent + 자동 셋업

### 11.1 plist 템플릿

`deploy/launchd/com.quantbot.ingest-fred.plist`:

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

`{{...}}` 자리는 `make install`이 실제 경로로 치환.

### 11.2 `make install` / `make uninstall` (루트 Makefile)

```makefile
install: ## quantbot 빌드 + 마이그레이션 적용 + LaunchAgent 등록 (1회 셋업)
	docker compose -f docker/docker-compose.yml up -d --wait  # healthcheck 통과까지 대기 (sleep보다 안전)
	$(MAKE) prepare-migrations
	$(MAKE) -C go build
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

uninstall: ## LaunchAgent 제거
	@launchctl unload ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@echo "✅ LaunchAgent 제거됨. (DB·로그·binary는 그대로 — 데이터 보존)"

logs: ## 오늘 봇 로그 tail (Ctrl+C로 종료)
	@tail -f logs/app-$$(date +%Y-%m-%d).log | jq -C .

prepare-migrations: ## shared/schema/migrations/ → go/internal/migrate/migrations/ 동기화 (R9 단일 진실 유지)
	@mkdir -p go/internal/migrate/migrations
	@cp -r shared/schema/migrations/. go/internal/migrate/migrations/
```

`make install`이 첫 셋업의 모든 단계를 자동으로 처리:
1. Postgres 기동 (`make up`)
2. 8초 대기 (헬스체크 통과까지)
3. 마이그레이션 SQL 동기화 (`prepare-migrations`)
4. quantbot binary 빌드
5. `quantbot migrate up` (테이블 생성)
6. LaunchAgent plist 설치 + 활성화

→ **사용자 매일 운영 부담 = 0건. 첫 셋업도 `make install` 한 명령.**

### 11.3 `go/Makefile` `build` target 갱신

```makefile
build:  ## quantbot binary 빌드 (마이그레이션 SQL 임베드 포함)
	go build -o quantbot -ldflags="-X main.Version=$$(git describe --tags --always --dirty)" ./cmd/quantbot
```

`-ldflags="-X main.Version=..."`로 빌드 타임 버전 임베드. `quantbot version` 출력에 사용:

```bash
$ quantbot version
quantbot v0.1.0-phase1a-3-g0e45875 (built 2026-05-03 11:42:30)
```

(`git describe --tags --always --dirty` → 최근 태그 + 그 이후 커밋 수 + 짧은 SHA + dirty 표시)

**의존**: `make build`는 마이그레이션 임베드 위해 `prepare-migrations` 사전 호출 필요.

```makefile
build: prepare-migrations  ## 의존 명시
	go build -o quantbot -ldflags="-X main.Version=$$(git describe --tags --always --dirty)" ./cmd/quantbot
```

(`prepare-migrations`는 루트 Makefile에 정의되어 있으므로 go/Makefile에서 호출하려면 `cd .. && make prepare-migrations`. 또는 루트 Makefile에서만 build 노출 — 결정 시 단순한 쪽 선택. 본 spec에선 **루트 Makefile에 `make build` 추가**, `go/Makefile`은 그대로 두기:

루트 `Makefile`에 추가:
```makefile
build: prepare-migrations  ## quantbot binary 빌드
	$(MAKE) -C go build
```

이 방식이 더 깨끗 — `prepare-migrations`는 루트 책임, go/Makefile은 순수 Go 작업.

생성된 `go/quantbot` binary는 `.gitignore`에 추가.

## 12. TimescaleDB 버전 핀 고정

### 12.1 결정

`docker/docker-compose.yml`의 image를 다음으로 변경:

OLD:
```yaml
image: timescale/timescaledb:latest-pg16
```

NEW:
```yaml
image: timescale/timescaledb:2.18.0-pg16
```

### 12.2 이유

- `latest-pg16`은 floating tag → 빌드 재현성 깨짐
- `2.18.0-pg16` (2026년 초 안정) — 최신 stable
- 향후 업그레이드는 명시적 PR로

## 13. 디렉터리 구조 (변경 사항)

```
quant-bot/
├── shared/schema/
│   └── migrations/                            ← 신규 디렉터리
│       ├── 20260503000001_create_macro_series.sql
│       └── 20260503000002_create_runs.sql
├── deploy/                                    ← 신규 디렉터리
│   └── launchd/
│       └── com.quantbot.ingest-fred.plist
├── go/
│   ├── cmd/quantbot/                          ← 신규 디렉터리
│   │   └── main.go
│   ├── internal/
│   │   ├── cli/                               ← 신규 패키지
│   │   │   ├── bootstrap.go
│   │   │   ├── bootstrap_test.go
│   │   │   ├── ingest.go
│   │   │   ├── ingest_test.go
│   │   │   ├── status.go
│   │   │   ├── status_test.go
│   │   │   ├── migrate.go
│   │   │   └── version.go
│   │   ├── ingest/fred/                       ← 신규 패키지
│   │   │   ├── client.go
│   │   │   ├── client_test.go                  (httptest 사용 — 실제 FRED 호출 X)
│   │   │   ├── ingester.go
│   │   │   └── ingester_test.go
│   │   ├── migrate/                           ← 신규 패키지
│   │   │   ├── migrate.go
│   │   │   ├── migrate_test.go
│   │   │   └── migrations/                    ← gitignore (shared/schema/migrations/에서 복사됨)
│   │   ├── retry/                             ← 신규 패키지
│   │   │   ├── retry.go
│   │   │   └── retry_test.go
│   │   └── repo/                              ← 신규 패키지 (DB 액세스 레이어)
│   │       ├── macro_series.go
│   │       ├── macro_series_test.go
│   │       ├── runs.go
│   │       └── runs_test.go
│   └── (기존 config, db, logging, buildinfo 그대로)
├── config/config.toml                         ← `[retry]`, `[ingest]` 섹션 추가
├── config/config.example.toml                 ← 동일 키 추가
├── docker/docker-compose.yml                  ← TimescaleDB 버전 핀
└── (기존 그대로)
```

`.gitignore` 추가:
- `go/quantbot` (build 산출물 binary)
- `go/internal/migrate/migrations/` (shared/schema에서 복사된 사본)
- `logs/launchd-*.log` (launchd가 직접 쓰는 stdout/stderr — 우리 구조화 로그와 별개)

## 14. 완료 기준 (Acceptance Criteria)

- [ ] `pressly/goose@v3.21.0` 의존성 추가
- [ ] `shared/schema/migrations/20260503000001_enable_timescaledb.sql` 작성 (`CREATE EXTENSION`, `NO TRANSACTION`)
- [ ] `shared/schema/migrations/20260503000002_create_macro_series.sql` 작성 (TimescaleDB hypertable)
- [ ] `shared/schema/migrations/20260503000003_create_runs.sql` 작성
- [ ] `go/internal/migrate/` 패키지 — goose 래퍼 + embed.FS, 단위 테스트
- [ ] `Makefile`에 `prepare-migrations` target (`shared/schema/migrations/` → `go/internal/migrate/migrations/` 복사)
- [ ] `go/internal/retry/` 패키지 — TDD로 작성, 단위 테스트 (성공/재시도 후 성공/모든 시도 실패/ctx 취소)
- [ ] `go/internal/ingest/fred/` 패키지 — `FetchSeries` (`httptest` 모의 서버로 단위 테스트), `Run` 통합 테스트(`RUN_INTEGRATION=1` 가드)
- [ ] `go/internal/repo/` 패키지 — `macro_series` insert/select, `runs` insert/update CRUD, 통합 테스트
- [ ] `go/internal/cli/` 패키지 — bootstrap + ingest/status/migrate/version subcommand, 단위 테스트
- [ ] Bootstrap 단위 테스트: `requireMigrated=true`인데 미적용 마이그레이션 있으면 `ErrMigrationsPending` 반환 검증
- [ ] Bootstrap 단위 테스트: `Close()` 호출 시 풀+로그 파일 둘 다 닫힘 검증
- [ ] `go/cmd/quantbot/main.go` — subcommand 라우터
- [ ] `config/config.toml` + `config.example.toml`에 `[retry]`, `[ingest]` 섹션 추가
- [ ] `config.validate`에 retry/ingest 검증 규칙 추가, 단위 테스트 추가
- [ ] `docker/docker-compose.yml` TimescaleDB 버전 `2.18.0-pg16`으로 핀
- [ ] `deploy/launchd/com.quantbot.ingest-fred.plist` 템플릿 작성
- [ ] 루트 `Makefile`에 `install`/`uninstall`/`logs`/`prepare-migrations` target 추가
- [ ] `go/Makefile`의 `build` target이 `./cmd/quantbot`을 `go/quantbot`으로 빌드
- [ ] `.gitignore` 추가 (`go/quantbot`, `go/internal/migrate/migrations/`, `logs/launchd-*.log`)
- [ ] 통합 테스트: `make up && make test-integration` → FRED 모의 서버 사용한 ingest 사이클 검증 (실제 FRED 호출 X — API key 필요해서 CI 어려움)
- [ ] R14를 `docs/ARCHITECTURE.md` R 요약 표에 추가
- [ ] CLAUDE.md R 요약 표에 R14 한 줄 요약 추가
- [ ] `docs/STATUS.md` Phase 1b-A 추가 + 완료 표시
- [ ] `docs/ROADMAP.md`에서 Phase 1b를 1b-A·1b-B로 분리

## 14.1 사용자 검증 시나리오 (자동 acceptance 외)

자동 acceptance 다 통과한 후, 사용자가 본인 환경에서 직접 검증:

1. `make install` 실행 (첫 셋업)
2. `launchctl start com.quantbot.ingest-fred` (다음 정시까지 안 기다리고 즉시 실행)
3. 약 30초 대기 (FRED API 응답)
4. `./go/quantbot status` 실행 → 최근 run 1건 ✅, 4개 시리즈 행 수 출력
5. 다음날 22:00 이후 다시 `quantbot status` → 새 run 1건 추가, 시리즈 행 수 +1~2 (영업일 기준)

이 시나리오는 사용자 실환경에서만 가능 (실제 FRED API key 필요). 자동 acceptance에 포함 X.

## 15. 미결정 사항

- 통합 테스트에서 FRED API key 필요 여부 — 모의 서버(`httptest`)로 우회 결정. 실제 FRED 호출은 사용자 수동 검증 시점에만.
- `quantbot status`의 색상 처리 — `fatih/color` 같은 라이브러리 vs ANSI 직접. 단순 ANSI escape sequence로 시작 (의존성 0).
- LaunchAgent 시간대 — `StartCalendarInterval`는 시스템 로컬 타임존 사용. 사용자 시스템이 KST인지 확인 안 됨. 일단 22:00 로컬 사용, 사용자 시스템 시간대가 KST 아니면 plist 직접 수정.
- 통합 테스트의 `goose Up` 실행 방식 — `make test-integration`이 매 테스트마다 fresh DB 만드는 게 좋음. testcontainers-go 도입할지 단순 docker 재시작인지 → 구현 시 결정.

## 16. 검토 이력

| 날짜 | 검토 종류 | 결과 |
|------|----------|------|
| 2026-05-03 | 1차 자체 검토 | Critical 1(C1), Important 7(I1·I2·I3·I4·I6·I7·I8), Minor 3(M2·M3·M4) 식별 → 모두 인라인 패치. 주요 변경: §6.4 Bootstrap에 `requireMigrated` 플래그 + Close 책임 명시(풀+로그), §7.5 INSERT 구현 디테일·§7.6 동시 호출 시나리오 신설, §8.3 IsRetryable 정책(4xx/5xx/network 분류), §9.5 트랜잭션 보장·§9.6 단위 테스트 fixture 전략, §10.3 다음 예정 시각 출처·§10.4 비정상 종료 표시, §11.2 install이 마이그레이션·빌드까지 자동 처리(사용자 첫 셋업도 1명령), §14.1 사용자 검증 시나리오 별도 섹션 분리. |
| 2026-05-03 | 2차 자체 검토 (1차 Critical 발견 → 자동 진행) | Critical 0, Important 3(I-2-1·I-2-2·I-2-3), Minor 2(M-2-1·M-2-2) 식별 → 모두 인라인 패치. 주요 변경: §11.2 `make install`의 `sleep 8` → `docker compose up --wait` (healthcheck polling 안전), §5.2 마이그레이션 분리(timescaledb extension은 `NO TRANSACTION` 별도 파일) + §5.3 파일명 003으로 갱신, §14 acceptance에 Bootstrap requireMigrated/Close 단위 테스트 명시, §11.3 `quantbot version` 출력 형식 + ldflags 빌드 정보 임베드 + `make build` 의존 정리(루트에서). 한계 효용 체감 (Important 7→3 감소). 3차 자동 진행 불필요. |
