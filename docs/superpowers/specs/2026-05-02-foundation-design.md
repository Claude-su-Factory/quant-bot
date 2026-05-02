---
title: Foundation Design — quant-bot
date: 2026-05-02
status: Approved (브레인스토밍 완료)
phase: Phase 0 — Project Skeleton & Harness Engineering
authors: yuhojin, Claude (브레인스토밍 세션)
---

# Foundation Design — quant-bot

## 1. 목표

미국 주식 스윙 트레이딩 퀀트 봇 프로젝트의 **골격, 에이전트 팀, 운영 룰**을 확립한다. 이번 Phase 0에서는 비즈니스 로직 코드(데이터 인제스트·전략·실행)를 작성하지 않는다 — Makefile, docker-compose, `go.mod`/`pyproject.toml` 같은 설정·빌드 자산만 생성한다. 모든 후속 Phase의 기반이 되는 디렉터리 구조, 하네스 문서, 에이전트 정의, 핵심 설계 결정을 파일로 박아둬서 **다른 세션이 들어와도 동일한 컨텍스트로 작업 재개 가능**하게 만든다.

## 2. 범위 외 (Phase 0에 포함하지 않음)

- 데이터 인제스트 코드 (Phase 1)
- Feature Engineering 구현 (Phase 2)
- 백테스트 엔진 (Phase 3)
- 전략 구현 (Phase 3+)
- 브로커 어댑터 (Phase 5)
- 실행 엔진 (Phase 6)
- 어떤 형태든 실행 가능한 비즈니스 로직

이번 Phase의 산출물은 **모두 비실행 자산** (디렉터리, 마크다운 문서, 설정 파일, 빈 스캐폴딩)이다.

## 3. 컨텍스트 (브레인스토밍 결정사항)

| # | 항목 | 결정 |
|---|------|------|
| 1 | 시장 | 미국 주식 |
| 2 | 거래 빈도 | 스윙 (일/주 단위 리밸런싱) |
| 3 | 인프라 | 로컬, Docker DB, 클라우드 X |
| 4 | 컴퓨터 다운 허용 | 모든 리스크 관리는 브로커 측 위임 |
| 5 | 학습 방식 | Walk-forward 재학습 + Champion/Challenger |
| 6 | 출발 전략 | Clenow Momentum + Yield Curve Regime Filter |
| 7 | 브로커 | 추상화 레이어 / 페이퍼=Alpaca / 라이브=KIS |
| 8 | 언어 스택 | Go + Python 풀 하이브리드 |
| 9 | 폴더 격리 | go/, research/ 각자 독립 빌드·테스트 |
| 10 | DB | Postgres 16 + TimescaleDB extension (Docker) |
| 11 | 지표 활용 | feature 입력으로만, 결정 규칙 금지 |

## 4. 핵심 설계 결정 (Architecture Rules)

`docs/ARCHITECTURE.md`에 영구 기록될 룰들. 모든 후속 작업은 이 룰을 위반하지 않아야 한다.

### R1. 모든 리스크 관리는 브로커 측 GTC 주문에 위임한다

- **Why**: 로컬 컴퓨터가 다운되어도 손실 한도가 보장되어야 함. 봇이 죽었을 때 포지션이 무한 노출되면 안 됨.
- **How**: 진입 주문 시 Stop-Loss / Take-Profit GTC 주문을 동시에 등록(가능하면 OCO/bracket 한 번에). 봇 측 가격 감시로 매도하는 패턴 금지.
- **브로커 capability 강제**: R5 어댑터 인터페이스는 `SupportsBracketGTC() bool` 또는 동등한 capability 검사를 노출해야 하며, 이를 지원하지 못하는 브로커는 라이브 단계 사용 금지. 페이퍼 단계는 capability 검증 + 시뮬레이션으로 우회 허용.
- **Phase 순서 강제**: Phase 6 페이퍼 라이브 첫 주문 시점부터 R1 적용. 즉 Phase 6에 GTC bracket 발주 로직이 포함되어야 하며, Phase 7은 "추가" 안전장치(일일 손실 한도, 글로벌 킬스위치)를 다룸. R1의 기본 GTC는 Phase 7로 이연 불가.

