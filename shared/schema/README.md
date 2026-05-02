# shared/schema/

DB 스키마의 단일 진실 원천 (R9). 모든 SQL migration 파일이 여기에 위치한다.

## 룰

- Postgres + TimescaleDB 호환 SQL만 사용
- ORM auto-migration 산출물을 그대로 커밋하지 않음 (R9 명시적 금지 사항)
- 양 언어(Go/Python)의 ORM 모델은 본 디렉터리의 SQL을 사람이 읽고 수동 매핑

## Phase 1에서 채워질 내용

- migration 도구 결정 (`golang-migrate` vs `goose`) 후 파일 명명 규약 확정
- 첫 마이그레이션: TimescaleDB extension 활성화 + `prices_daily` hypertable
