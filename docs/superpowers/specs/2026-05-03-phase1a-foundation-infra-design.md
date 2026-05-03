---
title: Phase 1a — Foundation Infrastructure (Go-side)
date: 2026-05-03
status: Brainstorming approved (pending user spec review)
phase: Phase 1a — Foundation Infrastructure
scope_note: Go 인프라만. Python 인프라는 Phase 2 시작 시 동일 패턴으로 구현.
authors: yuhojin, Claude
---

# Phase 1a — Foundation Infrastructure (Go-side)

## 1. 목표

Phase 1b 데이터 인제스트가 시작되기 전에 Go 측 코드가 공통으로 사용할 **4개 기반 시스템**을 구축한다:

1. **설정 시스템** — TOML 기반 단일 진실 원천 (Go·Python 양쪽이 공유할 파일)
2. **로그 시스템 (Go)** — 구조화 JSON 로그 (Unix 타임스탬프)
3. **데이터베이스 연결 풀 (Go)** — pgxpool 기반 효율적 연결 재사용
4. **공통 에러 처리 패턴 (Go)** — `fmt.Errorf("%w", ...)` wrapping + sentinel error

Python 인프라(`config.py`·`db.py`·`log.py`·예외 계층)는 Phase 2 시작 시 동일 패턴 따라 구현. 그때까지 Python 사용처가 없어 미리 만들면 YAGNI 위반.

이 4개 없이 Phase 1b를 시작하면 각 코드가 자체적으로 비슷한 패턴을 다시 만들어 중복이 생긴다.

## 2. 범위 외 (Phase 1a에 포함하지 않음)

- 비즈니스 로직 (데이터 수집, 백테스트, 주문) — Phase 1b+
- CLI 명령어 구조 — Phase 1b 진입점이 생긴 후 자연스럽게 정의
- 메트릭/모니터링 endpoint — Phase 7 (외부 알림과 함께)
- 분산 로그 수집 (ELK 등) — 단일 로컬 봇이라 불필요
- 시크릿 매니저(Vault, AWS Secrets Manager) — 단일 인스턴스 + gitignore로 충분
- **Python 인프라 (config 로더·log·DB 풀·예외 계층)** — Phase 2 시작 시 Go 패턴 따라 동일 구조로 구현. 사용처 없는 시점에 미리 만들지 않음 (YAGNI).
- **재시도 config (`[retry]` 섹션) 및 재시도 로직** — 사용처(외부 API 호출)가 처음 생기는 Phase 1b에서 추가

## 3. 컨텍스트 (브레인스토밍 결정)

| # | 항목 | 결정 |
|---|------|------|
| 1 | 작업 범위 | B (config + 로그 + DB 풀 + 에러 패턴) |
| 2 | 비밀 정보 처리 | A (단일 TOML, gitignore) |
| 3 | 환경 분리 | A (단일 파일 + environment 필드) |
| 4 | 시간 표현 | 균형형 (외부 직렬화 Unix, DB TIMESTAMPTZ, 내부 언어 타입, 설정/파일명 ISO) |

## 4. 핵심 설계 결정 (Architecture Rules R11~R13 신규)

기존 R1~R10에 이어 추가. 모든 후속 작업은 이 룰을 위반하지 않아야 한다.

### R11. 설정은 단일 TOML 파일이 단일 진실 원천

- **Why**: 양 언어가 같은 비밀번호·API 키를 쓰는데 두 곳에 두면 어긋날 위험 (R6/R9와 같은 정신).
- **How**: `config/config.toml` (gitignore)이 유일. Go·Python 둘 다 같은 파일 읽음. 코드에 하드코딩 금지.
- **example 파일 강제**: `config/config.example.toml`을 항상 동기화. 새 키 추가 시 example에도 동시 추가. 미동기화는 PR 거부 사유.

### R12. 봇 시작 시 fail-fast 검증

- **fail-fast란**: 잘못된 상태를 발견하면 즉시 종료하고 명확한 에러 메시지를 남기는 패턴. "조용히 잘못된 상태로 계속 진행"의 반대.
- **Why**: 잘못된 config·DB 연결 실패가 Phase 1 한참 후 발견되면 디버깅 비용 ↑
- **How**: 시작 시퀀스에서 다음 순서 강제:
  1. config 로드
  2. config 스키마 검증 (필수 필드, 값 범위, enum)
  3. DB 풀 생성 + `SELECT 1` 헬스체크
  4. 위 모두 통과 후에만 비즈니스 로직 진입
