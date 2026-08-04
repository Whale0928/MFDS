# MFDS Import Ledger Collector

[한국어](README.ko.md)

Go CLI that collects the public MFDS imported-liquor ledger. A task represents
one date and sequentially fetches whisky, brandy, general distilled spirits,
and liqueur, including every additional result page.

## Data model

```text
jobs → tasks → fetches → items
```

- `jobs`: requested collection range and aggregate status
- `tasks`: one retryable unit per date
- `fetches`: HTTP request metadata and compressed raw response
- `items`: append-only observations reconciled by RCNO

Multi-page results are collected again and accepted only after their RCNO sets
match. Repeated observations remain in the ledger for later normalization.

## Requirements

- Go 1.26+
- Task 3.17+
- Docker 28+ with Compose v2
- SOPS 3.11+ and an authorized age key
- Access to both private submodules

## Run

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

The CLI exposes only `collect`, `health`, and `migrate`.

## Configuration

Non-secret runtime constants are tracked in `data/config.yaml`. They include the
MFDS web endpoint, fixed liquor targets, QPS, retry delays, worker defaults, and
database pool settings. CLI flags and environment variables do not override
these values.

Database environment variables are encrypted at
`git.environment-variables/application.go/local.sops.env`:

```text
MYSQL_ROOT_PASSWORD  MYSQL_DATABASE  MYSQL_USER  MYSQL_PASSWORD  MYSQL_DSN
```

OS environment values take precedence over the ignored `.env.local` generated
by `task setup`. Migrations and generated sqlc code live in `git.secrets`.

## Structure

```text
cmd/                         Cobra commands
internal/app/                application wiring
internal/config/             YAML and database environment loading
internal/source/mfdsweb/     HTTP client and HTML parser
internal/usecase/weblist/    collection and RCNO reconciliation
internal/store/mysql/        jobs, tasks, fetches, and items persistence
data/config.yaml             non-secret runtime constants
```

## Verify

```bash
task check
task test:race
task sqlc:check
task compose:config
```