### R2. 상태는 무조건 DB, 인메모리 금지

- **Why**: 재시작 시 상태 복구가 가능해야 함. 컴퓨터가 언제든 꺼질 수 있는 환경.
- **How**: 모든 의사결정·주문·포지션은 DB에 즉시 기록. 시작 시 브로커 상태 ↔ 로컬 DB reconciliation 필수, 불일치 시 알림 + 자동 정지.
- **알림 채널 시작 기본값**: stderr + 회전 로그 파일(`logs/alerts-YYYY-MM-DD.log`). Phase 7에서 외부 채널(Slack/Discord/Telegram) 추가. Phase 6 페이퍼 라이브 단계는 stderr+파일로도 충분하다고 가정.

### R3. 주문은 idempotent하다

- **Why**: 재시작 시 같은 주문이 두 번 나가면 안 됨. 네트워크 재시도 안전성.
- **How**: 모든 주문에 `client_order_id`를 강제. 같은 ID로 재시도 시 브로커가 거부하도록 설계. ID 생성 규칙은 결정론적(`{instance}_{strategy}_{date}_{symbol}_{seq}`).
- **단일 인스턴스 가정**: Phase 0~9는 한 환경(페이퍼 또는 라이브)당 봇 인스턴스 1개를 가정. `{instance}` 토큰은 환경 식별자(`paper`/`live`/`test`)로 사용해 환경 간 충돌 방지. 같은 환경 내 다중 인스턴스 동시 실행은 미지원(Phase 10 이후 결정).
- **`{seq}` 생성**: Postgres sequence (`CREATE SEQUENCE order_seq_<strategy>_<date>`) 사용. 인메모리 카운터는 R2 위반이라 금지. 같은 (strategy, date) 내에서 단조 증가 보장.

### R4. Go ↔ Python 통신은 3가지 채널만 허용

- **Why**: 폴리글랏 의존 관계 폭발 방지. 격리 보장.
- **How**:
  - **Postgres 테이블** — 운영 핫 패스 (시그널, 주문, 포지션, 가격, features)
  - **Parquet 파일 `shared/artifacts/`** — 배치 산출물 (학습 모델, 시그널 스냅샷)
  - **CLI/HTTP** — 운영 도구만 (`go run cmd/cli/...`)
- **"운영 경로" 정의**: 라이브/페이퍼 거래 의사결정 또는 주문 실행에 직접 관여하는 코드 경로. 일회성 분석 노트북·CI 스크립트·관리자 도구·배치 재학습은 비운영 경로로 간주.
- **금지**: cgo, 운영 경로의 subprocess 호출, 같은 메모리 공유, 파일 락 기반 IPC
- **자동 재학습 트리거 (Phase 8)**: Go가 직접 Python을 subprocess로 호출하지 않음. 외부 스케줄러(cron/systemd timer 또는 Go의 별도 batch 프로세스)가 Python을 직접 실행. 이 트리거 경로는 운영 경로가 아니므로 R4 위반 아님.

### R5. 지표는 결정 규칙이 아닌 feature로만 도입한다

- **Why**: 단일 임계값 룰("RSI<30이면 매수")은 학술적으로 알파가 거의 없음. 데이터 스누핑 방지.
- **How**: 모든 지표는 `features` 테이블의 컬럼으로 추가. 전략은 features를 입력으로 받는 모델/공식이지 단순 if-else 아님.

### R6. Feature catalog는 단일 진실 원천이다

- **Why**: 어떤 feature가 왜 존재하는지 추적 불가능하면 시스템이 불투명해짐.
- **How**: `shared/contracts/features.md`(메타데이터 카탈로그)에 모든 feature의 정의/계산식/도입 가설/검증 결과/도입일 기록. 미등록 feature 코드 사용 금지.
- **`features` 테이블(R4) vs `features.md`(R6)의 관계**:
  - `features.md` = 카탈로그(메타데이터). "어떤 feature가 존재하고 왜 존재하는가"
  - Postgres `features` 테이블 = 실제 데이터. "각 (날짜, 종목)에 대한 feature 값"
  - 카탈로그에 등록되지 않은 컬럼이 테이블에 들어가면 R6 위반

