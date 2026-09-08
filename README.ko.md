# MFDS 수입주류 원장 수집기

[English](README.md)

식약처 수입식품정보마루의 공개 수입주류 원장을 수집하고 RCNO별로 비파괴
정제하는 Go CLI입니다.
날짜마다 Task 하나를 생성하고 위스키·브랜디·일반증류주·리큐르와 모든 추가
페이지를 순차 조회합니다.

## 데이터 구조

```text
mfds_jobs → mfds_tasks → mfds_fetches → mfds_items → mfds_declarations
```

- `mfds_jobs`: 요청한 수집 기간과 집계 상태
- `mfds_tasks`: 날짜별 재시도 단위
- `mfds_fetches`: HTTP 요청 정보와 압축 원문 응답
- `mfds_items`: RCNO로 대조하는 append-only 관찰 원장
- `mfds_declarations`: RCNO당 1행인 최신 원본 참조와 정제 결과
- `mfds_declaration_details`: 원본 관찰과 정제 결과를 함께 읽는 View

다중 페이지 결과는 다시 수집한 RCNO 집합이 일치할 때 확정합니다. 반복 관찰은
이후 정규화를 위해 원장에 유지합니다.

## 요구 환경

- Go 1.26 이상
- Task 3.17 이상
- Docker 28 이상과 Compose v2
- SOPS 3.11 이상과 등록된 age 키
- 비공개 서브모듈 2개에 대한 접근 권한

## 실행

```bash
task setup
task compose:up
task migrate
task health

task run -- collect \
  --from YYYY-MM-DD \
  --to YYYY-MM-DD \
  --workers 2

task run -- collect-recent

task run -- normalize
task run -- normalize --limit 100
task run -- normalize --rcno RCNO
task run -- normalize --dry-run
task run -- normalize --force --limit 20000

task run -- match --all --dry-run
task run -- match --all
```

`normalize`는 기본 100건을 처리합니다. `--rcno`는 상태와 관계없이 한 건을
재정제하고, `--dry-run`은 원장·정제 행·lease·시각을 변경하지 않습니다.
`--force`는 기존 terminal 정제 행을 한 번만 `STALE`로 되돌린 뒤 지정한
`--limit` 범위로 재정제합니다. 원본·기존 정제값·공식 수입사 연결과 수동
매칭 선택은 새 결과가 저장되기 전까지 보존됩니다. `--force`는 `--rcno`,
`--dry-run`과 함께 사용할 수 없습니다.
정제 상태는 `PENDING`, `STALE`, `NORMALIZED`, `PARTIAL`, `REVIEW_REQUIRED`,
`UNPARSED`로 구분합니다. 원문 `mfds_items`는 수정하거나 삭제하지 않습니다.
`collect-recent`는 인자 없이 KST 오늘을 포함한 최근 7일을 append-only로
수집합니다. 반복 실행 시 겹치는 관측치를 의도적으로 보존하며 정제는 실행하지
않습니다.
시스템 오류로 최대 재시도 횟수를 소진한 RCNO는 원인을 해결한 뒤
`normalize --rcno RCNO`로 강제 재정제합니다.

MFDS는 같은 BottleNote 데이터베이스의 `alcohols`, `distilleries`, `regions`
원본 테이블을 직접 조회합니다. 1차 정제는 증류소·리전 후보를 함께 저장하고,
`match`는 이미 정제된 행을 백필합니다. 두 경로 모두 관리자가 선택한 ID는
변경하지 않습니다.

## 설정

비밀이 아닌 고정 실행값은 `data/config.yaml`에서 관리합니다. MFDS 웹 주소,
고정 주류 대상, QPS, 재시도 간격, 기본 worker 수, 정제 batch/lease 상수와 DB
pool 설정이 포함됩니다. 정제 batch 크기는 `normalize --limit`으로만 변경할 수
있습니다.

DB 환경 변수는 `git.environment-variables/application.go/local.sops.env`에서
암호화해 관리합니다.

```text
MYSQL_ROOT_PASSWORD  MYSQL_DATABASE  MYSQL_USER  MYSQL_PASSWORD  MYSQL_DSN
```

OS 환경 변수가 `task setup`이 생성하는 추적 제외 `.env.local`보다 우선합니다.
Flyway migration은 `git.environment-variables/storage/db/migration`에서
관리합니다. MFDS 전용 sqlc 입력 스키마·쿼리와 생성 코드는 `git.secrets`에서
관리합니다.

V12는 과거 수입사 seed를 도입한 이력이고, V13은 폐기된 미매칭 관리 큐를 제거한 뒤
`mfds_importer_rcno_links`로 공식 확인 근거를 분리합니다. 이미 적용된 V12의 checksum은
바꾸지 않습니다.

