package fred

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/Claude-su-Factory/quant-bot/go/internal/repo"
	"github.com/Claude-su-Factory/quant-bot/go/internal/retry"
)

// Config는 ingester 동작 설정.
type Config struct {
	Series            []string
	BackfillStartDate time.Time
	Retry             retry.Config
}

// Result는 Run의 결과 통계.
type Result struct {
	RowsProcessed int
	RetryCount    int
}

// Ingester는 FRED 시리즈를 DB에 수집하는 작업.
type Ingester struct {
	client   *Client
	pool     *pgxpool.Pool
	cfg      Config
	instance string
}

// NewIngester는 의존성 주입으로 Ingester 생성.
// instance: 운영 환경 식별자(paper/live/dev/test). 현재 Run에선 미사용이지만
// 향후 structured logging (slog.With("instance", ...))에 사용 예정 (Phase 1b-B).
// runs 테이블의 instance 컬럼은 cli 레이어가 별도로 처리 (spec §7.2).
func NewIngester(client *Client, pool *pgxpool.Pool, cfg Config, instance string) *Ingester {
	return &Ingester{client: client, pool: pool, cfg: cfg, instance: instance}
}

// Run은 모든 시리즈를 수집한다 (증분 + 첫 실행 시 백필).
// 실패 시 마지막 에러 반환. 부분 성공 시 누적 통계 반환.
func (i *Ingester) Run(ctx context.Context) (Result, error) {
	var res Result
	// 같은 Run 내 모든 series에 일관된 cut-off 적용 (retry 지연으로 series 간 drift 방지)
	end := time.Now().UTC()
	for _, seriesID := range i.cfg.Series {
		last, err := repo.LastObservedAt(ctx, i.pool, seriesID)
		if err != nil {
			return res, fmt.Errorf("LastObservedAt %s: %w", seriesID, err)
		}
		var start time.Time
		if last.IsZero() {
			start = i.cfg.BackfillStartDate
		} else {
			// LastObservedAt은 UTC TIMESTAMPTZ 보장 (DST 무관, 24시간 ADD 안전)
			start = last.Add(24 * time.Hour)
		}
		if !start.Before(end) {
			continue
		}

		var obs []Observation
		retries, err := retry.Do(ctx, i.cfg.Retry, IsRetryable, func() error {
			var inner error
			obs, inner = i.client.FetchSeries(ctx, seriesID, start, end)
			return inner
		})
		res.RetryCount += retries
		if err != nil {
			return res, fmt.Errorf("FetchSeries %s: %w", seriesID, err)
		}

		repoObs := make([]repo.Observation, len(obs))
		for k, o := range obs {
			repoObs[k] = repo.Observation{
				SeriesID:   seriesID,
				ObservedAt: o.Date,
				Value:      o.Value,
			}
		}
		n, err := repo.InsertObservations(ctx, i.pool, repoObs)
		if err != nil {
			return res, fmt.Errorf("InsertObservations %s: %w", seriesID, err)
		}
		res.RowsProcessed += n
	}
	return res, nil
}
