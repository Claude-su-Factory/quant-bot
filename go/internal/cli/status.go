package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/Claude-su-Factory/quant-bot/go/internal/repo"
)

func RunStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "config/config.toml", "config 파일 경로")
	fs.Parse(args)

	ctx := context.Background()
	app, err := Bootstrap(ctx, *configPath, true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	defer app.Close()

	fmt.Println("=== quant-bot 운영 상태 ===")
	fmt.Println("환경:", app.Cfg.General.Environment)
	fmt.Println()

	runs, err := repo.RecentRuns(ctx, app.Pool, 10)
	if err != nil {
		fmt.Fprintln(os.Stderr, "RecentRuns 실패:", err)
		os.Exit(1)
	}
	fmt.Println("최근 실행 (10건):")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "시작\t작업\t상태\t행수\t재시도\t소요\t에러")
	for _, r := range runs {
		dur := ""
		if r.FinishedAt != nil {
			dur = r.FinishedAt.Sub(r.StartedAt).Round(time.Millisecond).String()
		} else {
			dur = "(진행중/비정상)"
		}
		emoji := statusEmoji(r.Status)
		errMsg := r.ErrorMessage
		if len(errMsg) > 40 {
			errMsg = errMsg[:40] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s %s\t%d\t%d\t%s\t%s\n",
			r.StartedAt.Local().Format("2006-01-02 15:04:05"),
			r.JobName, emoji, r.Status, r.RowsProcessed, r.RetryCount, dur, errMsg)
	}
	w.Flush()
	fmt.Println()

	fmt.Println("거시 시리즈 현황:")
	rows, err := app.Pool.Query(ctx,
		`SELECT series_id, MAX(observed_at), COUNT(*), MAX(ingested_at)
		 FROM macro_series GROUP BY series_id ORDER BY series_id`)
	if err == nil {
		ws := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(ws, "시리즈\t마지막 관측\t총 행수\tDB 입력")
		for rows.Next() {
			var sid string
			var observed, ingested time.Time
			var n int
			rows.Scan(&sid, &observed, &n, &ingested)
			fmt.Fprintf(ws, "%s\t%s\t%d\t%s\n", sid,
				observed.Local().Format("2006-01-02"), n,
				ingested.Local().Format("2006-01-02 15:04:05"))
		}
		rows.Close()
		ws.Flush()
	}
	fmt.Println()

	stale, err := repo.StaleRuns(ctx, app.Pool)
	if err == nil {
		if len(stale) == 0 {
			fmt.Println("비정상 종료: 0건")
		} else {
			fmt.Printf("⚠️  비정상 종료 (started_at + 1h 넘게 finished_at NULL): %d건\n", len(stale))
			for _, r := range stale {
				fmt.Printf("   id=%d %s 시작 %s\n", r.ID, r.JobName,
					r.StartedAt.Local().Format("2006-01-02 15:04:05"))
			}
		}
	}
	fmt.Println()
	fmt.Println("LaunchAgent: launchctl list | grep com.quantbot 로 활성 여부 확인")
	fmt.Println("다음 예정: 매일 22:00 (시스템 로컬 타임존, plist 기준)")
}

func statusEmoji(s string) string {
	switch s {
	case "success":
		return "✅"
	case "failed":
		return "❌"
	case "running":
		return "⏳"
	default:
		return "❓"
	}
}
