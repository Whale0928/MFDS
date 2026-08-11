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

task run -- normalize
task run -- normalize --limit 100
task run -- normalize --rcno RCNO
task run -- normalize --dry-run

task run -- match --all --dry-run
task run -- match --all
```

`normalize`는 기본 100건을 처리합니다. `--rcno`는 상태와 관계없이 한 건을
재정제하고, `--dry-run`은 원장·정제 행·lease·시각을 변경하지 않습니다.
정제 상태는 `PENDING`, `STALE`, `NORMALIZED`, `PARTIAL`, `REVIEW_REQUIRED`,
`UNPARSED`로 구분합니다. 원문 `mfds_items`는 수정하거나 삭제하지 않습니다.
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

## 구조

```text
cmd/                         Cobra 명령
internal/app/                애플리케이션 조립
internal/config/             YAML과 DB 환경 변수 로딩
internal/source/mfdsweb/     HTTP client와 HTML parser
internal/usecase/weblist/    수집과 RCNO 대조
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
