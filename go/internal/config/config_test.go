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