### R7. Walk-forward 검증 안 거친 전략은 페이퍼도 금지

- **Why**: in-sample 백테스트는 거의 항상 거짓말함. 페이퍼 트레이딩 자원도 검증된 전략에만 할당.
- **How**: 새 전략은 walk-forward (rolling out-of-sample) 백테스트 통과 + quant-skeptic 검증 통과 후에만 페이퍼 단계 진입.

### R8. 페이퍼 트레이딩 6개월 미만 → 라이브 전환 금지

- **Why**: 통계적으로 유의미한 표본 확보 전 실거래 전환은 도박.
- **How**: 페이퍼 트레이딩 시작일을 DB에 기록. (a) 6개월 경과 + (b) 최소 거래 횟수 충족 + (c) 일일 손실 한도 미위반 모두 충족 시에만 라이브 전환 게이트 통과.

### R9. shared/schema/ 가 DB 스키마 단일 진실 원천

- **Why**: 양 언어가 같은 DB를 쓰는데 스키마를 코드에 중복 정의하면 drift 발생.
- **How**: SQL migration 파일이 유일한 스키마 정의. Go/Python은 이걸 읽어서 ORM/쿼리 생성. 코드에 테이블 DDL 하드코딩 금지.
- **명시적 금지**: SQLAlchemy `Base.metadata.create_all()` / `Alembic --autogenerate` 무검토 적용 / GORM `AutoMigrate` / `db.exec("CREATE TABLE ...")` 등 코드에서 DDL을 생성·실행하는 패턴 모두 금지. ORM은 쿼리 빌더로만 사용.
- **읽기 전용 모델**: 양 언어의 ORM 모델 클래스는 `shared/schema/`의 SQL을 사람이 읽고 수동 매핑한 것이어야 함. 자동 생성 도구를 쓸 경우 출력물이 SQL 원본과 일치하는지 CI에서 검증.

### R10. 빌드·테스트 독립성

- **Why**: 한쪽 언어 환경 없이도 다른 쪽 작업 가능해야 함. 의존 관계 격리의 보증.
- **How**: `cd go && make test`로 Python 환경 없이 Go만 테스트 가능, `cd research && make test`로 Go 없이 Python만 테스트 가능. 루트 `make test`는 두 디렉터리를 순차 호출하는 wrapper일 뿐.

## 5. 디렉터리 구조

```
quant-bot/
├── go/                          # Self-contained Go module
│   ├── go.mod                   # 빈 초기화 (Phase 0)
│   ├── cmd/                     # 실행 진입점 — Phase 0에선 빈 디렉터리
│   ├── internal/                # 비공개 패키지 — Phase 0에선 빈 디렉터리
│   ├── pkg/                     # 공개 패키지 (필요 시)
│   ├── testdata/                # 픽스처용 (Go 테스트는 _test.go 인라인이 표준)
│   └── Makefile                 # go-only 명령
│
├── research/                    # Self-contained Python project
│   ├── pyproject.toml           # 빈 초기화 (Phase 0, uv 기반)
│   ├── src/quant_research/      # Phase 0에선 빈 디렉터리
│   ├── notebooks/               # 일회성 탐색
│   ├── tests/
│   └── Makefile                 # python-only 명령
│
├── shared/
│   ├── schema/                  # SQL migrations (Phase 1부터 채움)
│   ├── contracts/               # Data contracts (JSON Schema, features.md)
│   └── artifacts/               # 학습 모델·시그널 스냅샷 (gitignore)
│
├── docker/
│   └── docker-compose.yml       # Postgres+TimescaleDB
│
├── docs/
│   ├── STATUS.md                # 현재 Phase 진행 상황
│   ├── ROADMAP.md               # Phase 1~10 로드맵
│   ├── ARCHITECTURE.md          # 시스템 구성 + R1~R10
│   └── superpowers/
│       ├── specs/               # 본 문서 위치
│       └── plans/               # 구현 계획 (Phase 1부터)
│
├── .claude/
│   └── agents/                  # 6명 에이전트 정의
│
├── CLAUDE.md                    # 프로젝트 단위 하네스 룰
├── Makefile                     # 루트 편의 명령 (양쪽 호출)
├── .gitignore
└── README.md
```