- 어느 단계든 실패 시 즉시 종료 + critical 로그.

### R13. 시간 표현 컨벤션

- **Why**: 시스템 전반에서 시간 형식 일관성 + 자리별 적합성 균형
- **How**:
  | 자리 | 표현 |
  |------|------|
  | 로그 JSON `time` 필드 | Unix 타임스탬프 (밀리초 단위, 예: `1746260581.123`) |
  | DB 컬럼 | TIMESTAMPTZ (Postgres 표준) |
  | 코드 내부 변수 | 언어 표준 타입 (Go `time.Time`, Python `datetime`) |
  | 설정 파일 (config.toml) | ISO 8601 (`"2026-05-03"`) |
  | 파일·디렉터리 이름 | ISO 날짜 (`app-2026-05-03.log`) |
  | DB→외부 직렬화 경계 | TIMESTAMPTZ → Unix 변환 |

## 5. 컴포넌트 1: 설정 시스템 (Config)

### 5.1 파일 구조

```
config/
  config.toml         # 실제 설정. gitignore.
  config.example.toml # 템플릿. 커밋 OK. 모든 키를 placeholder 값으로.
```

### 5.2 TOML 스키마 (초기 버전)

```toml
[general]
environment = "paper"          # paper / live / dev / test
log_level = "info"             # debug / info / warn / error

[database]
host = "localhost"
port = 5432
name = "quantbot"
user = "quantbot"
password = "REPLACE_ME"
pool_min = 2
pool_max = 10

[alpaca]
api_key = "REPLACE_ME"
api_secret = "REPLACE_ME"
paper = true                   # true=가짜 계좌, false=실거래 (Phase 9+)
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = "REPLACE_ME"         # FRED 무료 키, 발급 필요

[logging]
file_dir = "logs"              # 로그 파일 디렉터리 (날짜별 회전)
include_caller = false         # caller(소스파일:라인) 필드 포함 여부 (성능 영향 ↓)
```

`[kis]`, `[notification]`, `[retry]`(Phase 1b 외부 API 호출 시) 등은 해당 Phase에서 추가.

### 5.3 검증 규칙 (시작 시 강제)

- 필수 필드 누락 → 종료
- `general.environment` ∈ {paper, live, dev, test}
- `general.log_level` ∈ {debug, info, warn, error}
- `database.port` 1~65535
- `database.pool_min` ≤ `database.pool_max`
- `database.pool_min` ≥ 1
- `paper`/`live` 환경에서 API key·password 비어있으면 종료
- `dev`/`test` 환경에서는 경고만 (개발 편의)
- (Phase 1b에서 `[retry]` 추가 시: max_attempts ≥ 1, backoff_initial_ms ≥ 1, backoff_multiplier ≥ 1.0 검증 추가)

### 5.4 라이브러리

| 언어 | 라이브러리 | 최소 버전 | 이유 |
|------|----------|----------|------|
| Go | `github.com/BurntSushi/toml` | Go 1.22+ | struct tag 매핑, 가장 안정적 |
| Python (Phase 2 시) | 표준 `tomllib` + `pydantic` v2 | Python 3.11+ | tomllib 표준, pydantic은 타입·값 검증 |

### 5.5 위치

| 자산 | 경로 | Phase |
|------|------|-------|
| TOML 파일 | `config/config.toml`, `config/config.example.toml` | 1a |
| Go 로더 | `go/internal/config/config.go`, `config_test.go` | 1a |
| Python 로더 | `research/src/quant_research/config.py`, `tests/test_config.py` | **Phase 2** |

### 5.6 사용 패턴 (Go)

봇 시작 시 한 번 로드 → 객체를 함수 인자로 전달.

```go
// Go
cfg, err := config.Load("config/config.toml")
if err != nil { /* 종료 */ }
// cfg를 인자로 전달
```

(Python 로더는 Phase 2에서 동일 패턴으로 추가)

