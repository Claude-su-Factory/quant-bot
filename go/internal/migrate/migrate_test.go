package migrate

import (
	"strings"
	"testing"
)

func TestMigrationsFS_HasFiles(t *testing.T) {
	expected := []string{
		"migrations/20260503000001_enable_timescaledb.sql",
		"migrations/20260503000002_create_macro_series.sql",
		"migrations/20260503000003_create_runs.sql",
	}
	for _, name := range expected {
		if _, err := MigrationsFS.Open(name); err != nil {
			t.Errorf("임베드된 fs에 %q 없음: %v", name, err)
		}
	}
}

func TestMigrationsFS_OnlySQL(t *testing.T) {
	entries, err := MigrationsFS.ReadDir("migrations")
	if err != nil {
		t.Fatalf("ReadDir 실패: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("마이그레이션 0개")
	}
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("하위 디렉터리는 없어야 함: %s", e.Name())
		}
		if !strings.HasSuffix(e.Name(), ".sql") {
			t.Errorf("non-SQL 파일 발견: %s", e.Name())
		}
	}
}
