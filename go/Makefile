.PHONY: test test-integration fmt lint build

test:  ## go test 단위만 (외부 의존 X)
	go test ./...

test-integration:  ## 통합 테스트 (실제 Postgres 필요)
	RUN_INTEGRATION=1 go test -tags=integration ./...

fmt:  ## go fmt 전체
	go fmt ./...

lint:  ## go vet 전체
	go vet ./...

build:  ## quantbot binary 빌드 (-o quantbot, ldflags Version 임베드)
	go build -o quantbot -ldflags="-X main.Version=$$(git describe --tags --always --dirty 2>/dev/null || echo dev)" ./cmd/quantbot