웹 원장 수집이 완료되면 이번 job에서 아직 RCNO 연결 근거가 없는 상호만 공식
수입식품정보마루 화면으로 순차 조회합니다. 국내업소 exact 후보가 한 건이면 해당
업소를 저장하고 `PAGE_NAME`으로 연결합니다. 후보가 복수이면 같은 처리일의 공식
제품 갤러리 상세에서 RCNO와 국내업소 내부 코드를 함께 확인한 건만 `PAGE_RCNO`로
연결합니다. 결과가 없거나 RCNO 근거가 일치하지 않으면 별도 상태나 큐를 만들지 않고
`mfds_declarations.importer_id`를 NULL로 유지합니다. 원장 원문과 정제 문자열은 그대로
보존합니다.

## 구조

```text
cmd/                         Cobra 명령
internal/app/                애플리케이션 조립
internal/config/             YAML과 DB 환경 변수 로딩
internal/source/mfdsweb/     HTTP client와 HTML parser
internal/source/mfdscompany/ 수입식품정보마루 국내업소 HTML client/parser
internal/usecase/weblist/    수집과 RCNO 대조
internal/usecase/importerresolution/ 공식 페이지 기반 RCNO 수입사 해소
internal/normalization/      순수 정제 규칙과 파서
internal/matching/           불변 alcohol·증류소·리전 matcher
internal/usecase/normalization/ 정제 batch와 상태 전이
internal/usecase/matching/   매칭 dry-run과 백필 조정
internal/store/mysql/        원장과 정제 결과 저장
data/config.yaml             비밀이 아닌 고정 실행값
```

## 검증

```bash
task check
task test:race
task sqlc:check
task compose:config
```

## 정기 배포

Kubernetes manifest는 `git.environment-variables` submodule에 있습니다. 수집과
정제는 서로 다른 일일 `CronJob`으로 등록해 한쪽 실패가 다른 쪽의 시작이나 결과에
영향을 주지 않게 했습니다. 검토용 기본 일정은 development 01:00/04:00 KST,
production 03:00/06:00 KST입니다. 두 실행 모두 `concurrencyPolicy: Forbid`를
사용하고, 수집은 `collect-recent`, 정제는 제한된 `normalize --limit 10000` batch만
실행합니다. 정제 창은 수집의 시작 허용 시간 30분과 최대 실행 시간 2시간이 지난
3시간 뒤에 시작합니다. 수집이 실패해도 기존 `PENDING` 또는 `STALE` 대상은 별도로
정제합니다.

development는 서명된 `v0.1.1` release image로 두 일정을 활성화했습니다.
`mfds-secrets` Secret은 `MYSQL_DSN`만 포함하고 환경 저장소에서 SOPS로 암호화하며,
BottleNote development DB를 가리킵니다. 활성화 전에 Flyway V13과 release image의
DB health를 확인했습니다.

production도 서명된 `v0.1.1` 이미지로 수집 03:00, 정규화 06:00 KST 일정을
활성화했습니다. 운영 DB의 V11~V13을 확인하고 개발 원본 RCNO 13,111건을
복제한 뒤 운영 기준으로 정규화·매칭을 다시 계산했습니다. 개발 알코올·증류소·리전
ID는 이관하지 않았습니다. 상세 검증은 [운영 이관 기록](docs/2026.09.08%20MFDS%20운영%20이관%20기록.md)에 있습니다.

운영 정규화는 `mfds-runtime-config` ConfigMap을 읽기 전용으로 마운트합니다.
한 번에 최대 10,000건을 처리하는 동안 lease가 만료되지 않도록 `lease_duration: 2h`를
사용하며 Job 제한도 2시간입니다. DB Secret은 SOPS 암호화로 관리합니다.

HTTP 요청 제한 시간은 목록·수입사 조회 모두 60초입니다. 목록은 기존 날짜 단위
재시도와 RCNO 일관성 검증을 유지합니다. 수입사 조회는 일시적 네트워크 오류와
HTTP 429/500/502/503/504에 대해 업체 그룹 단위로 최대 3회 시도하며, 기본 대기는
2초와 5초입니다. 재시도를 소진한 업체는 연결을 저장하지 않고 다음 업체를 처리합니다.
미연결 대상은 기존 pending 조회를 통해 다음 수집 실행에서 다시 조회됩니다.
DB 오류, 실행 취소, 데이터 검증 오류는 계속 명령 실패로 처리합니다.

stderr의 JSON 로그는 `collection_fetch_failed`, `collection_finished`,
`importer_group_attempt_failed`, `importer_group_deferred`, `importer_sync_finished`를
기록합니다. `job_id`, 대상 날짜·품목·페이지 또는 업체명, `attempt`, 오류 `code`를
사용해 원장을 찾을 수 있습니다. `HTTP_TIMEOUT`은 요청 시간 초과,
`IMPORTER_RETRY_EXHAUSTED`는 업체 조회 재시도 소진을 뜻합니다.
수집이 성공해도 `failed_groups > 0`이면 수입사 보강은 미완료입니다.
`resolved`, `unresolved`(후보 없음), `failed_rcnos`(통신 오류)는 별도로 집계합니다.
실패 이력은 Pod 로그에 남고 별도 실패 테이블은 만들지 않습니다.
