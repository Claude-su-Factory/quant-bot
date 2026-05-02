# Roadmap

**현재 추천 다음 작업**: Phase 1 — 데이터 인제스트 (Go)

## Phase 상세

### Phase 1 — 데이터 인제스트 (Go) [Tier 1 필수]

데이터 출처 3곳에서 자동 수집 (모두 무료):
- FRED 거시경제 지표 수집기 (장단기 금리차 T10Y2Y, 변동성지수 VIX, 신용 스프레드 BAMLH0A0HYM2 등)
- Alpaca 일봉 가격 수집기 (S&P 500 종목군 — 미국 대형주 500개)
- **EDGAR 재무제표 수집기** — 미국 증권거래위원회 SEC 공시 분기·연간 재무제표 (10-Q, 10-K). 무료
- Postgres TimescaleDB 시계열 테이블 스키마 (`shared/schema/`)
- DB 마이그레이션 도구 결정 (`golang-migrate` vs `goose`)
- TimescaleDB 이미지 버전 핀 고정 (`latest-pg16` → 특정 버전)

### Phase 2 — Feature Engineering (Python) [Tier 1 필수]

수집한 원시 데이터에서 매매에 쓸 지표(features)를 계산:
- `shared/contracts/features.md` 카탈로그 초기화 (R6)
- 가격 지표 (수익률 1일·5일·21일·63일·252일, 모멘텀 12-1, ATR 평균변동폭)
- 거래량 지표 (평균 대비 거래량 비율, OBV 누적 거래량)
- 거시 지표 (장단기 금리차로 시장 국면 분류)
- **펀더멘털 지표 (재무 건전성)** — 자기자본수익률 ROE, 부채비율, 매출성장률, 영업이익률 등. EDGAR 데이터로부터 계산
- `features` 테이블 적재
- `research/notebooks/` 커밋 정책 결정

### Phase 3 — 백테스트 엔진 + Clenow Momentum [Tier 1 필수]

- 백테스트 라이브러리 결정 (`vectorbt` vs `zipline-reloaded` vs 커스텀)
- Clenow Momentum 전략 (Stocks on the Move 기준 — 최근 12개월 잘 오른 종목 따라 매수)
- Walk-forward 검증 (과거 데이터를 시간순으로 잘라가며 미래 데이터로 검증, 과적합 방지)
- R7 "통과" 정량 기준 정의 (Sharpe·연수익률·최대 낙폭 임계값)
- quant-skeptic 적대적 검증 통과

### Phase 4 — Yield Curve Regime Filter 추가 [Tier 1 필수]

가격 시그널 + 거시 시장 국면 결합:
- 거시 국면 감지 (장단기 금리차 T10Y2Y 기반 — 음수면 경기침체 신호)
- Clenow Momentum과 결합 (위험 국면일 때 포지션 크기 자동 축소)

### Phase 4.5 — Quality 팩터 결합 (재무제표 기반 종목 필터) [Tier 1 필수]

가격이 잘 오르는 종목 중에서도 재무적으로 건강한 회사만 선별:
- Quality 팩터 정의 (수익성·재무 안정성·매출 성장 가중 결합)
- Phase 2에서 만든 펀더멘털 지표 활용
- 모멘텀 + 거시 + 재무 건전성 멀티 시그널 백테스트
- 단일 모멘텀 대비 추가 알파 입증 (walk-forward, 다중 시장 국면)
- 학술 근거: AQR "Quality Minus Junk" (Asness et al.), Novy-Marx profitability factor

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

### Phase 9 — KIS 어댑터 (라이브용) [조건부 — 사용자 라이브 결정 후 시작]

**사용자가 "실거래 시작" 결정한 시점부터 진행. 그 전까진 Alpaca 페이퍼만 사용.**
- 한국투자증권 해외주식 API 어댑터 (한국 거주자 미국 주식 실거래용)
- 추상화 누수 검증 (인터페이스 변경 없이 두 번째 어댑터 추가 가능해야)

### Phase 10 — 페이퍼→라이브 전환 게이트 [조건부 — Phase 9와 동시]

R8의 7가지 통과 기준 자동 검사 + 사용자 명시 결정:
- 페이퍼 트레이딩 12개월 경과 확인
- Sharpe ratio 1.0 이상 (95% 신뢰구간 하한)
- SPY 대비 연환산 +2% 이상 (양도세·환전·수수료 차감 후)
- 최대 낙폭 25% 이내
- 백테스트 ↔ 페이퍼 결과 차이 30% 이내
- 6개월 무중단 + critical 버그 0건
- **사용자가 "라이브 전환" 명시 결정** (자동 통과만으로 시작 안 함)
- DB 백업·복구 전략 확정 (`pg_dump` vs WAL)

## Tier 분류

- **Tier 1 (필수)**: Phase 1, 2, 3, 4, 4.5, 5, 6, 7
- **Tier 2 (권장)**: Phase 8 (없어도 라이브 운영은 가능)
- **Tier 3 (조건부)**: Phase 9, 10 (사용자가 라이브 전환 결정 시에만 시작)

## 업데이트 규칙

- 완료된 Phase는 본 문서에서 제거 (이력은 STATUS.md)
- 새 결정/요구사항으로 미결정 사항이 늘어나면 해당 Phase 섹션에 추가
- "현재 추천 다음 작업"은 Phase 완료 시점에 재설정
