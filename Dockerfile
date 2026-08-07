# syntax=docker/dockerfile:1

# BUILDPLATFORM에서 한 번만 돌고 TARGETARCH로 교차 컴파일한다.
# cgo를 쓰지 않으므로 QEMU 없이 amd64와 arm64를 같은 러너에서 만든다.
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETARCH

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags="-s -w" -o /out/mfds-crawler .

FROM gcr.io/distroless/static:nonroot

# 설정 경로가 cwd 기준 data/config.yaml 상수라 작업 디렉터리를 고정해야 한다.
WORKDIR /app

COPY --from=build /out/mfds-crawler /app/mfds-crawler
COPY data/config.yaml /app/data/config.yaml

USER nonroot:nonroot

# 서브커맨드는 실행 시 넘긴다. 예: docker run IMAGE normalize --limit 100
ENTRYPOINT ["/app/mfds-crawler"]
