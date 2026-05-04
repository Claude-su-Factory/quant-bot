package main

import (
	"fmt"
	"os"

	"github.com/Claude-su-Factory/quant-bot/go/internal/cli"
)

// Version은 -ldflags="-X main.Version=$(git describe ...)"로 빌드 타임 주입.
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "ingest":
		cli.RunIngest(os.Args[2:])
	case "status":
		cli.RunStatus(os.Args[2:])
	case "migrate":
		cli.RunMigrate(os.Args[2:])
	case "version", "--version", "-v":
		cli.RunVersion(Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "알 수 없는 명령: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println(`quantbot — 미국 주식 스윙 트레이딩 봇

사용법:
  quantbot <command> [args]

명령:
  ingest fred       FRED 거시 데이터 수집
  status            최근 운영 상태 표시
  migrate up        DB 마이그레이션 적용
  migrate status    마이그레이션 적용 상태
  version           버전 정보
  help              본 도움말`)
}
