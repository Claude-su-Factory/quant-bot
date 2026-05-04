# Project Status

**현재 Phase**: Phase 1b-A 완료. 다음: Phase 1b-B — Alpaca 가격 + EDGAR 재무제표 수집기
**마지막 업데이트**: 2026-05-04

## Phase 진행 상황

- [x] **Phase 0** — 골격 + 하네스 + 룰 (2026-05-02 완료)
- [x] Phase 1a — Foundation Infrastructure (Go) (2026-05-03 완료)
- [x] Phase 1b-A — 인프라 + FRED 수집기 (2026-05-04 완료)
- [ ] Phase 1b-B — Alpaca + EDGAR 수집기
- [ ] Phase 2 — Python Foundation Infra + Feature Engineering
- [ ] Phase 3 — 백테스트 엔진 + Clenow Momentum
- [ ] Phase 4 — Yield Curve Regime Filter
- [ ] Phase 4.5 — Quality 팩터 결합 (재무제표 기반 종목 필터)
- [ ] Phase 5 — 브로커 추상화 + Alpaca 어댑터 (단위)
- [ ] Phase 6 — 실행 엔진 + 페이퍼 자동 사이클 (R1 GTC 포함)
- [ ] Phase 7 — 추가 안전장치 (일일 손실 한도, 글로벌 킬스위치, 외부 알림)
- [ ] Phase 8 — Champion/Challenger 파이프라인
- [ ] Phase 9 — KIS 어댑터 (라이브용) [조건부 — 사용자 라이브 결정 후]
- [ ] Phase 10 — 페이퍼→라이브 전환 게이트 [조건부 — Phase 9와 동시]

## 알려진 결함

(없음)

## 최근 변경 이력

- **2026-05-04** Phase 1b-A 완료 — TimescaleDB 마이그레이션 (goose), retry helper, repo 패턴, FRED 인제스터 (4 시리즈 × 20년 백필), CLI (ingest/status/migrate/version), LaunchAgent 자동 셋업 (사용자 매일 부담 0), R14 도입, v0.2.0-phase1b-a 태그
- **2026-05-03** Phase 1a 완료 — Go 인프라 4종(설정 로더·로거·DB 풀·에러 패턴) 구현, R11~R13 도입, test-integration target 신설, v0.1.0-phase1a 태그
- **2026-05-03** 오너 모드 룰 + Phase 1a/1b 분리 + Foundation Infra spec — Claude 오너 모드 룰 신설(CLAUDE.md), Phase 1을 1a(Foundation Infra) + 1b(Data Ingest) 분리, Python 인프라 Phase 2로 이연 (YAGNI), Phase 1a spec(`2026-05-03-phase1a-foundation-infra-design.md`) 작성·자체 검토 4라운드
- **2026-05-02** Option A 강화 + 재무제표 도입 + Phase 4.5 신설 — R8 강화 (페이퍼 12개월 + 정량 게이트 7종 + 사용자 명시 결정), Phase 1에 EDGAR 재무제표 수집 추가, Phase 2에 펀더멘털 지표 추가, Phase 4.5 (Quality 팩터) 신설, Phase 9·10 조건부 표기
- **2026-05-02** 에이전트 모델 핀 고정 — 6개 에이전트 frontmatter에 `model:` 추가 (strategist/execution/data=sonnet, skeptic/risk=opus, docs-keeper=haiku). 호출 컨텍스트 무관하게 일관된 모델 사용
- **2026-05-02** 룰 보강 — Phase 1+ code task에 TDD(`test-driven-development` 스킬), 코드 리뷰(`requesting-code-review` 스킬), 디버깅(`investigate` 스킬) 강제 룰 추가 (CLAUDE.md / spec / plan 동기화)
- **2026-05-02** Phase 0 완료 — 디렉터리 골격, 하네스 문서 5종, 6명 에이전트 정의, Docker Postgres+TimescaleDB 인프라 구축
- **2026-05-02** Phase 0 시작 — 브레인스토밍 완료, foundation spec 승인, 구현 plan 작성

## 업데이트 규칙

- 기능 구현 완료 시 해당 항목을 ✅로 변경
- "최근 변경 이력" 맨 위에 한 줄 추가 (역시간순)
- "마지막 업데이트" 날짜 갱신
- 미구현 결함 발견 시 "알려진 결함"에 즉시 추가
