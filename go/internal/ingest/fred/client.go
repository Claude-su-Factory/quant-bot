// Package fred는 FRED API 클라이언트 + IsRetryable 정책.
package fred

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Observation은 FRED API의 한 관측 (date + value).
// Value는 NULL 가능 (FRED가 휴장일 등에 "." 반환).
type Observation struct {
	Date  time.Time
	Value *float64
}

// HTTPError는 FRED API에서 4xx/5xx 받았을 때 반환된다.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("FRED HTTP %d: %s", e.StatusCode, e.Body)
}

// Client는 FRED API 호출자.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New는 baseURL(예: "https://api.stlouisfed.org/fred")과 API key로 클라이언트 생성.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchSeries는 지정한 기간의 시리즈 관측을 가져온다.
func (c *Client) FetchSeries(ctx context.Context, seriesID string, start, end time.Time) ([]Observation, error) {
	q := url.Values{}
	q.Set("series_id", seriesID)
	q.Set("api_key", c.apiKey)
	q.Set("file_type", "json")
	q.Set("observation_start", start.Format("2006-01-02"))
	q.Set("observation_end", end.Format("2006-01-02"))

	endpoint := c.baseURL + "/series/observations?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("FRED req 생성: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("FRED HTTP: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var raw struct {
		Observations []struct {
			Date  string `json:"date"`
			Value string `json:"value"`
		} `json:"observations"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("FRED JSON 파싱: %w", err)
	}

	out := make([]Observation, 0, len(raw.Observations))
	for _, o := range raw.Observations {
		date, err := time.Parse("2006-01-02", o.Date)
		if err != nil {
			return nil, fmt.Errorf("FRED date 파싱 %q: %w", o.Date, err)
		}
		var v *float64
		if o.Value != "." && o.Value != "" {
			f, err := strconv.ParseFloat(o.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("FRED value 파싱 %q: %w", o.Value, err)
			}
			v = &f
		}
		out = append(out, Observation{Date: date.UTC(), Value: v})
	}
	return out, nil
}

// IsRetryable는 FRED API 결과가 재시도 대상인지 판단.
// HTTPError 4xx → false (단, 429는 rate limit이라 재시도). 5xx 재시도. 네트워크 등 그 외 재시도.
func IsRetryable(err error) bool {
	var hErr *HTTPError
	if errors.As(err, &hErr) {
		if hErr.StatusCode == 429 {
			return true
		}
		if hErr.StatusCode >= 400 && hErr.StatusCode < 500 {
			return false
		}
		return true
	}
	return true
}
