// Package logging은 구조화 JSON 로거를 제공한다.
// 시간은 R13 컨벤션에 따라 Unix 초.밀리초 (float)로 직렬화.
// 모든 로그에 environment 필드 자동 첨부.
package logging

import (
	"io"
	"log/slog"
	"os"
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
			// source 필드 → caller로 이름 변경
			if a.Key == slog.SourceKey {
				src, ok := a.Value.Any().(*slog.Source)
				if ok && src != nil {
					return slog.String("caller", shortPath(src.File)+":"+itoa(src.Line))
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
	if err := os.MkdirAll(fileDir, 0755); err != nil {
		return nil, nil, err
	}
	fileName := fileDir + "/app-" + time.Now().Format("2006-01-02") + ".log"
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	mw := io.MultiWriter(os.Stderr, f)
	logger := NewWithWriter(mw, logLevel, environment, includeCaller)
	return logger, f.Close, nil
}

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

func shortPath(p string) string {
	// 마지막 경로 세그먼트 두 개만 남김 (가독성)
	idx := -1
	count := 0
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			count++
			if count == 2 {
				idx = i + 1
				break
			}
		}
	}
	if idx < 0 {
		return p
	}
	return p[idx:]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// 경과 시간(예: API 호출 소요 시간) 기록 시 wall clock 차이가 아닌 monotonic clock을 써야 한다.
// Go에선 time.Since(start)가 자동으로 monotonic을 사용하므로 다음 패턴 권장:
//
//	start := time.Now()
//	doWork()
//	logger.Info("done", "duration_ms", time.Since(start).Milliseconds())
//
// time.Now() - time.Now() 같은 빼기는 NTP 동기화 시 음수 가능. R13 컨벤션 참조.
