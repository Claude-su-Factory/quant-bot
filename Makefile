.PHONY: help up down db-check test test-integration fmt lint

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