**전역 변수 패턴 금지 이유**:
- 테스트에서 다른 config로 갈아끼우기 어려움 (mock/override 복잡)
- 함수 시그니처만 봐도 어떤 의존성이 있는지 명확 (암묵적 전역 의존 X)
- (R2와는 무관 — config는 운영 상태가 아닌 정적 설정)

### 5.7 테스트 픽스처 (R10 빌드 독립성과 양립)

단위 테스트는 실제 `config/config.toml`에 의존하지 않는다. 각 언어 테스트 디렉터리에 fixture 파일 사용:

| 언어 | 픽스처 위치 | Phase |
|------|------------|-------|
| Go | `go/internal/config/testdata/valid.toml`, `invalid.toml` 등 (Go 표준 `testdata/`) | 1a |
| Python | `research/tests/fixtures/config_valid.toml`, `config_invalid.toml` | Phase 2 |

**원칙**: 단위 테스트는 fixture만 사용 → `cd go && make test`가 `config/config.toml` 없이도 통과 (R10).

**통합 테스트 분류 + Make target 신규**:

| 명령 | 정의 위치 | 동작 |
|------|----------|------|
| `make test` | `go/Makefile`, `research/Makefile` | 단위 테스트만. fixture만 사용. 외부 의존 X |
| `make test-integration` | `go/Makefile`, `research/Makefile` (신규) | `RUN_INTEGRATION=1` 환경변수 셋업 후 통합 테스트 (실제 Postgres + 실제 config.toml 필요) |
| 루트 `make test` | 루트 `Makefile` | 양쪽 단위 테스트만. 통합은 별도. |
| 루트 `make test-integration` (신규) | 루트 `Makefile` | go·research 양쪽 통합 테스트 일괄 호출 wrapper |

통합 테스트는 CI에서 별도 job, 로컬에선 명시적으로 `make test-integration` 호출 시에만 실행.

## 6. 컴포넌트 2: 로그 시스템

### 6.1 형식 (Structured JSON, NDJSON)

```
{"time":1746260581.123,"level":"info","msg":"FRED 수집 시작","series":"T10Y2Y","environment":"paper"}
{"time":1746260582.456,"level":"info","msg":"수집 완료","series":"T10Y2Y","rows":252,"duration_ms":1234}
{"time":1746260585.789,"level":"error","msg":"DB 연결 실패","attempt":3,"err":"timeout"}
```

`time` 필드는 **Unix 밀리초 단위** (R13).

### 6.2 출력

- **stderr**: 실시간 모니터링
- **파일**: `logs/app-YYYY-MM-DD.log` (날짜별 자동 회전, ISO 형식)
- `logs/` 디렉터리는 `.gitignore` (이미 포함)

### 6.3 라이브러리

| 언어 | 라이브러리 | 비고 |
|------|----------|------|
| Go | 표준 `log/slog` (1.21+) | JSON 핸들러 내장, 외부 의존 없음 |
| Python (Phase 2 시) | `structlog` | 구조화 로그 사실상 표준, JSON 출력 설정 |

### 6.4 공통 필드 (모든 로그에 자동 첨부)

- `time` (Unix 밀리초)
- `level` (debug/info/warn/error)
- `msg`
- `environment` (config의 environment 값)
- `caller` (소스 파일:라인) — `[logging].include_caller=true`일 때만. 기본값 false (성능)

### 6.5 위치

| 자산 | 경로 | Phase |
|------|------|-------|
| Go | `go/internal/logging/logger.go`, `logger_test.go` | 1a |
| Python | `research/src/quant_research/log.py`, `tests/test_log.py` | **Phase 2** |

(Python에서 `quant_research.log`로 쓰면 stdlib `logging`과 충돌 없음 — 패키지 prefix가 분리.)

### 6.6 시간 측정 (monotonic clock 강제)

`duration_ms` 같은 경과 시간 필드는 wall clock(`time.Now()`) 차이로 계산하면 NTP 동기화 시 음수 가능. **monotonic clock 강제**:

| 언어 | 사용 함수 | 비고 |
|------|---------|------|
| Go | `time.Since(start)` | 자동으로 monotonic 사용 |
| Python (Phase 2 시) | `time.perf_counter()` | wall clock과 분리된 monotonic |

`time` 필드(절대 시각)는 그대로 wall clock + Unix 변환. monotonic은 경과 시간 계산용.

