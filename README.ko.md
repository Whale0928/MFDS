# MFDS 수입주류 원장 수집기

[English](README.md)

식약처 수입식품정보마루의 공개 수입주류 원장을 수집하는 Go CLI입니다.
날짜마다 Task 하나를 생성하고 위스키·브랜디·일반증류주·리큐르와 모든 추가
페이지를 순차 조회합니다.

## 데이터 구조

```text
jobs → tasks → fetches → items
```

- `jobs`: 요청한 수집 기간과 집계 상태
- `tasks`: 날짜별 재시도 단위
- `fetches`: HTTP 요청 정보와 압축 원문 응답
- `items`: RCNO로 대조하는 append-only 관찰 원장

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
```

CLI는 `collect`, `health`, `migrate`만 제공합니다.

## 설정

비밀이 아닌 고정 실행값은 `data/config.yaml`에서 관리합니다. MFDS 웹 주소,
고정 주류 대상, QPS, 재시도 간격, 기본 worker 수와 DB pool 설정이 포함되며
CLI flag나 환경 변수로 덮어쓰지 않습니다.

DB 환경 변수는 `git.environment-variables/application.go/local.sops.env`에서
암호화해 관리합니다.

```text
MYSQL_ROOT_PASSWORD  MYSQL_DATABASE  MYSQL_USER  MYSQL_PASSWORD  MYSQL_DSN
```

OS 환경 변수가 `task setup`이 생성하는 추적 제외 `.env.local`보다 우선합니다.
Migration과 생성된 sqlc 코드는 `git.secrets`에서 관리합니다.

## 구조

```text
cmd/                         Cobra 명령
internal/app/                애플리케이션 조립
internal/config/             YAML과 DB 환경 변수 로딩
internal/source/mfdsweb/     HTTP client와 HTML parser
internal/usecase/weblist/    수집과 RCNO 대조
internal/store/mysql/        jobs, tasks, fetches, items 저장
data/config.yaml             비밀이 아닌 고정 실행값
```

## 검증

```bash
task check
task test:race
task sqlc:check
task compose:config
```
