# Roadmap

**현재 추천 다음 작업**: Phase 1 — 데이터 인제스트 (Go)

## Phase 상세

### Phase 1 — 데이터 인제스트 (Go) [Tier 1 필수]

- FRED 거시 데이터 수집기 (T10Y2Y, VIX, BAMLH0A0HYM2 등)
- Alpaca 일봉 가격 수집기 (S&P 500 universe)
- Postgres TimescaleDB hypertable 스키마 (`shared/schema/`)
- DB migration 도구 결정 (`golang-migrate` vs `goose`)
- TimescaleDB 이미지 버전 핀 고정 (`latest-pg16` → 특정 버전)

### Phase 2 — Feature Engineering (Python) [Tier 1 필수]

- `shared/contracts/features.md` 카탈로그 초기화 (R6)
- 가격 features (수익률 1d/5d/21d/63d/252d, 모멘텀 12-1, ATR)
- 거래량 features (volume ratio, OBV)
- 거시 features (yield curve regime)
- `features` 테이블 적재
- `research/notebooks/` 커밋 정책 결정

### Phase 3 — 백테스트 엔진 + Clenow Momentum [Tier 1 필수]

- 백테스트 라이브러리 결정 (`vectorbt` vs `zipline-reloaded` vs 커스텀)
- Clenow Momentum 전략 (Stocks on the Move 기준)
- Walk-forward (rolling out-of-sample) 검증
- R7 "통과" 정량 기준 정의 (Sharpe·CAGR·MaxDD 임계)
- quant-skeptic 적대적 검증 통과

### Phase 4 — Yield Curve Regime Filter 추가 [Tier 1 필수]

- 거시 레짐 감지 (T10Y2Y 기반)
- Clenow Momentum과 결합 (포지션 사이즈 조절)

### Phase 5 — 브로커 추상화 + Alpaca 어댑터 (단위) [Tier 1 필수]

- `BrokerAdapter` 인터페이스 정의
- `SupportsBracketGTC()` capability (R1)
- Alpaca 어댑터 단위 테스트 통과 (실제 주문 X)

### Phase 6 — 실행 엔진 + 페이퍼 자동 사이클 (R1 GTC 포함) [Tier 1 필수]

- Go 실행 엔진 (스케줄러, 주문 관리)
- 페이퍼 계좌 자동 주문 + GTC bracket 동시 발주 (R1 강제)
- 시작 시 reconciliation (R2)
- `client_order_id` Postgres sequence 기반 (R3)
- 실행 스케줄러 결정 (cron / systemd / Go 내장)

### Phase 7 — 추가 안전장치 [Tier 1 필수]

- 일일 손실 한도
- 글로벌 킬스위치
- 외부 알림 채널 (Slack/Discord/Telegram 결정)

### Phase 8 — Champion/Challenger 파이프라인 [Tier 2 권장]

- 자동 재학습 (cron/systemd timer)
- 모델 직렬화 형식 결정
- 챔피언 교체 게이트

### Phase 9 — KIS 어댑터 (라이브용) [Tier 1 필수]

- 한국투자증권 해외주식 API 어댑터
- 추상화 누수 검증 (인터페이스 변경 없이 추가 가능해야)

### Phase 10 — 페이퍼→라이브 전환 게이트 [Tier 1 필수]

- R8 정량 기준 정의 (최소 거래 횟수)
- 6개월 페이퍼 통계 검토 게이트
- DB 백업·복구 전략 확정 (`pg_dump` vs WAL)

## Tier 분류

- **Tier 1 (필수)**: Phase 1, 2, 3, 4, 5, 6, 7, 9, 10
- **Tier 2 (권장)**: Phase 8 (없어도 라이브 운영은 가능)
- **Tier 3 (선택)**: 현재 없음

## 업데이트 규칙

- 완료된 Phase는 본 문서에서 제거 (이력은 STATUS.md)
- 새 결정/요구사항으로 미결정 사항이 늘어나면 해당 Phase 섹션에 추가
- "현재 추천 다음 작업"은 Phase 완료 시점에 재설정
