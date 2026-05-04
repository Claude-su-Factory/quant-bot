.PHONY: help up down db-check test test-integration fmt lint prepare-migrations build install uninstall logs

COMPOSE := docker compose -f docker/docker-compose.yml

help:  ## 사용 가능한 명령 출력
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

up:  ## Postgres+TimescaleDB 기동
	$(COMPOSE) up -d

down:  ## Postgres 정지
	$(COMPOSE) down

db-check:  ## DB 연결 확인 (컨테이너 내 pg_isready)
	$(COMPOSE) exec db pg_isready -U quantbot -d quantbot

test:  ## Go + Python 전체 테스트
	$(MAKE) -C go test
	$(MAKE) -C research test

test-integration:  ## Go·Python 통합 테스트 일괄 (실제 Postgres 기동 필요)
	$(MAKE) -C go test-integration

fmt:  ## Go + Python 포매팅
	$(MAKE) -C go fmt
	$(MAKE) -C research fmt

lint:  ## Go + Python 린트
	$(MAKE) -C go lint
	$(MAKE) -C research lint

prepare-migrations:  ## shared/schema/migrations → go/internal/migrate/migrations 동기화 (R9)
	@mkdir -p go/internal/migrate/migrations
	@cp -r shared/schema/migrations/. go/internal/migrate/migrations/

build: prepare-migrations  ## quantbot binary 빌드 (마이그레이션 임베드 포함)
	$(MAKE) -C go build

install:  ## quantbot 빌드 + 마이그레이션 적용 + LaunchAgent 등록 (1회 셋업)
	docker compose -f docker/docker-compose.yml up -d --wait
	$(MAKE) build
	./go/quantbot migrate up
	@mkdir -p ~/Library/LaunchAgents logs
	@sed -e "s|{{QUANTBOT_BINARY_PATH}}|$(CURDIR)/go/quantbot|g" \
	     -e "s|{{QUANTBOT_PROJECT_PATH}}|$(CURDIR)|g" \
	     deploy/launchd/com.quantbot.ingest-fred.plist \
	     > ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@launchctl unload ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist 2>/dev/null || true
	@launchctl load ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@echo ""
	@echo "✅ 셋업 완료."
	@echo ""
	@echo "📅 매일 22:00 (시스템 로컬 타임존)에 자동 실행됨."
	@echo "    install 직후엔 즉시 실행되지 않습니다 (다음 정시 대기)."
	@echo "    지금 한 번 테스트하려면: launchctl start com.quantbot.ingest-fred"
	@echo ""
	@echo "🔍 상태 확인:    ./go/quantbot status"
	@echo "📜 로그 tail:    make logs"
	@echo "🗑  제거:         make uninstall"

uninstall:  ## LaunchAgent 제거 (DB·로그·binary 보존)
	@launchctl unload ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist 2>/dev/null || true
	@rm -f ~/Library/LaunchAgents/com.quantbot.ingest-fred.plist
	@echo "✅ LaunchAgent 제거됨. (DB·로그·binary는 그대로 — 데이터 보존)"

logs:  ## 오늘 봇 로그 tail (Ctrl+C로 종료)
	@tail -f logs/app-$$(date +%Y-%m-%d).log | jq -C .
