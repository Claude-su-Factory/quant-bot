package cli

import (
	"context"
	"errors"
	"testing"
)

func TestBootstrap_InvalidConfigPath(t *testing.T) {
	_, err := Bootstrap(context.Background(), "testdata/does_not_exist.toml", false)
	if err == nil {
		t.Fatalf("존재하지 않는 path에 에러 기대")
	}
	if !errors.Is(err, ErrBootstrap) {
		t.Errorf("ErrBootstrap 기대, 실제 %v", err)
	}
}