## 6. 에이전트 팀 (6명)

`.claude/agents/` 하위에 각 에이전트 정의 파일을 작성. 다른 세션이 들어와도 같은 전문가 팀을 그대로 사용하기 위함.

**파일 형식**: Claude Code 표준 에이전트 정의 형식을 따른다.

```markdown
---
name: <agent-name>
description: <언제 호출해야 하는가 — 1~2줄>
tools: Read, Write, Edit, Bash, Glob, Grep   # 권한 화이트리스트
---

<system prompt 본문 — 역할, 핵심 원칙, 작업 흐름, 금지 사항>
```

| 에이전트 | 역할 | 도구 권한 | 시스템 프롬프트 핵심 원칙 |
|---------|------|----------|---------------------------|
| `quant-strategist` | 전략 설계, 백테스트, 모델 훈련 (Python) | Read, Write, Edit, Bash, Glob, Grep | walk-forward 검증 의무, in-sample 신뢰 금지, 모든 가정을 명시, **주문/실행 API 직접 호출 금지, `live`·`paper` 브로커 키 접근 금지** (research/ 외부 디렉터리 쓰기 금지) |
| `execution-engineer` | 브로커 어댑터, 주문 관리, 실행 엔진 (Go) | Read, Write, Edit, Bash, Glob, Grep | R1·R2·R3 강제, idempotency, reconciliation |
| `data-engineer` | 데이터 파이프라인, 스키마 (Go+Python) | Read, Write, Edit, Bash, Glob, Grep | point-in-time 보장, look-ahead 방지, 출처/타임스탬프 보존 |
| `quant-skeptic` | 적대적 전략 검증 | Read, Glob, Grep, Bash, WebSearch | "이 전략은 작동하지 않는다"가 기본 입장, 증명 책임을 strategist에 |
| `risk-reviewer` | 리스크 변경 검증 | Read, Glob, Grep (의도적 read-only) | "봇이 죽을 때 최대 손실은?"을 항상 답하게 함 |
| `docs-keeper` | 하네스 문서 동기화 | Read, Edit, Glob, Grep, Bash (`git log`) | "구현 끝났지만 문서 안 됐다 = 미완료" |

### 사용 흐름 (참고)

```
새 전략 추가:
  1. brainstorming (사용자 ↔ Claude)
  2. quant-strategist → 백테스트 + Python 코드
  3. quant-skeptic → 적대적 검증
  4. (통과) execution-engineer → Go 실행 코드
  5. risk-reviewer → 리스크 영향 검증
  6. superpowers:code-reviewer → 일반 코드 품질
  7. docs-keeper → STATUS/ROADMAP/ARCHITECTURE 업데이트
```

## 7. 하네스 문서 (5개)

### 7.1 CLAUDE.md (프로젝트 루트)

- 사용자 글로벌 CLAUDE.md를 보완하는 프로젝트 특화 룰
- 현재 Phase 명시
- 에이전트 호출 가이드 (어떤 작업에 어떤 에이전트)
- 빠른 네비게이션 (STATUS/ROADMAP/ARCH 링크)
- **R1~R10 요약 표 (강제 형식)**: `| 룰 | 1줄 요약 | spec 링크 |` 컬럼. 본문 복사 금지. 예:

  ```markdown
  | R1 | 모든 리스크 관리는 브로커 측 GTC 위임 | [spec §4](docs/superpowers/specs/2026-05-02-foundation-design.md#r1) |
  ```

