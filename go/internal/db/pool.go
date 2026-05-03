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
	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
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
		return nil, fmt.Errorf("%w: pgxpool.New: %v", ErrPoolUnreachable, err)
	}

	// 헬스체크는 외부 ctx 취소·deadline과 무관하게 1초 보장 (R12 fail-fast).
	// 부모 ctx에서 파생하면 caller가 짧은 deadline을 줬을 때 spurious 실패 가능.
	pingCtx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("%w: %v", ErrPoolUnreachable, err)
	}

	return pool, nil
}
