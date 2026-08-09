# 변경 이력

## Unreleased

### 추가

- 식품안전나라 C001, I2821, I0250, I0470을 직렬 수집하는 독립 Cobra 명령 `collect-company-registry`를 추가했다.
- 네 API의 모든 필드, row 원문 JSON, gzip 응답 원문, HTTP 메타데이터를 보존하는 source별 raw 4개를 포함해 원장 7개 테이블을 추가했다.
- 기존 `items.importer_name`과 C001 업소명을 exact/normalized/ambiguous/unresolved로 판정하고 근거를 보존하는 매칭 원장을 추가했다.
- `FOODSAFETYKOREA_API_KEY`를 기존 `MFDS_API_KEY`와 분리해 암호화 환경변수 서브모듈에서 관리한다.

### 안전장치

- 공식 HTTPS endpoint, 요청당 1,000행, run당 최대 500회, 0.5 QPS, 직렬 호출을 강제한다.
- HTTP 200 HTML·빈 본문·비 JSON·wrapper 불일치·비정상 RESULT를 완료로 처리하지 않고 원문과 오류 분류를 저장한다.
- 날짜와 `PUBLIC_DT`는 의미를 보정하지 않고 원문 문자열로 저장하며, 모호한 업체는 자동 병합하지 않는다.

### 조사

- 실제 API/원장 결합 수치와 미확정 사항은 `docs/company-registry-api-audit.md`에 기록했다.