- **구현 실행 방식 (MANDATORY)** — 본 프로젝트의 모든 plan은 `superpowers:subagent-driven-development`로 실행한다. Inline Execution(`superpowers:executing-plans`)은 사용하지 않는다. 이유: (a) Task별 fresh subagent 디스패치로 메인 컨텍스트 보존, (b) Task 사이 spec/code 리뷰 단계 자동 삽입, (c) 사용자가 정한 "작업별 전문 에이전트 최적 활용" 철학과 부합.

- **스펙 자체 검토 사이클 (MANDATORY)** — 본 프로젝트의 모든 spec(`docs/superpowers/specs/*.md`)은 다음 절차를 거친다:
  1. **1차 자체 검토 (자동/필수)**: 작성 직후 Critical / Important / Minor 분류로 이슈 식별 → 사용자에게 보고 → 인라인 패치 → §검토 이력에 기록
  2. **2차 이상 검토 (조건부 자동)**: 1차에서 Critical 또는 Important 이슈가 1건이라도 발견되면 패치 후 자동으로 2차 검토 진행 (한계 효용 체감 시까지). 1차에서 Minor만 나왔으면 2차는 사용자 요청 시에만.
  3. **추가 라운드**: 사용자가 명시적으로 요청하면 N차까지 진행. 매 라운드는 직전 라운드 결과를 입력으로 받아 새로운 시각으로 점검.
  4. **검토 이력 기록**: 라운드별로 발견 이슈 수(Critical/Important/Minor) + 주요 패치 요약을 §검토 이력 표에 한 줄씩 추가. 검토 없는 spec은 미완성으로 간주.
  5. **근거**: 본 프로젝트의 2026-05-02 Foundation Design 작성 시 1차 검토에서 Critical 3건, 2차 검토에서 Critical 2건이 추가 발견됨 → 단일 검토는 명백히 불충분함이 실증됨.

### 7.2 docs/STATUS.md

- Phase별 체크리스트 (Phase 0~10)
- 현재 Phase: 0 (진행 중)
- 알려진 결함 섹션 (현재 비어있음, 발견 시 즉시 추가)
- 최근 변경 이력 (역시간순)
- 마지막 업데이트 날짜

### 7.3 docs/ROADMAP.md

- Phase 1~10 상세 (아래 8절 표 참고)
- 현재 추천 다음 작업: Phase 1 (데이터 인제스트)
- Tier 1 (필수) / Tier 2 (권장) / Tier 3 (선택) 작업 분류

### 7.4 docs/ARCHITECTURE.md

- 시스템 구성 도식 (Mermaid)
- R1~R10 핵심 설계 결정 — **요약 표 + 본 spec(`docs/superpowers/specs/2026-05-02-foundation-design.md` §4) 링크**. 본문을 복사하지 않음 (drift 방지).
- 데이터 흐름 다이어그램 (Mermaid)
- 통신 채널 명세 (Postgres / Parquet / CLI)
- 컴포넌트 책임 분리 (Go execution / Python research / shared)
- 신규 룰(R11+) 추가는 ARCHITECTURE.md 본문에 신규 섹션으로, 기존 룰 변경은 spec 개정 후 ARCHITECTURE.md 요약 갱신

### 7.5 docs/superpowers/{specs,plans}/

- specs/ — 기능별 상세 설계 (본 문서가 첫 번째)
- plans/ — 기능별 구현 계획 (Phase 1부터)

## 8. Database 초기 설정

### docker-compose.yml

```yaml
services:
  db:
    image: timescale/timescaledb:latest-pg16
    container_name: quant-bot-db
    environment:
      POSTGRES_DB: quantbot
      POSTGRES_USER: quantbot
      POSTGRES_PASSWORD: ${DB_PASSWORD:-changeme}
    ports:
      - "5432:5432"
    volumes:
      - quant-bot-data:/var/lib/postgresql/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U quantbot -d quantbot"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  quant-bot-data:
```

### 초기 스키마

