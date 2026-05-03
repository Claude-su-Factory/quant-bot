//go:build integration

package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Claude-su-Factory/quant-bot/go/internal/config"
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