## 7. 컴포넌트 3: 데이터베이스 연결 풀

### 7.1 라이브러리

| 언어 | 라이브러리 | 이유 |
|------|----------|------|
| Go | `github.com/jackc/pgx/v5/pgxpool` | 가장 빠르고 안정. PostgreSQL 전용 |
| Python (Phase 2 시) | `psycopg[pool]` (3.x) | psycopg3 내장 풀. SQLAlchemy 의존 없음 |

### 7.2 풀 설정 (config 기반)

`[database]` 섹션의 `pool_min`·`pool_max` 사용. 기본값: min 2, max 10. Phase 1에서 충분.

### 7.3 헬스체크 (R12 fail-fast)

```
config 로드 → DB 풀 생성 → SELECT 1 핑 (1초 timeout)
                            ├─ 통과 → 비즈니스 로직 진입
                            └─ 실패 → 종료 + critical 로그
```

**timeout = 1초**: 로컬 Docker Postgres는 보통 100ms 이내 응답. 1초가 충분하고 fail-fast 정신과 일치 (5초는 너무 김).

### 7.4 위치 (생성자 함수: Go `db.NewPool(ctx, cfg.Database) (*pgxpool.Pool, error)`)

| 자산 | 경로 | Phase |
|------|------|-------|
| Go | `go/internal/db/pool.go`, `pool_test.go` | 1a |
| Python | `research/src/quant_research/db.py`, `tests/test_db.py` | **Phase 2** |

### 7.5 사용 패턴

config와 마찬가지로 풀 객체를 함수 인자로 전달. 전역 변수 금지 (5.6과 동일 이유).

**Graceful shutdown (필수)**: 풀 생성 직후 종료 시점 등록.

```go
// Go
pool, err := db.NewPool(ctx, cfg.Database)
if err != nil { /* 종료 */ }
defer pool.Close()  // 종료 시 모든 연결 정리
```

(Python 사용 패턴은 Phase 2에서 동일 정신으로 추가)

종료 처리 누락 시 Postgres 측에 stale 연결 누적 → 다음 시작 시 max_connections 도달 위험.

### 7.6 테스트 전략

- **단위 테스트**: `pgxpool.Pool`은 인터페이스가 아닌 concrete struct라 직접 mock 불가능. 순수 함수(`BuildDSN` 등)만 단위 테스트로 검증. `pgxmock`은 `pgx.Conn` 수준만 mock 가능하므로 풀 lifecycle 검증엔 부적합.
- **통합 테스트**: `NewPool`의 실제 동작(헬스체크, 연결 풀 생성)은 build tag `integration` + `RUN_INTEGRATION=1` 가드 하에 Docker Postgres로 검증. 이게 pgxpool 계열의 정직한 테스트 전략.
- (Python은 Phase 2에서 동일 정신: 순수 함수만 단위, 풀 lifecycle은 통합)

## 8. 컴포넌트 4: 공통 에러 처리 패턴

### 8.1 Go: 표준 error wrapping

```go
if err != nil {
    return fmt.Errorf("FRED 수집: %w", err)
}
```

호출 측에서 `errors.Is`, `errors.As`로 원본 에러 검사 가능.

### 8.2 Python: 도메인 예외 계층 (Phase 2에서 도입)

Python 인프라가 Phase 2로 이연되므로 예외 계층도 그때 만든다. **사용 시점에 추가** 원칙 (YAGNI):

| 클래스 | 도입 Phase | 사용처 |
|--------|----------|--------|
| `QuantBotError` (base) | Phase 2 | 모든 봇 예외 최상위 |
| `ConfigError` | Phase 2 | config 로더 |
| `DBConnectionError` | Phase 2 | DB 풀 |
| `DataIngestError` | Phase 1b/2 | 데이터 수집기 (Phase 1b는 Go라 적용 X. Phase 2 Python feature compute에서 사용) |
| `StrategyError` | Phase 3 | 전략 실행 |

위 표는 spec 단계 명시. 모든 클래스를 Phase 2에서 한꺼번에 정의해도 OK (단일 파일이라 비용 거의 없음).

### 8.3 운영 경로 에러 정책

