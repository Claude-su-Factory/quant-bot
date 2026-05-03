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
