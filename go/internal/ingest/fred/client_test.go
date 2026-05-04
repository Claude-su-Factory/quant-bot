package fred

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSeries_ParsesObservations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Query params 검증 — client가 올바른 URL 패턴 보내는지
		q := r.URL.Query()
		if got := q.Get("series_id"); got != "T10Y2Y" {
			t.Errorf("series_id 기대 T10Y2Y, 실제 %q", got)
		}
		if got := q.Get("api_key"); got != "test_api_key" {
			t.Errorf("api_key 기대 test_api_key, 실제 %q", got)
		}
		if got := q.Get("file_type"); got != "json" {
			t.Errorf("file_type 기대 json, 실제 %q", got)
		}
		if got := q.Get("observation_start"); got != "2026-04-01" {
			t.Errorf("observation_start 기대 2026-04-01, 실제 %q", got)
		}
		if got := q.Get("observation_end"); got != "2026-04-03" {
			t.Errorf("observation_end 기대 2026-04-03, 실제 %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"observations": [
				{"date": "2026-04-01", "value": "0.25"},
				{"date": "2026-04-02", "value": "."},
				{"date": "2026-04-03", "value": "0.30"}
			]
		}`))
	}))
	defer server.Close()

	client := New(server.URL, "test_api_key")
	obs, err := client.FetchSeries(context.Background(), "T10Y2Y", mustDate("2026-04-01"), mustDate("2026-04-03"))
	if err != nil {
		t.Fatalf("FetchSeries 실패: %v", err)
	}
	if len(obs) != 3 {
		t.Fatalf("3개 기대, 실제 %d", len(obs))
	}
	if !obs[0].Date.Equal(mustDate("2026-04-01")) {
		t.Errorf("date 파싱 실패: %v", obs[0].Date)
	}
	if obs[0].Value == nil || *obs[0].Value != 0.25 {
		t.Errorf("value 파싱 실패: %v", obs[0].Value)
	}
	if obs[1].Value != nil {
		t.Errorf("'.' value는 nil 기대, 실제 %v", *obs[1].Value)
	}
}

func TestFetchSeries_5xxReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", 503)
	}))
	defer server.Close()

	client := New(server.URL, "test")
	_, err := client.FetchSeries(context.Background(), "T10Y2Y", mustDate("2026-04-01"), mustDate("2026-04-01"))
	if err == nil {
		t.Fatalf("503에 에러 기대")
	}
	var hErr *HTTPError
	if !errors.As(err, &hErr) {
		t.Fatalf("HTTPError 기대, 실제 %T", err)
	}
	if hErr.StatusCode != 503 {
		t.Errorf("StatusCode 기대 503, 실제 %d", hErr.StatusCode)
	}
}

func TestIsRetryable_4xxNotRetryable(t *testing.T) {
	err := &HTTPError{StatusCode: 400, Body: "bad request"}
	if IsRetryable(err) {
		t.Errorf("4xx는 비재시도 기대")
	}
}

func TestIsRetryable_5xxRetryable(t *testing.T) {
	err := &HTTPError{StatusCode: 503, Body: "x"}
	if !IsRetryable(err) {
		t.Errorf("5xx는 재시도 기대")
	}
}

func TestIsRetryable_429Retryable(t *testing.T) {
	err := &HTTPError{StatusCode: 429, Body: "rate limit"}
	if !IsRetryable(err) {
		t.Errorf("429는 재시도 기대")
	}
}

func TestIsRetryable_NetworkErrorRetryable(t *testing.T) {
	err := errors.New("connection reset")
	if !IsRetryable(err) {
		t.Errorf("일반 에러는 재시도 기대 (네트워크 가정)")
	}
}

func mustDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t.UTC()
}