| 상황 | 정책 |
|------|------|
| 모든 에러 발생 | 구조화 로그 (level: error 또는 warn) |
| 일시 장애 (네트워크, API rate limit) | (Phase 1b에서 `[retry]` config 도입 후) 백오프 재시도. 누적 실패 시 종료 |
| 영구 실패 (DB 영구 다운, config 잘못, API 키 만료) | 즉시 종료 + critical 로그 |

R2 정신("불일치 시 알림 + 자동 정지")의 일반 적용.

### 8.4 Go sentinel error 패턴 (선택)

같은 패키지에서 자주 쓰는 에러는 sentinel로 정의 (Go 관용구):

```go
// go/internal/config/config.go
var (
    ErrConfigMissing  = errors.New("config 파일을 찾을 수 없음")
    ErrConfigInvalid  = errors.New("config 검증 실패")
)
```

호출 측: `if errors.Is(err, config.ErrConfigInvalid) { ... }`.

### 8.5 위치

- Go: 도메인별 에러 타입은 각 패키지에서 정의 (sentinel + `fmt.Errorf` wrapping). 공통 패키지 불필요.
- Python: `research/src/quant_research/errors.py` (단일 파일, **Phase 2**)

## 9. 디렉터리 구조 (변경 사항)

```
quant-bot/
├── config/                          ← 신규 디렉터리 (Phase 1a)
│   ├── README.md                    ← 신규 (셋업 안내)
│   ├── config.toml                  ← gitignore
│   └── config.example.toml          ← 커밋
├── go/                              ← Phase 1a 작업
│   └── internal/
│       ├── config/
│       │   ├── config.go            ← 신규
│       │   ├── config_test.go       ← 신규
│       │   └── testdata/
│       │       ├── valid.toml       ← 신규 (단위 테스트용)
│       │       └── invalid.toml     ← 신규
│       ├── db/
│       │   ├── pool.go              ← 신규
│       │   └── pool_test.go         ← 신규
│       └── logging/
│           ├── logger.go            ← 신규
│           └── logger_test.go       ← 신규
├── research/                        ← Phase 2 작업 (Phase 1a 미작업)
│   ├── src/quant_research/
│   │   ├── config.py                (Phase 2)
│   │   ├── db.py                    (Phase 2)
│   │   ├── log.py                   (Phase 2)
│   │   └── errors.py                (Phase 2)
│   └── tests/
│       ├── fixtures/                (Phase 2)
│       ├── test_config.py           (Phase 2)
│       ├── test_db.py               (Phase 2)
│       └── test_log.py              (Phase 2)
├── logs/                            ← 런타임 자동 생성. .gitignore 이미 포함
└── (기존 그대로)
```

`.gitignore` 추가 필요 항목:
- `config/config.toml`

`config/README.md` 내용 (요약):
- "이 디렉터리는 봇 설정 파일을 보관합니다"
- "처음 셋업 시: `cp config.example.toml config.toml` 후 실제 값 채우기"
- "`config.toml`은 절대 git에 커밋되지 않습니다 (.gitignore 적용)"
- "각 키의 의미는 spec(`docs/superpowers/specs/2026-05-03-pre-execution-foundation-design.md`) §5.2 참조"

## 10. 완료 기준 (Acceptance Criteria — Phase 1a)

- [ ] `config/config.example.toml` 작성, 모든 키 placeholder 값
- [ ] `config/config.toml` 작성 (개발용 값), `.gitignore`에 추가 확인
- [ ] `config/README.md` 작성 (셋업 안내)
- [ ] Go: config 로드·검증·실패 시 종료 동작 (단위 테스트 통과, fixture 사용)
- [ ] Go: `go/Makefile`에 `test-integration` target 신규 추가, `RUN_INTEGRATION=1` 가드
- [ ] 루트 `Makefile`에 `test-integration` wrapper target 신규 추가 (현재는 go 측만 호출)
- [ ] Go: structured JSON 로그 출력 (stderr + 파일), `time` 필드 Unix 밀리초, `duration_ms`는 monotonic clock (`time.Since`)
- [ ] Go: pgxpool 생성, `SELECT 1` 핑 통과 (1초 timeout), 풀 min/max config 반영, `defer pool.Close()` 적용
- [ ] Go: sentinel error + `fmt.Errorf("%w", ...)` 패턴 적용된 예제 함수 (테스트로 wrapping·`errors.Is` 검증)
- [ ] R11~R13을 `docs/ARCHITECTURE.md` R 요약 표에 추가
- [ ] CLAUDE.md R 요약 표에 R11~R13 한 줄 요약 추가
- [ ] `docs/STATUS.md`에 "Phase 1a — Foundation Infrastructure" 추가, 최근 변경 이력 갱신
- [ ] `docs/ROADMAP.md`에서 Phase 1을 1a (Foundation) + 1b (Data Ingest)로 분리
- [ ] 단위 테스트는 `config/config.toml` 의존 X 확인 (R10 빌드 독립성 유지)
- [ ] (Phase 2로 이연) Python config·log·db·errors — 본 spec의 동일 패턴 따라 구현

