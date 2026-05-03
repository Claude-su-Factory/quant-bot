# Phase 1a — Foundation Infrastructure (Go) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Code task의 implementer는 반드시 superpowers:test-driven-development 스킬을 호출. 코드 리뷰는 superpowers:requesting-code-review 형식 사용. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Phase 1b 데이터 인제스트 시작 전 Go 측 공통 인프라 4종(설정 로더, 로깅, DB 풀, 에러 패턴) + 인프라 룰 R11~R13 도입.

**Architecture:** TOML 설정을 단일 진실 원천으로 두고, Go 측에 `BurntSushi/toml`로 로드·검증 → `log/slog` 기반 JSON 로거 → `pgx/v5/pgxpool` 기반 DB 풀 → sentinel error + `fmt.Errorf("%w", ...)` wrapping 패턴. 모든 컴포넌트는 fail-fast 검증을 거쳐 `main`에서 의존성 주입으로 사용.

**Tech Stack:** Go 1.22+, BurntSushi/toml v1.4+, log/slog (표준), jackc/pgx/v5/pgxpool, pgxmock/v3 (단위 테스트), Make.

**Reference Spec:** [`docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md`](../specs/2026-05-03-phase1a-foundation-infra-design.md)

---

## 작업 가정

- 작업 디렉터리: `/Users/yuhojin/Desktop/quant-bot`
- Phase 0 완료 상태 (git repo, go module `github.com/yuhojin/quant-bot/go` 초기화됨)
- Docker Postgres 기동 가능 (`make up`/`make down`/`make db-check` 동작)
- Go 1.22+, Python 3.12+, uv 모두 설치

---

## File Structure (이 plan으로 생성/수정될 파일)

**생성:**
- `config/config.toml` (gitignore)
- `config/config.example.toml`
- `config/README.md`
- `go/internal/config/config.go`
- `go/internal/config/config_test.go`
- `go/internal/config/testdata/valid.toml`
- `go/internal/config/testdata/missing_password.toml`
- `go/internal/config/testdata/invalid_environment.toml`
- `go/internal/config/testdata/invalid_pool_range.toml`
- `go/internal/logging/logger.go`
- `go/internal/logging/logger_test.go`
- `go/internal/db/pool.go`
- `go/internal/db/pool_test.go`
- `go/internal/db/pool_integration_test.go`

**수정:**
- `.gitignore` (config/config.toml 추가)
- `go/Makefile` (test-integration target 추가)
- `Makefile` 루트 (test-integration wrapper 추가)
- `go/go.mod`, `go/go.sum` (의존성 추가 자동)
- `docs/ARCHITECTURE.md` (R11~R13 표 추가)
- `CLAUDE.md` (R11~R13 표 추가)
- `docs/STATUS.md` (Phase 1a ✅ + 변경 이력)
- `docs/ROADMAP.md` (Phase 1a 제거, Phase 1b를 다음 작업으로)

---

## Task 1: 디렉터리 + .gitignore + config 템플릿 (doc-only)

**Files:**
- Create: `config/config.toml`, `config/config.example.toml`, `config/README.md`
- Modify: `.gitignore`

- [ ] **Step 1: config/ 디렉터리 생성**

```bash
cd /Users/yuhojin/Desktop/quant-bot && mkdir -p config
```

- [ ] **Step 2: `.gitignore`에 `config/config.toml` 추가**

`.gitignore` 파일에서 `# Project` 섹션 안에 새 줄 추가. 다음 패치 적용:

OLD:
```
# Project
.env
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
```

- [ ] **Step 3: `config/config.example.toml` 작성 (커밋 OK, placeholder 값)**

Create `config/config.example.toml` with EXACTLY:
```toml
[general]
environment = "dev"
log_level = "info"

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
paper = true
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = "REPLACE_ME"

[logging]
file_dir = "logs"
include_caller = false
```

- [ ] **Step 4: `config/config.toml` 작성 (gitignore, 개발용 값)**

Create `config/config.toml` with EXACTLY:
```toml
[general]
environment = "dev"
log_level = "debug"

[database]
host = "localhost"
port = 5432
name = "quantbot"
user = "quantbot"
password = "changeme"
pool_min = 2
pool_max = 10

[alpaca]
api_key = ""
api_secret = ""
paper = true
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = ""

[logging]
file_dir = "logs"
include_caller = true
```

(API 키는 빈 문자열. environment="dev"라 검증에서 경고만, 종료 X.)

- [ ] **Step 5: `config/README.md` 작성**

