package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// 헬퍼: 메모리 버퍼로 로거 만들고 한 줄 로그 후 JSON 파싱
func captureLogJSON(t *testing.T, environment string, includeCaller bool, write func(l *Logger)) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, "info", environment, includeCaller)
	write(logger)

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatalf("로그 출력이 비어 있음")
	}
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("로그가 유효한 JSON이 아님: %v\n원문: %s", err, line)
	}
	return entry
}

func TestLogger_JSON_TimeIsUnixSecondsFloat(t *testing.T) {
	entry := captureLogJSON(t, "paper", false, func(l *Logger) {
		l.Info("hello")
	})
	tv, ok := entry["time"]
	if !ok {
		t.Fatalf("time 필드 없음")
	}
	switch tv.(type) {
	case float64:
		// OK — Unix 초 + 소수점 ms
	default:
		t.Errorf("time 필드 타입 기대 float64 (Unix 초.밀리초), 실제 %T", tv)
	}
}

func TestLogger_JSON_EnvironmentField(t *testing.T) {
	entry := captureLogJSON(t, "paper", false, func(l *Logger) {
		l.Info("hello")
	})
	if env, _ := entry["environment"].(string); env != "paper" {
		t.Errorf("environment 필드 기대 paper, 실제 %v", entry["environment"])
	}
}

func TestLogger_JSON_MsgField(t *testing.T) {
	entry := captureLogJSON(t, "dev", false, func(l *Logger) {
		l.Info("greetings")
	})
	if msg, _ := entry["msg"].(string); msg != "greetings" {
		t.Errorf("msg 필드 기대 greetings, 실제 %v", entry["msg"])
	}
}

func TestLogger_IncludeCaller_True(t *testing.T) {
	entry := captureLogJSON(t, "dev", true, func(l *Logger) {
		l.Info("with caller")
	})
	if _, ok := entry["caller"]; !ok {
		t.Errorf("include_caller=true이지만 caller 필드 없음")
	}
}

func TestLogger_IncludeCaller_False(t *testing.T) {
	entry := captureLogJSON(t, "dev", false, func(l *Logger) {
		l.Info("no caller")
	})
	if _, ok := entry["caller"]; ok {
		t.Errorf("include_caller=false인데 caller 필드 있음")
	}
}

func TestNew_WritesToFile(t *testing.T) {
	dir := t.TempDir()
	logger, closeFn, err := New(dir, "info", "paper", false)
	if err != nil {
		t.Fatalf("New 실패: %v", err)
	}
	defer closeFn()

	logger.Info("file test")

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("로그 파일 1개 기대, 실제 %d개", len(entries))
	}
	data, _ := os.ReadFile(dir + "/" + entries[0].Name())
	if !bytes.Contains(data, []byte(`"msg":"file test"`)) {
		t.Errorf("파일에 메시지 기록 X. 내용:\n%s", data)
	}
}
