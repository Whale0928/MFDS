# 변경 이력

## Unreleased (크론잡 전용 단순화)

### 삭제

- 운영·개발 `CronJob`이 실행하지 않는 CLI 명령을 삭제했다. 남은 명령은 `collect-recent`와 `normalize`(`--limit`, `--rcno`, `--dry-run`, `--force`) 두 개다.
  - `collect`: `--from`, `--to`, `--workers`로 기간을 지정하던 수집 명령. 최근 7일 수집은 `collect-recent`가 맡는다.
  - `match`: 정제된 행의 매칭 백필 명령. 매칭은 `normalize`가 정제와 함께 계산한다.
  - `health`: 설정과 MySQL 연결 확인 명령.
  - `reference-sync`: 기준 데이터 동기화 명령과 `internal/reference` 패키지.
- 정제 결과 대시보드(`tools/normalization-dashboard`)와 Taskfile의 `dashboard:*` 태스크, `task health`를 삭제했다.
- 검증 스크립트 `tools/verification-harness.sh`를 삭제했다.
- `match` 백필 전용 코드(`internal/usecase/matching`, 저장소의 `ListMatchingSources`, `SaveMatchingResult`, `MatchingRemaining`)와 테스트를 삭제했다.
- 정제 결과 테이블의 후보 1~3 슬롯 컬럼을 더 이상 쓰지 않는다. 매칭 후보는 `mfds_matching_candidates` 이력에 계속 기록한다.

## 0.2.1

### 변경

- 재정제(`STALE`, `--rcno`, `--force`)와 `match` 백필이 관리자 확정(`CANDIDATE`, `MANUAL`)과 상속(`INHERITED`) 행의 선택 알코올·증류소·리전 ID, 결정, 출처, `inherited_from_declaration_id`를 덮어쓰지 않는다. 후보 슬롯과 매칭 실행 기록은 계속 최신 매처 결과로 갱신한다.
- 선택값이 보존된 행에는 `mfds_matching_selections`에 `AUTO` 선택 이력을 남기지 않는다.
- 정제 규칙을 `mfds-normalization-v4`로 올렸다. 괄호 안·문자열 끝 퍼센트와 `ALC` 앵커는 몰트·그레인·인삼 같은 품목 단어가 곁에 있어도 병 도수로 읽고, 성분 판정은 `함유`, 향·추출물 같은 함량 서술, `100% 호밀`로 한정한다. 20%를 넘는 성분 값은 검토 사유 `INGREDIENT_PERCENT_ABOVE_AUTOMATIC_RANGE`를 붙이되, `함유`나 `100% 원재료`처럼 함량을 직접 서술한 표기는 예외로 둔다.
- 한글 `N도`는 뒤가 공백·괄호·끝이면 문장 가운데에서도 도수로 읽는다(`56도 프리미엄금문고량주`, `북경이과두주(56도)`).
- `AGED n YEARS`의 `YEARS`를 이름에서 함께 제거하고, `주년`·`ANNIVERSARY`로 쓰인 숫자는 숙성연수로 쓰지 않는다(`HENNESSY VS 260YEARS`).
- 규칙 버전 변경은 기존 행을 자동으로 `STALE`로 바꾸지 않는다.

### 성능

- 정제 시작 시 수입사 연결 동기화를 신고별 조회·갱신 대신 UPDATE 한 번으로 처리한다. 연결 규칙은 같으며, 원격 개발 DB 기준 할 일이 없는 `normalize`가 약 10분에서 약 5초로 줄었다.
- 동일성 키 채움을 500건 묶음당 UPDATE 한 번으로 저장한다.

### 추가

- 정제 결과에 제품 동일성 키 `product_identity_key_sha256`(한글·영문 검색 키, 도수, 숙성 연수, 스트렝스 표기. 용량·수입사 제외)을 저장한다. Flyway V21 컬럼을 사용한다.
- `normalize`가 정제를 마친 뒤 키가 없는 기존 정제 행에 저장된 정제값으로 동일성 키를 채운다.
- 정제 중 같은 동일성 키에 관리자 확정이 있으면 자동 매칭 계산을 건너뛰고 그 매칭을 바로 쓴다(`admin_match_reused`). 자동 확정은 기준으로 인정하지 않는다.
- `normalize`가 이어서 관리자 확정을 같은 동일성 키의 다른 신고로 이어받는다(`INHERITED`). 관리자 확정이 자동 매칭보다 우선해 자동 확정 행도 덮어쓴다.
- `normalize`가 마지막으로 알코올이 매칭된 모든 신고의 알코올명(`alcohol_name_ko/en`, 공개 수입 신고 화면 표시명)을 매칭된 알코올의 이름으로 덮어쓴다. 기본 제품명·검색 키·SKU 표시명은 원문 기준으로 유지하고, 매칭이 풀려도 이름은 되돌리지 않는다. 시드 충돌, 검토·범용명·위스키 외·제조국 불일치·관리자 해제·참조 중복 묶음을 제외하고, 시드 해제·삭제·재확정을 다음 실행에서 전파한다. `--dry-run`이면 건수만 출력한다.
- `normalize` 결과 줄에 `identity_filled`, `inherited`, `inheritance_released`, `inheritance_conflicts`, `alcohol_names_applied`를 추가했다. 새 명령이나 옵션은 없다.

## Unreleased

### 추가

- 수입식품 영업신고 정보(C001), 수입식품업 폐업정보(I2821), 우수수입업소 현황(I0250), 행정처분 결과(I0470)를 수집하는 독립 코브라(Cobra) 명령 `sync-company-registry`를 추가했다.
- 최초 `--since` 기준일과 마지막 완료일을 이용한 변경일자 증분 동기화를 추가했다. 변경일자 필터가 없는 우수수입업소 현황(I0250)은 매번 전체 조회한다.
- 네 API의 모든 필드, 행 원문 제이슨(JSON), gzip 응답 원문, HTTP 메타데이터를 보존하는 출처별 원본(raw) 4개를 포함해 원장 6개 테이블을 추가했다.
- 대시보드에 업체 공식정보 목록·상세 화면을 추가하고, 수입 기록 상세에서는 저장된 연결 없이 수입사명과 공식 업소명을 조회 시점에 비교한다.
- 잘못 도입됐던 업체 연결 근거 테이블과 현재 연결 보기(view)는 제거했다.
- `FOODSAFETYKOREA_API_KEY`를 기존 `MFDS_API_KEY`와 분리해 암호화 환경변수 서브모듈에서 관리한다.

### 안전장치

- 공식 HTTPS endpoint, 요청당 1,000행, run당 최대 500회, 0.5 QPS, 직렬 호출을 강제한다.
- HTTP 200 HTML·빈 본문·비 JSON·wrapper 불일치·비정상 RESULT를 완료로 처리하지 않고 원문과 오류 분류를 저장한다.
- 날짜와 공개기한(`PUBLIC_DT`, 행정처분 정보를 공개할 수 있는 기한)은 의미를 보정하지 않고 원문 문자열로 저장하며, 모호한 업체는 자동 병합하지 않는다.

### 조사

- 실제 API/원장 결합 수치와 미확정 사항은 `docs/company-registry-api-audit.md`에 기록했다.
