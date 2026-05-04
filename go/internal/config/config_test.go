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
	_, err := Load("testdata/missing_password.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (paper에서 password 빈 값), 실제 %v", err)
	}
}

func TestLoad_MalformedTOML(t *testing.T) {
	_, err := Load("testdata/invalid_syntax.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (TOML 파싱 실패), 실제 %v", err)
	}
}

func TestLoad_UnknownKey(t *testing.T) {
	_, err := Load("testdata/unknown_key.toml")
	if !errors.Is(err, ErrConfigInvalid) {
		t.Errorf("ErrConfigInvalid 기대 (unknown key), 실제 %v", err)
	}
}

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
