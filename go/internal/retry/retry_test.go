package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

var defaultCfg = Config{
	MaxAttempts:       3,
	BackoffInitialMs:  10,   // 테스트 빠르게
	BackoffMultiplier: 2.0,
}

func TestDo_FirstTrySuccess(t *testing.T) {
	calls := 0
	retries, err := Do(context.Background(), defaultCfg, nil, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("기대 nil, 실제 %v", err)
	}
	if calls != 1 {
		t.Errorf("calls 기대 1, 실제 %d", calls)
	}
	if retries != 0 {
		t.Errorf("retries 기대 0, 실제 %d", retries)
	}
}

func TestDo_RetryThenSuccess(t *testing.T) {
	calls := 0
	retries, err := Do(context.Background(), defaultCfg, nil, func() error {
		calls++
		if calls < 3 {
			return errors.New("temporary")
		}
		return nil
	})
	if err != nil {
		t.Errorf("기대 nil, 실제 %v", err)
	}
	if calls != 3 {
		t.Errorf("calls 기대 3, 실제 %d", calls)
	}
	if retries != 2 {
		t.Errorf("retries 기대 2, 실제 %d", retries)
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	want := errors.New("permanent")
	calls := 0
	retries, err := Do(context.Background(), defaultCfg, nil, func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("마지막 에러 기대 %v, 실제 %v", want, err)
	}
	if calls != 3 {
		t.Errorf("calls 기대 3, 실제 %d", calls)
	}
	if retries != 2 {
		t.Errorf("retries 기대 2, 실제 %d", retries)
	}
}

func TestDo_NonRetryableErrorStopsImmediately(t *testing.T) {
	want := errors.New("non-retryable")
	calls := 0
	isRetryable := func(err error) bool {
		return false
	}
	retries, err := Do(context.Background(), defaultCfg, isRetryable, func() error {
		calls++
		return want
	})
	if !errors.Is(err, want) {
		t.Errorf("에러 기대 %v, 실제 %v", want, err)
	}
	if calls != 1 {
		t.Errorf("calls 기대 1, 실제 %d", calls)
	}
	if retries != 0 {
		t.Errorf("retries 기대 0, 실제 %d", retries)
	}
}

func TestDo_ContextCancelStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	_, err := Do(ctx, defaultCfg, nil, func() error {
		calls++
		return errors.New("would retry")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("context.Canceled 기대, 실제 %v", err)
	}
	if calls > 1 {
		t.Errorf("최대 1회 호출 기대, 실제 %d", calls)
	}
}

func TestDo_BackoffIncreases(t *testing.T) {
	cfg := Config{
		MaxAttempts:       3,
		BackoffInitialMs:  50,
		BackoffMultiplier: 2.0,
	}
	start := time.Now()
	Do(context.Background(), cfg, nil, func() error {
		return errors.New("always fail")
	})
	elapsed := time.Since(start)
	// 최소 50ms (1차 후 대기) + 100ms (2차 후 대기) = 150ms (3차 실패 후 대기 X)
	if elapsed < 130*time.Millisecond {
		t.Errorf("백오프 너무 짧음: %v", elapsed)
	}
}
