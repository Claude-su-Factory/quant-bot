# CLAUDE.md (Project: quant-bot)

이 파일은 quant-bot 프로젝트의 작업 룰을 정의한다. 사용자 글로벌 `~/.claude/CLAUDE.md`를 보완한다.

## 현재 Phase

**Phase 0** — Project Skeleton & Harness Engineering

다음 Phase: **Phase 1** — 데이터 인제스트 (Go)

상세는 [`docs/STATUS.md`](docs/STATUS.md)와 [`docs/ROADMAP.md`](docs/ROADMAP.md) 참조.

## 빠른 네비게이션

- [`docs/STATUS.md`](docs/STATUS.md) — 현재 어디까지 됐나 (Phase별 체크리스트, 최근 변경)
- [`docs/ROADMAP.md`](docs/ROADMAP.md) — 다음 무엇을 할까 (Tier 1/2/3 작업)
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — 시스템 구성 + R1~R10 요약
- [`docs/superpowers/specs/`](docs/superpowers/specs/) — 기능별 상세 설계 (foundation 포함)
- [`docs/superpowers/plans/`](docs/superpowers/plans/) — 기능별 구현 계획

## 핵심 설계 룰 (R1~R10 요약)

상세는 [`docs/superpowers/specs/2026-05-02-foundation-design.md`](docs/superpowers/specs/2026-05-02-foundation-design.md) §4 참조. 본문 복사 금지 (drift 방지).

| 룰 | 1줄 요약 | spec 링크 |
|----|---------|----------|
| R1 | 모든 리스크 관리는 브로커 측 GTC 위임 (capability 강제) | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R2 | 상태는 무조건 DB, 인메모리 금지 (시작 시 reconciliation) | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R3 | 주문은 idempotent (deterministic client_order_id, Postgres sequence) | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R4 | Go ↔ Python 통신은 Postgres / Parquet / CLI 3채널만 | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R5 | 지표는 결정 규칙이 아닌 feature로만 도입 | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R6 | Feature catalog (`shared/contracts/features.md`)가 단일 진실 원천 | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R7 | Walk-forward 검증 안 거친 전략은 페이퍼도 금지 | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R8 | 페이퍼 12개월 + 정량 게이트 7종 + 사용자 명시 결정 시에만 라이브 전환 | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R9 | `shared/schema/`가 DB 스키마 단일 진실 (ORM auto-DDL 명시 금지) | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |
| R10 | 빌드·테스트 독립성 (`go/`·`research/` 각자 단독 실행 가능) | [§4](docs/superpowers/specs/2026-05-02-foundation-design.md) |

## 에이전트 호출 가이드

| 작업 | 호출할 에이전트 |
|------|----------------|
| 전략 설계, 백테스트, 모델 훈련 | `quant-strategist` |
| 브로커 어댑터, 주문 실행 (Go) | `execution-engineer` |
| 데이터 파이프라인, DB 스키마 | `data-engineer` |
| 새 전략·백테스트 적대적 검증 | `quant-skeptic` |
| 리스크 관련 변경 검증 | `risk-reviewer` |
| 문서 동기화 (STATUS/ROADMAP/ARCH) | `docs-keeper` |
| 일반 코드 리뷰 | `superpowers:code-reviewer` |
| 코드베이스 탐색 | `Explore` |

## 구현 실행 방식 (MANDATORY)

본 프로젝트의 모든 plan은 `superpowers:subagent-driven-development`로 실행한다. Inline Execution(`superpowers:executing-plans`)은 사용하지 않는다.

- **Why**: (a) Task별 fresh subagent 디스패치로 메인 컨텍스트 보존, (b) Task 사이 spec/code 리뷰 단계 자동 삽입, (c) 사용자가 정한 "작업별 전문 에이전트 최적 활용" 철학과 부합.
- **리뷰 단계 — task 성격에 따라 차등 적용**:
  - **Code task** (실행 가능 코드 또는 빌드/실행 인프라 변경 포함): implementer → spec reviewer → code-quality reviewer 3-stage 모두 진행.
  - **Doc-only task** (마크다운/JSON/YAML 데이터 파일·README·system prompt 등 실행 불가능 자산만): implementer → spec reviewer까지만 진행, code-quality 리뷰 생략.
  - 판단 기준: "이 task의 산출물에 버그가 있을 수 있는가?" 가능 → code task. 불가능(텍스트 일치만) → doc-only task.