Create `config/README.md` with EXACTLY:
```markdown
# config/

봇 설정 파일이 위치한다.

## 처음 셋업

```bash
cp config.example.toml config.toml
# config.toml 열어 실제 값 채우기 (DB 비밀번호, API 키 등)
```

## 파일 안내

| 파일 | git 추적 | 설명 |
|------|---------|------|
| `config.example.toml` | ✅ 커밋 | 키 목록 템플릿. placeholder 값. 새 키 추가 시 함께 갱신 (R11) |
| `config.toml` | ❌ gitignore | 실제 값. 비밀 포함. 절대 커밋되지 않음 |
| `README.md` | ✅ 커밋 | 본 안내 |

## 키 의미

각 키의 의미·검증 규칙은 spec [`docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md`](../docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) §5.2~5.3 참조.

## 환경 (`general.environment`)

| 값 | 동작 |
|----|------|
| `dev` | 개발용. API 키 비어 있어도 경고만 |
| `test` | 테스트용. API 키 비어 있어도 경고만 |
| `paper` | 페이퍼 트레이딩. API 키 필수 |
| `live` | 실거래. API 키 필수 (R8 게이트 통과 후 사용) |
```

- [ ] **Step 6: 검증 — 파일 존재 + 추적 상태**

```bash
cd /Users/yuhojin/Desktop/quant-bot
ls config/config.toml config/config.example.toml config/README.md
git check-ignore config/config.toml && echo "config.toml is gitignored OK"
git status --short config/
```

Expected:
- 3개 파일 모두 출력
- `config.toml is gitignored OK` 출력
- `git status`에 `?? config/config.example.toml`, `?? config/README.md`만 보임 (config.toml은 untracked + ignored)

- [ ] **Step 7: 커밋**

```bash
git add .gitignore config/config.example.toml config/README.md
git commit -m "chore(config): add TOML config templates + gitignore actual config"
```

(Note: `config/config.toml`은 gitignore라 자동 제외됨)

---

## Task 2: Go config 로더 (TDD)

**Files:**
- Create: `go/internal/config/config.go`, `config_test.go`, `testdata/valid.toml`, `testdata/missing_password.toml`, `testdata/invalid_environment.toml`, `testdata/invalid_pool_range.toml`
- Modify: `go/go.mod`, `go/go.sum` (auto)

**구현자에게**: 이 task는 code task이므로 `superpowers:test-driven-development` 스킬을 반드시 호출하여 따른다. 사이클: 실패 테스트 작성 → 실행해 실패 확인 → 최소 구현 → 실행해 통과 → 커밋.

- [ ] **Step 1: BurntSushi/toml 의존성 추가**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go
go get github.com/BurntSushi/toml@v1.4.0
```

- [ ] **Step 2: 테스트 fixture 파일 작성 (먼저 만들어 놓아야 테스트 작성 가능)**

```bash
mkdir -p go/internal/config/testdata
```

Create `go/internal/config/testdata/valid.toml`:
```toml
[general]
environment = "paper"
log_level = "info"

[database]
host = "localhost"
port = 5432
name = "quantbot"
user = "quantbot"
password = "secret123"
pool_min = 2
pool_max = 10

[alpaca]
api_key = "AK_TEST"
api_secret = "AS_TEST"
paper = true
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = "FRED_TEST"

[logging]
file_dir = "logs"
include_caller = false
```

Create `go/internal/config/testdata/missing_password.toml`:
```toml
[general]
environment = "paper"
log_level = "info"

[database]
host = "localhost"
port = 5432
name = "quantbot"
user = "quantbot"
password = ""
pool_min = 2
pool_max = 10

[alpaca]
api_key = "AK_TEST"
api_secret = "AS_TEST"
paper = true
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = "FRED_TEST"

[logging]
file_dir = "logs"
include_caller = false
```

Create `go/internal/config/testdata/invalid_environment.toml`:
```toml
[general]
environment = "production"
log_level = "info"

[database]
host = "localhost"
port = 5432
name = "quantbot"
user = "quantbot"
password = "secret123"
pool_min = 2
pool_max = 10

[alpaca]
api_key = "AK_TEST"
api_secret = "AS_TEST"
paper = true
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = "FRED_TEST"

[logging]
file_dir = "logs"
include_caller = false
```

Create `go/internal/config/testdata/invalid_pool_range.toml`:
```toml
[general]
environment = "paper"
log_level = "info"

[database]
host = "localhost"
port = 5432
name = "quantbot"
user = "quantbot"
password = "secret123"
pool_min = 10
pool_max = 5

[alpaca]
api_key = "AK_TEST"
api_secret = "AS_TEST"
paper = true
base_url = "https://paper-api.alpaca.markets"

[fred]
api_key = "FRED_TEST"

[logging]
file_dir = "logs"
include_caller = false
```

- [ ] **Step 3: 첫 실패 테스트 — valid TOML 로드**

Create `go/internal/config/config_test.go`:
```go
package config

import (
	"errors"
	"testing"
)

