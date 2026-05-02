# quant-bot

미국 주식 스윙 트레이딩 퀀트 봇.

**현재 Phase**: 0 (골격 구축 중) — 진행은 [`docs/STATUS.md`](docs/STATUS.md), 다음 작업은 [`docs/ROADMAP.md`](docs/ROADMAP.md) 참조.

## 시작하기

### 사전 준비

- macOS (Apple Silicon/Intel)
- Docker Desktop
- Go 1.22+ (`brew install go`)
- uv (`brew install uv` 또는 `curl -LsSf https://astral.sh/uv/install.sh | sh`)
- Make

### 환경 설정

```bash
# 1. 환경 변수 파일 복사 후 비밀번호 채우기
cp .env.example .env
# .env 편집

# 2. Postgres + TimescaleDB 기동
make up

# 3. 연결 확인
make db-check
```

### 디렉터리 안내

```
go/         — Go: 데이터 인제스트, 실행 엔진, 스케줄러
research/   — Python: 백테스트, 팩터 연구, 모델 훈련
shared/     — 양 언어 공유 (스키마, 계약, 산출물)
docker/     — 로컬 인프라 (Postgres + TimescaleDB)
docs/       — 문서 (STATUS, ROADMAP, ARCHITECTURE, specs/, plans/)
.claude/    — Claude Code 설정 (에이전트 정의)
```

### 자주 쓰는 명령

```bash
make up         # Postgres 기동
make down       # Postgres 정지
make db-check   # DB 연결 확인
make test       # Go + Python 전체 테스트
make fmt        # 포매팅
make lint       # 린트
make help       # 전체 명령 목록
```

### 핵심 룰 / 다음 작업

상세는 [`CLAUDE.md`](CLAUDE.md) 및 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) 참조.

## 라이선스

Private.
