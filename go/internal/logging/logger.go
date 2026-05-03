// Package logging은 구조화 JSON 로거를 제공한다.
// 시간은 R13 컨벤션에 따라 Unix 초.밀리초 (float)로 직렬화.
// 모든 로그에 environment 필드 자동 첨부.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Logger는 slog 기반 구조화 로거.
type Logger = slog.Logger

// NewWithWriter는 임의의 io.Writer로 출력하는 로거를 만든다 (테스트용).
// 일반 사용은 New를 사용.
func NewWithWriter(w io.Writer, logLevel, environment string, includeCaller bool) *Logger {
	level := parseLevel(logLevel)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: includeCaller,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// time 필드: Unix 초.밀리초 float
			if a.Key == slog.TimeKey {
				t := a.Value.Time()
				return slog.Float64(slog.TimeKey, float64(t.UnixMilli())/1000.0)
			}
			// level 필드: 소문자로 통일 (spec §6.1 예시 일관성)
			if a.Key == slog.LevelKey {
				return slog.String(slog.LevelKey, strings.ToLower(a.Value.String()))
			}
			// source 필드 → caller로 이름 변경
			if a.Key == slog.SourceKey {
				src, ok := a.Value.Any().(*slog.Source)
				if ok && src != nil {
					return slog.String("caller", shortPath(src.File)+":"+strconv.Itoa(src.Line))
				}
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(w, opts)
	logger := slog.New(handler).With("environment", environment)
	return logger
}

// New는 stderr + 로그 파일 양쪽으로 출력하는 로거 + close 함수를 만든다.
func New(fileDir, logLevel, environment string, includeCaller bool) (*Logger, func() error, error) {
	return newWithStderr(os.Stderr, fileDir, logLevel, environment, includeCaller)
}

// newWithStderr는 stderr 대신 임의 writer를 1차 출력으로 받는다 (테스트용 편의).
// 외부에 공개하지 않음 — 일반 사용자는 New를 사용.
func newWithStderr(stderrLike io.Writer, fileDir, logLevel, environment string, includeCaller bool) (*Logger, func() error, error) {
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return nil, nil, err
	}
	fileName := filepath.Join(fileDir, "app-"+time.Now().Format("2006-01-02")+".log")
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	mw := io.MultiWriter(stderrLike, f)
	logger := NewWithWriter(mw, logLevel, environment, includeCaller)
	return logger, f.Close, nil
}

// parseLevel은 문자열을 slog.Level로 변환한다.
// 알 수 없는 값은 slog.LevelInfo로 fallback한다 — config.validate()가 사전에
// {debug, info, warn, error}만 허용하므로 일반 경로에선 fallback 발동 안 함.
// NewWithWriter를 외부에서 임의 문자열로 직접 호출하는 경우만 fallback 의미.
func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// shortPath는 마지막 두 경로 세그먼트만 남긴다 (예: "logging/logger.go").
// cross-platform 지원: os.PathSeparator 사용.
func shortPath(p string) string {
	sep := string(os.PathSeparator)
	idx := strings.LastIndex(p, sep)
	if idx < 0 {
		return p
	}
	prefix := p[:idx]
	idx2 := strings.LastIndex(prefix, sep)
	if idx2 < 0 {
		return p
	}
	return p[idx2+1:]
}

// 경과 시간(예: API 호출 소요 시간) 기록 시 wall clock 차이가 아닌 monotonic clock을 써야 한다.
// Go에선 time.Since(start)가 자동으로 monotonic을 사용하므로 다음 패턴 권장:
//
//	start := time.Now()
//	doWork()
//	logger.Info("done", "duration_ms", time.Since(start).Milliseconds())
//
// time.Now() - time.Now() 같은 빼기는 NTP 동기화 시 음수 가능. R13 컨벤션 참조.