- `shared/schema/` 디렉터리 + README만 생성. 마이그레이션 파일은 Phase 1부터.
- 마이그레이션 도구 결정은 Phase 1로 이연 (`golang-migrate` vs `goose` 후보).

### 보안 노트

- 비밀번호는 `.env` (gitignore)에서 주입. 기본값은 `changeme` 플레이스홀더.
- `.env.example`을 커밋해 키 목록만 노출 (`DB_PASSWORD=`). 실제 `.env`는 사용자가 복사해 채움.
- Phase 0의 docker-compose는 로컬 5432 포트 노출 (개발 편의). Phase 6 라이브 전환 직전에 보안 강화 재검토.

## 9. 로드맵 (Phase 1~10)

| Phase | 내용 | 산출물 |
|-------|------|--------|
| 0 (이번) | 골격 + 하네스 + 룰 | 디렉터리/문서/에이전트 정의 |
| 1 | 데이터 인제스트 (Go) | FRED·Alpaca 가격 수집기 → Postgres |
| 2 | Feature Engineering (Python) | 가격/거래량/거시 features 테이블 |
| 3 | 백테스트 엔진 + Clenow Momentum | walk-forward 백테스트 결과 |
| 4 | Yield Curve Regime Filter 추가 | 멀티 시그널 결합 결과 |
| 5 | 브로커 추상화 + Alpaca 어댑터 (단위) | 인터페이스 + 어댑터 단위 테스트 통과 |
| 6 | 실행 엔진 (Go) + 페이퍼 자동 사이클 (R1 GTC 포함) | 어댑터를 사용한 자동 주문·청산, 페이퍼 계좌 |
| 7 | 추가 안전장치 (일일 손실 한도, 글로벌 킬스위치, 외부 알림 채널) | Phase 6의 기본 GTC 위에 운영 안전망 |
| 8 | Champion/Challenger 파이프라인 | 자동 재학습·교체 워크플로우 |
| 9 | KIS 어댑터 (라이브용) | 두 번째 어댑터, 추상화 검증 |
| 10 | 페이퍼→라이브 전환 게이트 | 6개월 페이퍼 통계 검토 후 결정 |

각 Phase는 자체 spec → plan → implement → review 사이클을 가진다.

## 10. Phase 0 완료 기준 (Acceptance Criteria)

- [ ] 디렉터리 구조 §5에 명시한 대로 모두 생성
- [ ] `CLAUDE.md`, `docs/STATUS.md`, `docs/ROADMAP.md`, `docs/ARCHITECTURE.md` 작성 완료
- [ ] 6개 에이전트 정의 파일 (`.claude/agents/*.md`) 작성 완료, §6에 명시한 표준 frontmatter 형식 준수
- [ ] `docker/docker-compose.yml` 작성 완료
- [ ] `make up` → `make db-check` 순으로 실행 시 Postgres 컨테이너 기동 + `docker compose exec db pg_isready -U quantbot -d quantbot` 통과 (호스트 psql 의존 X)
- [ ] `.env.example` 커밋, 실제 `.env`는 gitignore
- [ ] `go/` 에 빈 `go.mod` 초기화 (모듈 경로는 Phase 0 setup 시 사용자와 합의)
- [ ] `research/` 에 빈 `pyproject.toml` 초기화 (uv 기반 — §11에서 확정)
- [ ] 루트 `Makefile` 작성 (`up`, `down`, `db-check`, `test`, `fmt`, `lint` 명령)
- [ ] `README.md` 작성 (프로젝트 개요, 시작하기, 디렉터리 안내)
- [ ] `.gitignore` 작성 (Go, Python, OS, IDE, `shared/artifacts/`, `.env` 포함)
- [ ] `git init` + 초기 커밋 완료 (docs-keeper가 `git log` 사용 가능해야 함)
- [ ] 새 세션이 들어와 `cat docs/STATUS.md` 만 읽어도 "현재 Phase 0 완료, 다음은 Phase 1" 파악 가능

## 11. 미결정 사항 (Phase 1 이후 결정)

