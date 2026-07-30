# MFDS 수입주류 원장 수집기

[English](README.md)

식약처 수입식품정보마루의 공개 수입 원장을 수집하는 Go 애플리케이션입니다.
설정된 주류 4종을 하나의 고정 그룹으로 조회하고 RCNO 기준으로 결과를 검증합니다.

## 주요 기능

- `jobs → tasks → fetches → items` 원장 구조
- 날짜별 Task 하나에서 설정된 주류 전체 조회
- HTTP 원문 증거와 관찰 이력 보존
- RCNO 수량·집합 검증
- 다중 페이지 결과 반복 검증
- MySQL, sqlc, Cobra CLI, Bubble Tea TUI

## 요구 환경

- Go 1.26 이상
- Task 3.17 이상
- Docker 28 이상과 Compose v2
- 비공개 실행 자산 서브모듈 접근 권한

## 실행

```bash
task setup
task compose:up
task migrate
task run
```

기간 수집:

```bash
task run -- web list-job \
  --from YYYY-MM-DD \
  --to YYYY-MM-DD \
  --workers 2
```

설정 우선순위:

```text
CLI flag > OS 환경 변수 > 비공개 .env > 비공개 YAML > 기본값
```

## 검증

```bash
task test
task test:race
task vet
task build
task sqlc:check
```

실행 설정, migration, 생성된 DB 코드, 스냅샷과 운영 보고서는 비공개
서브모듈에서 관리합니다. 이 저장소에는 실제 자격증명이나 수집 원장을
커밋하지 않습니다.
