.PHONY: sync test fmt lint

sync:  ## 의존성 동기화
	uv sync

test: sync  ## pytest 실행
	uv run pytest

fmt: sync  ## ruff format
	uv run ruff format .

lint: sync  ## ruff check
	uv run ruff check .
