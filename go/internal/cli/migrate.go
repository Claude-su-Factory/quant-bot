package cli

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Claude-su-Factory/quant-bot/go/internal/migrate"
)

func RunMigrate(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	configPath := fs.String("config", "config/config.toml", "config 파일 경로")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "사용법: quantbot migrate <up|status>")
		os.Exit(2)
	}

	ctx := context.Background()
	app, err := Bootstrap(ctx, *configPath, false)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
	defer app.Close()

	switch fs.Arg(0) {
	case "up":
		if err := migrate.Up(ctx, app.Pool); err != nil {
			fmt.Fprintln(os.Stderr, "마이그레이션 실패:", err)
			os.Exit(1)
		}
		fmt.Println("✅ 마이그레이션 적용 완료")
	case "status":
		if err := migrate.Status(ctx, app.Pool); err != nil {
			fmt.Fprintln(os.Stderr, "status 실패:", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 migrate 하위 명령: %s\n", fs.Arg(0))
		os.Exit(2)
	}
}