- **Task 번들링 허용**: 동일 성격 doc 파일 다수(예: 에이전트 정의 6개)는 1 task로 묶어도 무방. 단, 커밋은 파일별로 분리해 원자성 유지.
- **Phase 1+ code task 추가 룰 (MANDATORY)**:
  - **TDD 강제**: implementer 프롬프트는 반드시 서브에이전트가 `superpowers:test-driven-development` 스킬을 호출하도록 명시한다. 사이클: 실패 테스트 작성 → 실행해 실패 확인 → 최소 구현 → 실행해 통과 확인 → 커밋. placeholder 테스트로 우회 금지.
  - **코드 리뷰는 스킬 형식 사용**: code-quality 리뷰 단계에서 `superpowers:code-reviewer` 에이전트를 직접 Agent 툴로 호출하지 말고 `superpowers:requesting-code-review` 스킬을 호출해 그 템플릿(BASE_SHA / HEAD_SHA / WHAT_WAS_IMPLEMENTED / DESCRIPTION)을 따른다.
  - **디버깅 시**: 버그·테스트 실패·예상치 못한 동작 발생 시 `superpowers:investigate` 스킬을 호출 (root cause 4-phase: investigate → analyze → hypothesize → implement). "iron law: no fixes without root cause".
  - **적용 시점**: Phase 0(scaffolding)은 코드 자산이 없어 면제. Phase 1부터 적용.

## 스펙 자체 검토 사이클 (MANDATORY)

본 프로젝트의 모든 spec(`docs/superpowers/specs/*.md`)은 다음 절차를 거친다:

1. **1차 자체 검토 (자동/필수)**: 작성 직후 Critical / Important / Minor 분류로 이슈 식별 → 사용자에게 보고 → 인라인 패치 → §검토 이력에 기록.
2. **2차 이상 검토 (조건부 자동)**: 1차에서 Critical 또는 Important 이슈가 1건이라도 발견되면 패치 후 자동으로 2차 검토 진행 (한계 효용 체감 시까지). 1차에서 Minor만 나왔으면 2차는 사용자 요청 시에만.
3. **추가 라운드**: 사용자가 명시적으로 요청하면 N차까지 진행. 매 라운드는 직전 라운드 결과를 입력으로 받아 새로운 시각으로 점검.
4. **검토 이력 기록**: 라운드별로 발견 이슈 수(Critical/Important/Minor) + 주요 패치 요약을 §검토 이력 표에 한 줄씩 추가.
5. **근거**: 2026-05-02 Foundation Design 작성 시 1차에서 Critical 3건, 2차에서 Critical 2건이 추가 발견됨 → 단일 검토는 명백히 불충분함이 실증됨.

## 문서 업데이트 규칙 (MANDATORY)

기능 구현 완료 시 다음 파일을 반드시 업데이트한다. 문서 업데이트 없이는 작업이 완료된 것으로 간주하지 않는다.

1. `docs/STATUS.md` — 해당 항목을 ✅로 이동, "최근 변경 이력" 맨 위에 한 줄 추가, "마지막 업데이트" 날짜 갱신
2. `docs/ROADMAP.md` — 완료된 항목 제거, 필요 시 "현재 추천 다음 작업" 재설정
3. `docs/ARCHITECTURE.md` — 아키텍처에 영향을 준 변경에만 반영 (새 컴포넌트, 설계 결정 등)

`docs-keeper` 에이전트가 이 작업을 담당.

## 하네스 엔지니어링 룰

작업 중 발견한 **규칙·판단 기준·프로젝트 결정**은 반드시 적절한 문서에 기록한다. 메모리나 대화 맥락에만 남기는 것은 허용되지 않는다.

| 성격 | 기록 위치 |
|------|----------|
| 프로젝트 전반 작업 룰 | 본 `CLAUDE.md` |
| 아키텍처 설계 결정 | `docs/ARCHITECTURE.md` (출처 spec 링크 필수) |
| 작업 흐름 / 문서 관리 룰 | 해당 문서 하단의 "업데이트 규칙" 섹션 |
| 기능별 상세 룰·트레이드오프 | `docs/superpowers/specs/<feature>-design.md` |
| 알려진 결함·미구현 이슈 | `docs/STATUS.md`의 "알려진 결함" + `docs/ROADMAP.md` |

## 시작 명령

```bash
make up        # Postgres+TimescaleDB 기동
make db-check  # 연결 확인
make down      # 정지
make test      # Go + Python 전체 테스트
make help      # 전체 명령 목록
```

상세는 [`README.md`](README.md) 참조.
