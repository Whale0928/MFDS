# MFDS 정제 결과 대시보드

고객 시연을 위해 로컬에서만 실행하는 읽기 전용 화면입니다. 루트 수집기와 별도
Go module 및 React project로 구성되며, 루트 `cmd`, `internal/app`, `go.mod`에서
이 폴더를 참조하지 않습니다.

## 실행

루트에서 SOPS 환경 파일과 MySQL을 준비합니다.

```bash
task setup
task compose:up

cd tools/normalization-dashboard
task api
```

다른 terminal에서 web을 실행한 뒤 `http://127.0.0.1:5173`을 엽니다.

```bash
cd tools/normalization-dashboard
npm --prefix web ci
task web
```

API는 `127.0.0.1:8787`에만 bind하며 `MFDS_DEMO_DSN`을 사용합니다. 이 DSN의
MySQL 계정에는 `mfds_ledger.*`의 `SELECT`만 부여합니다. 화면은 원본 HTML,
전체 hash, 요청·fetch metadata, 내부 경로 또는 DSN을 반환하지 않습니다.

## 검증

```bash
task test
task build
```

시연 종료 후 이 폴더를 삭제하거나 대시보드 추가 커밋만 revert하면 루트 수집기와
정제 CLI를 유지한 채 화면을 제거할 수 있습니다.
