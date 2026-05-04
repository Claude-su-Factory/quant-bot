// Package cli는 quantbot CLI subcommand 구현.
// Bootstrap은 모든 명령이 공유하는 셋업 (config + logger + DB pool + 마이그레이션 검증).
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
	"github.com/Claude-su-Factory/quant-bot/go/internal/db"
	"github.com/Claude-su-Factory/quant-bot/go/internal/logging"
	"github.com/Claude-su-Factory/quant-bot/go/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
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
// requireMigrated가 true면 미적용 마이그레이션 감지 시 ErrMigrationsPending wrap된 에러 반환.
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
