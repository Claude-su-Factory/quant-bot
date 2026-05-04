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
		if isRetryable != nil && !isRetryable(err) {
			return retries, err
		}
		if attempt == cfg.MaxAttempts {
			return retries, err
		}
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