## 11. 미결정 사항

- 로그 파일 회전 라이브러리 — Go의 `lumberjack` vs 표준 라이브러리만 → 구현 시 결정 (시작은 단순한 일별 파일 분리로)
- example 동기화 자동화 (pre-commit hook으로 config.toml 키가 example에도 있는지 자동 검증) → Phase 1a 구현 후 도입 검토
- 타임존 정책 (US 시장 데이터 + 한국 사용자) → Phase 1b 데이터 수집 spec에서 결정
- 테스트 환경 로그 출력 (stderr 직출력 vs discard handler) → Phase 1a 구현 시 결정
- (Phase 1b 도입 시) 재시도 백오프 알고리즘 — 기본 exponential 1s → 2s → 4s
- (Phase 2 도입 시) pydantic 모델을 immutable로 할지

## 12. 검토 이력

| 날짜 | 검토 종류 | 결과 |
|------|----------|------|
| 2026-05-03 | 1차 자체 검토 | Critical 0, Important 4(I1·I2·I3·I4), Minor 3(M1·M2·M3) 식별 → 모두 인라인 패치. 주요 변경: R12에 fail-fast 정의 추가, `[logging]` config 섹션 추가, §5.6 R2 인용 제거 + 진짜 이유 명시, §5.7 신설(테스트 픽스처 + R10 양립), §7.6 신설(DB 풀 테스트 전략), §8.4 신설(Go sentinel error), 디렉터리 구조에 README + testdata + fixtures 추가. |
| 2026-05-03 | 2차 자체 검토 (1차에서 Important 다수 → 자동 진행) | Critical 0, Important 5(I-2-1~5), Minor 2(M-2-1~2) 식별 → 모두 인라인 패치. 주요 변경: `[retry]` config 섹션 추가(재시도 N 명시), §6.4 caller 필드 조건을 include_caller config로 통일, §7.5 graceful shutdown 패턴 명시(defer pool.Close), §5.7 통합 테스트 make target 명시화, §5.4에 Python 3.11+ 최소 버전 명시, §10 acceptance 항목 정리(TDD 표현 중복 제거 + graceful shutdown 추가 + integration target 추가). |
| 2026-05-03 | 3차 자체 검토 (2차 5건 → 한계 효용 미체감, 자동 진행) | Critical 0, Important 2(I-3-1~2), Minor 2(M-3-1~2) 식별 → 모두 인라인 패치. 주요 변경: §5.3 검증 규칙에 `[retry]` 항목 추가(max_attempts·backoff 검증), §7.5 Python 예제를 sync로 통일 + sync/async 결정 노트, §5.7과 §10에 루트 Makefile의 `test-integration` wrapper 추가. 한계 효용 체감 도달 (Important 4→5→2 감소). 4차 자동 진행 불필요. |
| 2026-05-03 | 4차 자체 검토 (사용자 요청, 오너 시각) | Critical 0, Important 6(I-4-1~6), Minor 3(M-4-1~3) 식별. 오너 결정 적용: (1) Phase 명명 "0.5" → "1a/1b" 분리, (2) Python 인프라 Phase 2로 이연 (YAGNI), (3) `[retry]` config·Python 도메인 예외 모두 사용 시점으로 이연, (4) health check timeout 5s → 1s, (5) monotonic clock 강제 명시, (6) Python 파일명 `logging_setup.py` → `log.py` (Phase 2). spec 파일명 rename: `pre-execution-foundation` → `phase1a-foundation-infra`. acceptance criteria 대폭 축소 (Python 항목 deferred). 미결정 사항 정리. |
