# config/

봇 설정 파일이 위치한다.

## 처음 셋업

```bash
cp config.example.toml config.toml
# config.toml 열어 실제 값 채우기 (DB 비밀번호, API 키 등)
```

## 파일 안내

| 파일 | git 추적 | 설명 |
|------|---------|------|
| `config.example.toml` | ✅ 커밋 | 키 목록 템플릿. placeholder 값. 새 키 추가 시 함께 갱신 (R11) |
| `config.toml` | ❌ gitignore | 실제 값. 비밀 포함. 절대 커밋되지 않음 |
| `README.md` | ✅ 커밋 | 본 안내 |

## 키 의미

각 키의 의미·검증 규칙은 spec [`docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md`](../docs/superpowers/specs/2026-05-03-phase1a-foundation-infra-design.md) §5.2~5.3 참조.

## 환경 (`general.environment`)

| 값 | 동작 |
|----|------|
| `dev` | 개발용. API 키 비어 있어도 경고만 |
| `test` | 테스트용. API 키 비어 있어도 경고만 |
| `paper` | 페이퍼 트레이딩. API 키 필수 |
| `live` | 실거래. API 키 필수 (R8 게이트 통과 후 사용) |
