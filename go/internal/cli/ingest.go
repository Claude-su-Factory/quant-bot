package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Claude-su-Factory/quant-bot/go/internal/ingest/fred"
	"github.com/Claude-su-Factory/quant-bot/go/internal/repo"
	"github.com/Claude-su-Factory/quant-bot/go/internal/retry"
)

const fredDefaultBaseURL = "https://api.stlouisfed.org/fred"

func RunIngest(args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	defaultConfig := os.Getenv("QUANTBOT_CONFIG")
	if defaultConfig == "" {
		defaultConfig = "config/config.toml"
	}
	configPath := fs.String("config", defaultConfig, "config 파일 경로 (기본: $QUANTBOT_CONFIG 또는 config/config.toml)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "사용법: quantbot ingest <fred>")
		os.Exit(2)
	}

	ctx := context.Background()
	app, err := Bootstrap(ctx, *configPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	defer app.Close()

	switch fs.Arg(0) {
	case "fred":
		runFRED(ctx, app)
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 ingest source: %s\n", fs.Arg(0))
		os.Exit(2)
	}
}

func runFRED(ctx context.Context, app *App) {
	jobName := "ingest_fred"
	runID, err := repo.StartRun(ctx, app.Pool, jobName, app.Cfg.General.Environment)
	if err != nil {
		app.Logger.Error("StartRun 실패", "err", err)
		os.Exit(1)
	}

	var backfillStart time.Time
	backfillStart, err = time.Parse("2006-01-02", app.Cfg.Ingest.BackfillStartDate)
	if err != nil {
		app.Logger.Error("BackfillStartDate 파싱 실패",
			"val", app.Cfg.Ingest.BackfillStartDate, "err", err)
		os.Exit(1)
	}
	client := fred.New(fredDefaultBaseURL, app.Cfg.FRED.APIKey)
	ing := fred.NewIngester(client, app.Pool, fred.Config{
		Series:            app.Cfg.Ingest.FREDSeries,
		BackfillStartDate: backfillStart,
		Retry: retry.Config{
			MaxAttempts:       app.Cfg.Retry.MaxAttempts,
			BackoffInitialMs:  app.Cfg.Retry.BackoffInitialMs,
			BackoffMultiplier: app.Cfg.Retry.BackoffMultiplier,
		},
	}, app.Cfg.General.Environment)

	res, err := ing.Run(ctx)
	if err != nil {
		if ferr := repo.FinishRun(ctx, app.Pool, runID, repo.RunResult{
			Status: "failed", Error: err, RowsProcessed: res.RowsProcessed, RetryCount: res.RetryCount,
		}); ferr != nil {
			app.Logger.Error("FinishRun 기록 실패", "err", ferr)
		}
		app.Logger.Error("FRED 인제스트 실패", "err", err, "rows", res.RowsProcessed)
		os.Exit(1)
	}

	if ferr := repo.FinishRun(ctx, app.Pool, runID, repo.RunResult{
		Status: "success", RowsProcessed: res.RowsProcessed, RetryCount: res.RetryCount,
	}); ferr != nil {
		app.Logger.Error("FinishRun 기록 실패", "err", ferr)
	}
	app.Logger.Info("FRED 인제스트 완료", "rows", res.RowsProcessed, "retries", res.RetryCount)
	fmt.Printf("✅ FRED 수집 완료: %d행 (재시도 %d회)\n", res.RowsProcessed, res.RetryCount)
}