func TestLoad_Valid(t *testing.T) {
	cfg, err := Load("testdata/valid.toml")
	if err != nil {
		t.Fatalf("정상 TOML인데 에러: %v", err)
	}
	if cfg.General.Environment != "paper" {
		t.Errorf("environment: 기대 paper, 실제 %q", cfg.General.Environment)
	}
	if cfg.Database.Password != "secret123" {
		t.Errorf("database.password 매핑 실패")
	}
	if cfg.Database.PoolMin != 2 || cfg.Database.PoolMax != 10 {
		t.Errorf("pool min/max 매핑 실패: min=%d max=%d", cfg.Database.PoolMin, cfg.Database.PoolMax)
	}
	if !cfg.Alpaca.Paper {
		t.Errorf("alpaca.paper 기대 true")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("testdata/does_not_exist.toml")
	if !errors.Is(err, ErrConfigMissing) {
		t.Errorf("ErrConfigMissing 기대, 실제 %v", err)
	}
}
```

- [ ] **Step 4: 테스트 실행해 실패 확인**

```bash
cd go && go test ./internal/config/... -v
```

Expected: 컴파일 에러 — `Load`, `ErrConfigMissing` 등 미정의.

- [ ] **Step 5: 최소 구현으로 컴파일 + 테스트 통과**

Create `go/internal/config/config.go`:
```go
// Package config는 TOML 기반 봇 설정을 로드·검증한다.
// 단일 진실 원천 룰(R11)에 따라 봇 시작 시 한 번만 로드되어야 한다.
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

var (
	// ErrConfigMissing은 config 파일을 찾을 수 없을 때 반환된다.
	ErrConfigMissing = errors.New("config 파일을 찾을 수 없음")
	// ErrConfigInvalid는 config 파싱·검증 실패 시 반환된다.
	ErrConfigInvalid = errors.New("config 검증 실패")
)

// Config는 봇 전체 설정의 최상위 구조다.
type Config struct {
	General  GeneralConfig  `toml:"general"`
	Database DatabaseConfig `toml:"database"`
	Alpaca   AlpacaConfig   `toml:"alpaca"`
	FRED     FREDConfig     `toml:"fred"`
	Logging  LoggingConfig  `toml:"logging"`
}

type GeneralConfig struct {
	Environment string `toml:"environment"`
	LogLevel    string `toml:"log_level"`
}

type DatabaseConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	Name     string `toml:"name"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	PoolMin  int    `toml:"pool_min"`
	PoolMax  int    `toml:"pool_max"`
}

type AlpacaConfig struct {
	APIKey    string `toml:"api_key"`
	APISecret string `toml:"api_secret"`
	Paper     bool   `toml:"paper"`
	BaseURL   string `toml:"base_url"`
}

type FREDConfig struct {
	APIKey string `toml:"api_key"`
}

type LoggingConfig struct {
	FileDir       string `toml:"file_dir"`
	IncludeCaller bool   `toml:"include_caller"`
}

// Load는 path의 TOML 파일을 읽고 검증한다.
// 실패 시 ErrConfigMissing 또는 ErrConfigInvalid로 wrap된 에러 반환.
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrConfigMissing, path)
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("%w: TOML 파싱 실패: %v", ErrConfigInvalid, err)
	}
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigInvalid, err)
	}
	return &cfg, nil
}

func validate(cfg *Config) error {
	// Step 6에서 추가
	return nil
}
```

- [ ] **Step 6: 테스트 실행해 통과 확인**

```bash
cd go && go test ./internal/config/... -v -run "TestLoad_Valid|TestLoad_MissingFile"
```

Expected: 두 테스트 모두 PASS.

- [ ] **Step 7: 다음 실패 테스트 — environment enum 검증**

`go/internal/config/config_test.go`에 다음 테스트 추가:
```go
func TestValidate_InvalidEnvironment(t *testing.T) {
	_, err := Load("testdata/invalid_environment.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대, 실제 %v", err)
	}
}

func TestValidate_InvalidPoolRange(t *testing.T) {
	_, err := Load("testdata/invalid_pool_range.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대, 실제 %v", err)
	}
}

func TestValidate_MissingPasswordInPaper(t *testing.T) {
	// environment=paper인데 password 비어있으면 에러
	_, err := Load("testdata/missing_password.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (paper에서 password 빈 값), 실제 %v", err)
	}
}
```

- [ ] **Step 8: 테스트 실행해 실패 확인**

```bash
cd go && go test ./internal/config/... -v -run "TestValidate"
```

Expected: 3개 테스트 모두 FAIL (validate가 빈 함수라 통과해버림).

- [ ] **Step 9: 검증 로직 구현**

`go/internal/config/config.go`의 `validate` 함수를 다음으로 교체:
```go
func validate(cfg *Config) error {
	// general
	switch cfg.General.Environment {
	case "paper", "live", "dev", "test":
	default:
		return fmt.Errorf("general.environment: paper/live/dev/test 중 하나여야 함, 실제 %q", cfg.General.Environment)
	}
	switch cfg.General.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("general.log_level: debug/info/warn/error 중 하나여야 함, 실제 %q", cfg.General.LogLevel)
	}

	// database
	if cfg.Database.Port < 1 || cfg.Database.Port > 65535 {
		return fmt.Errorf("database.port: 1~65535 범위 벗어남: %d", cfg.Database.Port)
	}
	if cfg.Database.PoolMin < 1 {
		return fmt.Errorf("database.pool_min: 1 이상이어야 함: %d", cfg.Database.PoolMin)
	}
	if cfg.Database.PoolMin > cfg.Database.PoolMax {
		return fmt.Errorf("database.pool_min(%d) > pool_max(%d)", cfg.Database.PoolMin, cfg.Database.PoolMax)
	}

	// 비밀 검증: paper/live 환경에서만 강제
	strict := cfg.General.Environment == "paper" || cfg.General.Environment == "live"
	if strict {
		if cfg.Database.Password == "" {
			return fmt.Errorf("database.password: %s 환경에서 빈 값 금지", cfg.General.Environment)
		}
		if cfg.Alpaca.APIKey == "" || cfg.Alpaca.APISecret == "" {
			return fmt.Errorf("alpaca 키: %s 환경에서 빈 값 금지", cfg.General.Environment)
		}
		if cfg.FRED.APIKey == "" {
			return fmt.Errorf("fred.api_key: %s 환경에서 빈 값 금지", cfg.General.Environment)
		}
	}

	return nil
}
```

- [ ] **Step 10: 테스트 실행해 통과 확인**

```bash
cd go && go test ./internal/config/... -v
```

Expected: 모든 테스트 PASS (Load_Valid + Load_MissingFile + 3 validate 테스트).

- [ ] **Step 11: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/config/ go/go.mod go/go.sum
git commit -m "feat(config): TOML config loader with fail-fast validation (R11/R12)"
```

---

## Task 3: Go 로거 (TDD)

**Files:**
- Create: `go/internal/logging/logger.go`, `logger_test.go`

**구현자에게**: TDD 강제. `superpowers:test-driven-development` 스킬 호출.

- [ ] **Step 1: 디렉터리 생성**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/go/internal/logging
```

- [ ] **Step 2: 첫 실패 테스트 — JSON 출력 + Unix 밀리초 time 필드**

Create `go/internal/logging/logger_test.go`:
```go
package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// 헬퍼: 메모리 버퍼로 로거 만들고 한 줄 로그 후 JSON 파싱
func captureLogJSON(t *testing.T, environment string, includeCaller bool, write func(l *Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, "info", environment, includeCaller)
	write(logger)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("로그 출력이 비어 있음")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("로그가 유효한 JSON이 아님: %v\n원문: %s", err, line)
	}
	return entry
}

func TestLogger_JSON_TimeIsUnixSecondsFloat(t *testing.T) {
	entry := captureLogJSON(t, "paper", false, func(l *Logger) {
		l.Info("hello")
	})
	tv, ok := entry["time"]
	if !ok {
		t.Fatalf("time 필드 없음")
	}
	switch tv.(type) {
	case float64:
		// OK — Unix 초 + 소수점 ms
	default:
		t.Errorf("time 필드 타입 기대 float64 (Unix 초.밀리초), 실제 %T", tv)
	}
}

func TestLogger_JSON_EnvironmentField(t *testing.T) {
	entry := captureLogJSON(t, "paper", false, func(l *Logger) {
		l.Info("hello")
	})
	if env, _ := entry["environment"].(string); env != "paper" {
		t.Errorf("environment 필드 기대 paper, 실제 %v", entry["environment"])
	}
}

func TestLogger_JSON_MsgField(t *testing.T) {
	entry := captureLogJSON(t, "dev", false, func(l *Logger) {
		l.Info("greetings")
	})
	if msg, _ := entry["msg"].(string); msg != "greetings" {
		t.Errorf("msg 필드 기대 greetings, 실제 %v", entry["msg"])
	}
}

func TestLogger_IncludeCaller_True(t *testing.T) {
	entry := captureLogJSON(t, "dev", true, func(l *Logger) {
		l.Info("with caller")
	})
	if _, ok := entry["caller"]; !ok {
		t.Errorf("include_caller=true이지만 caller 필드 없음")
	}
}

func TestLogger_IncludeCaller_False(t *testing.T) {
	entry := captureLogJSON(t, "dev", false, func(l *Logger) {
		l.Info("no caller")
	})
	if _, ok := entry["caller"]; ok {
		t.Errorf("include_caller=false인데 caller 필드 있음")
	}
}
```

- [ ] **Step 3: 테스트 실행해 실패 확인**

```bash
cd go && go test ./internal/logging/... -v
```

Expected: 컴파일 에러 — `Logger`, `NewWithWriter` 미정의.

- [ ] **Step 4: 최소 구현**

Create `go/internal/logging/logger.go`:
```go
// Package logging은 구조화 JSON 로거를 제공한다.
// 시간은 R13 컨벤션에 따라 Unix 초.밀리초 (float)로 직렬화.
// 모든 로그에 environment 필드 자동 첨부.
package logging

import (
	"io"
	"log/slog"
	"os"
	"time"
)

// Logger는 slog 기반 구조화 로거.
type Logger = slog.Logger

// NewWithWriter는 임의의 io.Writer로 출력하는 로거를 만든다 (테스트용).
// 일반 사용은 New를 사용.
func NewWithWriter(w io.Writer, logLevel, environment string, includeCaller bool) *Logger {
	level := parseLevel(logLevel)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: includeCaller,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// time 필드: Unix 초.밀리초 float
			if a.Key == slog.TimeKey {
				t := a.Value.Time()
				return slog.Float64(slog.TimeKey, float64(t.UnixMilli())/1000.0)
			}
			// source 필드 → caller로 이름 변경
			if a.Key == slog.SourceKey {
				src, ok := a.Value.Any().(*slog.Source)
				if ok && src != nil {
					return slog.String("caller", shortPath(src.File)+":"+itoa(src.Line))
				}
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(w, opts)
	logger := slog.New(handler).With("environment", environment)
	return logger
}

// New는 stderr + 로그 파일 양쪽으로 출력하는 로거 + close 함수를 만든다.
func New(fileDir, logLevel, environment string, includeCaller bool) (*Logger, func() error, error) {
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return nil, nil, err
	}
	fileName := fileDir + "/app-" + time.Now().Format("2006-01-02") + ".log"
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	mw := io.MultiWriter(os.Stderr, f)
	logger := NewWithWriter(mw, logLevel, environment, includeCaller)
	return logger, f.Close, nil
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func shortPath(p string) string {
	// 마지막 경로 세그먼트 두 개만 남김 (가독성)
	idx := -1
	count := 0
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			count++
			if count == 2 {
				idx = i + 1
				break
			}
		}
	}
	if idx < 0 {
		return p
	}
	return p[idx:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
```

- [ ] **Step 5: 테스트 실행해 통과 확인**

```bash
cd go && go test ./internal/logging/... -v
```

Expected: 5개 테스트 모두 PASS.

- [ ] **Step 6: 추가 테스트 — File output**

`go/internal/logging/logger_test.go`에 추가:
```go
import "os"

func TestNew_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	logger, closeFn, err := New(dir, "info", "paper", false)
	if err != nil {
		t.Fatalf("New 실패: %v", err)
	}
	defer closeFn()

	logger.Info("file test")

	// 파일 찾기 (날짜별 회전)
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("로그 파일 1개 기대, 실제 %d개", len(entries))
	}
	data, _ := os.ReadFile(dir + "/" + entries[0].Name())
	if !bytes.Contains(data, []byte(`"msg":"file test"`)) {
		t.Errorf("파일에 메시지 기록 X. 내용:\n%s", data)
	}
}
```

- [ ] **Step 7: 테스트 실행해 통과 확인**

```bash
cd go && go test ./internal/logging/... -v
```

Expected: 6개 테스트 모두 PASS.

- [ ] **Step 8: monotonic clock 사용 안내 주석 추가**

`go/internal/logging/logger.go` 최하단(또는 package doc 안)에 다음 주석 추가:

```go
// 경과 시간(예: API 호출 소요 시간) 기록 시 wall clock 차이가 아닌 monotonic clock을 써야 한다.
// Go에선 time.Since(start)가 자동으로 monotonic을 사용하므로 다음 패턴 권장:
//
//   start := time.Now()
//   doWork()
//   logger.Info("done", "duration_ms", time.Since(start).Milliseconds())
//
// time.Now() - time.Now() 같은 빼기는 NTP 동기화 시 음수 가능. R13 컨벤션 참조.
```

(Phase 1a엔 실제 duration 측정 코드가 없지만 Phase 1b 외부 API 호출 시 이 패턴 강제 — 주석으로 미래 구현자에게 안내.)

- [ ] **Step 9: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/logging/
git commit -m "feat(logging): structured JSON logger with Unix-ms time + file output (R13)"
```

---

## Task 4: Go DB 풀 (TDD + 통합 테스트)

**Files:**
- Create: `go/internal/db/pool.go`, `pool_test.go`, `pool_integration_test.go`
- Modify: `go/go.mod`, `go/go.sum` (자동)

**구현자에게**: TDD 강제. 단위 테스트는 fixture/mocking, 통합 테스트는 build tag로 분리.

- [ ] **Step 1: pgx 의존성 추가**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go
go get github.com/jackc/pgx/v5/pgxpool@v5.6.0
```

- [ ] **Step 2: 디렉터리 생성**

```bash
mkdir -p /Users/yuhojin/Desktop/quant-bot/go/internal/db
```

- [ ] **Step 3: 첫 실패 테스트 — BuildDSN 순수 함수**

Create `go/internal/db/pool_test.go`:
```go
package db

import (
	"strings"
	"testing"

	"github.com/yuhojin/quant-bot/go/internal/config"
)

func TestBuildDSN_FormatsCorrectly(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host: "db.local", Port: 5432,
		Name: "quantbot", User: "qb", Password: "p@ss",
		PoolMin: 2, PoolMax: 10,
	}
	dsn := BuildDSN(cfg)
	for _, want := range []string{
		"postgres://qb:p%40ss@db.local:5432/quantbot",
		"pool_min_conns=2",
		"pool_max_conns=10",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN에 %q 없음: %s", want, dsn)
		}
	}
}

func TestBuildDSN_EscapesSpecialPasswordChars(t *testing.T) {
	cfg := config.DatabaseConfig{
		Host: "h", Port: 5432, Name: "n", User: "u",
		Password: "a:b/c?d#e",
		PoolMin: 1, PoolMax: 1,
	}
	dsn := BuildDSN(cfg)
	// '/'는 path 구분자가 아니라 password 안에서 escape돼야 함
	if strings.Contains(dsn, "a:b/c?d#e") {
		t.Errorf("특수문자 escape 안 됨: %s", dsn)
	}
}
```

- [ ] **Step 4: 테스트 실행해 실패 확인**

```bash
cd go && go test ./internal/db/... -v
```

Expected: 컴파일 에러 — `BuildDSN` 미정의.

- [ ] **Step 5: BuildDSN 구현**

Create `go/internal/db/pool.go`:
```go
// Package db는 Postgres+TimescaleDB 연결 풀을 관리한다.
// fail-fast 룰(R12)에 따라 NewPool은 시작 시 헬스체크까지 통과해야 풀 반환.
package db

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yuhojin/quant-bot/go/internal/config"
)

var (
	// ErrPoolUnreachable은 헬스체크 실패 (DB 다운, 잘못된 자격 등) 시 반환된다.
	ErrPoolUnreachable = errors.New("DB 연결 풀 헬스체크 실패")
)

// healthCheckTimeout은 시작 시 SELECT 1 대기 시간 상한이다.
// 로컬 Docker Postgres는 보통 100ms 이내 응답. 1초면 충분 (R12 fail-fast).
const healthCheckTimeout = 1 * time.Second

// BuildDSN은 config로부터 PostgreSQL 연결 문자열을 만든다 (순수 함수).
// password 등 user-info에 들어가는 특수문자는 percent-encode된다.
func BuildDSN(cfg config.DatabaseConfig) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + cfg.Name,
	}
	q := u.Query()
	q.Set("pool_min_conns", fmt.Sprintf("%d", cfg.PoolMin))
	q.Set("pool_max_conns", fmt.Sprintf("%d", cfg.PoolMax))
	u.RawQuery = q.Encode()
	return u.String()
}

// NewPool은 풀 생성 + 시작 시 SELECT 1 헬스체크까지 한다 (R12).
// 실패 시 ErrPoolUnreachable로 wrap된 에러 반환.
func NewPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, BuildDSN(cfg))
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("%w: %v", ErrPoolUnreachable, err)
	}

	return pool, nil
}
```

- [ ] **Step 6: 테스트 실행해 통과 확인**

```bash
cd go && go test ./internal/db/... -v
```

Expected: 두 BuildDSN 테스트 모두 PASS.

- [ ] **Step 7: 통합 테스트 추가 (build tag로 단위 테스트와 분리)**

Create `go/internal/db/pool_integration_test.go`:
```go
//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/yuhojin/quant-bot/go/internal/config"
)

// 통합 테스트: 실제 Postgres가 5432에 떠 있어야 함.
// 실행: cd go && RUN_INTEGRATION=1 go test -tags=integration ./internal/db/...
func TestNewPool_HealthCheckPasses(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("RUN_INTEGRATION=1 환경변수 없음")
	}
	cfg := config.DatabaseConfig{
		Host: "localhost", Port: 5432,
		Name: "quantbot", User: "quantbot", Password: "changeme",
		PoolMin: 2, PoolMax: 10,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("NewPool 실패: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Errorf("두 번째 Ping 실패: %v", err)
	}
}

func TestNewPool_HealthCheckFails_BadHost(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION") != "1" {
		t.Skip("RUN_INTEGRATION=1 환경변수 없음")
	}
	cfg := config.DatabaseConfig{
		Host: "127.0.0.1", Port: 1, // 닿을 수 없는 포트
		Name: "quantbot", User: "quantbot", Password: "changeme",
		PoolMin: 1, PoolMax: 1,
	}
	ctx := context.Background()
	_, err := NewPool(ctx, cfg)
	if err == nil {
		t.Fatalf("도달 불가능한 host인데 에러 없음")
	}
	if !errors.Is(err, ErrPoolUnreachable) {
		t.Errorf("ErrPoolUnreachable 기대, 실제 %v", err)
	}
}
```

- [ ] **Step 8: 단위 테스트만 실행 (통합은 자동 skip)**

```bash
cd go && go test ./internal/db/... -v
```

Expected: BuildDSN 테스트 2개 PASS. 통합 테스트는 build tag 때문에 컴파일조차 안 됨.

- [ ] **Step 9: 통합 테스트 실행 (Postgres 기동 필요)**

```bash
cd /Users/yuhojin/Desktop/quant-bot
make up
sleep 8
cd go && RUN_INTEGRATION=1 go test -tags=integration ./internal/db/... -v
cd /Users/yuhojin/Desktop/quant-bot
make down
```

Expected: 통합 테스트 2개 (HealthCheckPasses + HealthCheckFails_BadHost) 모두 PASS.

- [ ] **Step 10: 커밋**

```bash
cd /Users/yuhojin/Desktop/quant-bot
git add go/internal/db/ go/go.mod go/go.sum
git commit -m "feat(db): pgxpool wrapper with fail-fast healthcheck (R12)"
```

---

## Task 5: Makefile 업데이트 (test-integration target)

**Files:**
- Modify: `go/Makefile`, `Makefile` (루트)

- [ ] **Step 1: `go/Makefile`에 test-integration target 추가**

`go/Makefile`을 다음으로 교체:
```makefile
.PHONY: test test-integration fmt lint build

test:  ## go test 단위만 (외부 의존 X)
	go test ./...

test-integration:  ## 통합 테스트 (실제 Postgres 필요)
	RUN_INTEGRATION=1 go test -tags=integration ./...

fmt:  ## go fmt 전체
	go fmt ./...

lint:  ## go vet 전체
	go vet ./...

build:  ## 모든 cmd 빌드
	go build ./...
```

- [ ] **Step 2: 루트 `Makefile`에 test-integration wrapper 추가**

루트 `Makefile`의 `.PHONY` 줄을 수정하고 새 target 추가:

OLD:
```makefile
.PHONY: help up down db-check test fmt lint
```

NEW:
```makefile
.PHONY: help up down db-check test test-integration fmt lint
```

그리고 `lint:` target 위에 다음 target 추가:
```makefile
test-integration:  ## Go·Python 통합 테스트 일괄 (실제 Postgres 기동 필요)
	$(MAKE) -C go test-integration
```

(Python은 Phase 2에서 추가 시 동시 호출하도록 갱신.)

- [ ] **Step 3: 단위 테스트 명령 동작 확인**

```bash
cd /Users/yuhojin/Desktop/quant-bot/go && make test && cd ..
```

Expected: 모든 단위 테스트 PASS, 통합 테스트는 build tag로 제외.

- [ ] **Step 4: 통합 테스트 명령 동작 확인 (Postgres 기동 후)**

```bash
cd /Users/yuhojin/Desktop/quant-bot
make up && sleep 8
make test-integration
make down
```

Expected: 통합 테스트 PASS. `make down`으로 정리.

- [ ] **Step 5: 커밋**

```bash
git add go/Makefile Makefile
git commit -m "build: add test-integration target (Go + root wrapper)"
```

---

## Task 6: 문서 동기화 (ARCHITECTURE / CLAUDE / STATUS / ROADMAP)

**Files:**
- Modify: `docs/ARCHITECTURE.md`, `CLAUDE.md`, `docs/STATUS.md`, `docs/ROADMAP.md`

이 task는 doc-only.

- [ ] **Step 1: `docs/ARCHITECTURE.md` R 요약 표에 R11~R13 추가**

`docs/ARCHITECTURE.md`에서 R10 줄 아래에 다음 3줄 추가:

기존:
```
| R10 | 빌드·테스트 독립성 (`go/`·`research/` 각자 단독 실행 가능) | [foundation §4](superpowers/specs/2026-05-02-foundation-design.md) |
```

다음 3줄을 R10 줄 바로 아래에 추가:
```
| R11 | 설정은 단일 TOML 파일이 단일 진실 원천 (example 동기화 강제) | [phase1a §4](superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) |
| R12 | 봇 시작 시 fail-fast 검증 (config → 검증 → DB 핑 → 진입) | [phase1a §4](superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) |
| R13 | 시간 표현 컨벤션 (로그 Unix ms, DB TIMESTAMPTZ, 코드 언어 타입) | [phase1a §4](superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) |
```

- [ ] **Step 2: `CLAUDE.md` R 요약 표에 R11~R13 추가**

`CLAUDE.md`에서 R10 줄 바로 아래에 다음 3줄 추가:
```
| R11 | 설정은 단일 TOML 파일이 단일 진실 원천 (example 동기화 강제) | [§4](docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) |
| R12 | 봇 시작 시 fail-fast 검증 (config → 검증 → DB 핑 → 진입) | [§4](docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) |
| R13 | 시간 표현 컨벤션 (로그 Unix ms, DB TIMESTAMPTZ, 코드 언어 타입) | [§4](docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) |
```

- [ ] **Step 3: `docs/STATUS.md` 갱신 — Phase 1a 완료 표시**

(a) 헤더의 "현재 Phase" 줄을 다음으로 변경:

OLD:
```
**현재 Phase**: Phase 0 완료. 다음: Phase 1a — Foundation Infrastructure (Go)
```

NEW:
```
**현재 Phase**: Phase 1a 완료. 다음: Phase 1b — 데이터 인제스트 (Go)
```

(b) "마지막 업데이트" 줄을 오늘 날짜로 갱신:
```
**마지막 업데이트**: 2026-05-03
```

(c) Phase 진행 상황의 Phase 1a 줄을 다음으로 변경:

OLD:
```
- [ ] Phase 1a — Foundation Infrastructure (Go)
```

NEW:
```
- [x] Phase 1a — Foundation Infrastructure (Go) (2026-05-03 완료)
```

(d) "최근 변경 이력" 맨 위에 한 줄 추가 (기존 줄들은 그대로):
```
- **2026-05-03** Phase 1a 완료 — Go 인프라 4종(설정 로더·로거·DB 풀·에러 패턴) 구현, R11~R13 도입, test-integration target 신설
```

- [ ] **Step 4: `docs/ROADMAP.md` 갱신 — Phase 1a 제거 + 다음 작업 재설정**

(a) "현재 추천 다음 작업" 줄을 변경:

OLD:
```
**현재 추천 다음 작업**: Phase 1a — Foundation Infrastructure (Go)
```

NEW:
```
**현재 추천 다음 작업**: Phase 1b — 데이터 인제스트 (Go)
```

(b) `### Phase 1a — Foundation Infrastructure (Go)` 전체 섹션 제거 (Phase 0 다음에 바로 Phase 1b가 오도록).

(c) Tier 분류의 Phase 1a 제거:

OLD:
```
- **Tier 1 (필수)**: Phase 1a, 1b, 2, 3, 4, 4.5, 5, 6, 7
```

NEW:
```
- **Tier 1 (필수)**: Phase 1b, 2, 3, 4, 4.5, 5, 6, 7
```

- [ ] **Step 5: 검증**

```bash
cd /Users/yuhojin/Desktop/quant-bot
grep -q "R11" docs/ARCHITECTURE.md && grep -q "R13" docs/ARCHITECTURE.md && echo "ARCH OK"
grep -q "R11" CLAUDE.md && grep -q "R13" CLAUDE.md && echo "CLAUDE OK"
grep -q "Phase 1a 완료" docs/STATUS.md && echo "STATUS OK"
grep -q "Phase 1b — 데이터 인제스트" docs/ROADMAP.md && ! grep -q "Phase 1a — Foundation" docs/ROADMAP.md && echo "ROADMAP OK"
```

Expected: 4줄 모두 OK 출력.

- [ ] **Step 6: 커밋 + 태그**

```bash
git add docs/ARCHITECTURE.md CLAUDE.md docs/STATUS.md docs/ROADMAP.md
git commit -m "$(cat <<'EOF'
docs: sync R11~R13 + mark Phase 1a complete

- ARCHITECTURE.md, CLAUDE.md: R11~R13 한 줄 요약 추가 (spec 링크)
- STATUS.md: Phase 1a 완료 표시, 변경 이력 갱신
- ROADMAP.md: Phase 1a 섹션 제거, 다음 작업 = Phase 1b
EOF
)"

git tag -a v0.1.0-phase1a -m "Phase 1a complete: Foundation infrastructure (Go) — config + log + db pool + error patterns"
```

- [ ] **Step 7: 최종 상태 확인**

```bash
git log --oneline | head -10
git tag | tail -5
```

Expected: 새 태그 `v0.1.0-phase1a` + Phase 1a 관련 커밋 6개 보임 (Task 1~6).