- **Python 패키지 매니저**: `uv` 확정 (가장 빠름, 표준 pyproject.toml, 사실상 차세대 표준). Phase 0 구현 시 사용자가 다른 의견 있으면 재논의 가능.
- **Go 모듈 경로**: Phase 0 구현 시작 시 사용자 합의 (예: `github.com/yuhojin/quant-bot/go` 또는 `quantbot/go`)
- DB migration 도구 (`golang-migrate` vs `goose`) — Phase 1 시작 시 결정
- 백테스트 라이브러리 (`vectorbt` vs `zipline-reloaded` vs 커스텀) — Phase 3
- 모델 직렬화 형식 — Phase 8 Champion/Challenger 시점
- 자본 규모 — 페이퍼 종료 후 사용자 결정
- 알림 채널 (Slack/Discord/Telegram) — Phase 7 시점 (Phase 0~6 기본값: stderr+로그파일, R2 참조)
- 실행 스케줄러 (cron / systemd / Go 내장 스케줄러) — Phase 6
- R7 "백테스트 통과" 정량 기준 (Sharpe·CAGR·MaxDD 임계) — Phase 3 spec 작성 시
- R8의 "최소 거래 횟수" 정량 기준 — Phase 10 게이트 설계 시점
- **TimescaleDB 이미지 버전 핀**: Phase 0은 `latest-pg16`, Phase 1 첫 마이그레이션 작성 시 특정 버전 태그(예: `pg16-2.20.0`)로 핀 고정
- **`research/notebooks/` 커밋 정책**: 전체 커밋 vs `nbstripout` vs 산출물 별도 분리 — Phase 2 시작 시 결정
- **DB 백업·복구 전략**: 로컬 pg_dump 자동화 vs WAL 아카이브 — Phase 6 라이브 진입 전 결정

## 12. 검토 이력

| 날짜 | 검토 종류 | 결과 |
|------|----------|------|
| 2026-05-02 | 작성 직후 1차 자체 검토 | Critical 3건(C1·C2·C3), Important 5건(I1·I2·I3·I4·I6), Minor 4건(M2·M3·M4·M5) 식별 → 모두 인라인 패치. 주요 변경: client_order_id에 instance 토큰(R3), features 테이블 vs catalog 관계(R6), "운영 경로" 정의(R4), `go/test/` → `go/testdata/`, `.env.example`·`make db-check`·`git init`을 §10에 추가, ARCHITECTURE.md는 spec을 참조 |
| 2026-05-02 | 2차 자체 검토 (사용자 요청) | Critical 2건(C4·C5), Important 5건(I7·I8·I9·I11·I12·I13), Minor 5건(M6·M7·M9·M10·M12) 식별 → 모두 인라인 패치. 주요 변경: R1에 브로커 capability 강제 + Phase 6/7 순서 명시, R2에 알림 채널 시작 기본값(stderr+로그), R3에 `{seq}` 생성 메커니즘(Postgres sequence), R4에 자동 재학습은 외부 스케줄러로 명시, R9에 ORM 자동 DDL 명시적 금지, quant-strategist 권한 제약 강화, CLAUDE.md R요약 표 형식 강제, Phase 5/6/7 표현 명확화, db-check를 컨테이너 내 `pg_isready`로 변경, §11에 Go 모듈 경로·이미지 버전 핀·노트북 정책·백업 전략 추가 |
| 2026-05-02 | 사용자 승인 + 메타룰 추가 | 사용자가 "다중 라운드 자체 검토" 패턴을 프로젝트 표준으로 채택 요청 → §7.1 CLAUDE.md 명세에 "스펙 자체 검토 사이클 (MANDATORY)" 5단계 절차 추가. 본 spec 승인. |
| 2026-05-02 | 실행 방식 룰 추가 | 사용자가 "항상 Subagent-Driven 실행"을 프로젝트 표준으로 채택 → §7.1 CLAUDE.md 명세에 "구현 실행 방식 (MANDATORY)" 룰 추가. Inline Execution 사용 금지. |
