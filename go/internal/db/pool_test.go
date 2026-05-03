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
	// negative: raw 형태 그대로 X
	if strings.Contains(dsn, "a:b/c?d#e") {
		t.Errorf("특수문자 raw 노출: %s", dsn)
	}
	// positive: 각 reserved 문자가 percent-encode된 형태 포함
	// (RFC 3986 userinfo 컨텍스트: ':' '/' '?' '#'는 인코딩됨)
	for _, encoded := range []string{"%3A", "%2F", "%3F", "%23"} {
		if !strings.Contains(dsn, encoded) {
			t.Errorf("기대 인코딩 %q 없음: %s", encoded, dsn)
		}
	}
}
