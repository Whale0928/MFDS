# MFDS Import Ledger Collector

[한국어](README.ko.md)

Go CLI that collects the public MFDS imported-liquor ledger and derives
non-destructive normalization results per RCNO. A collection task represents
one date and sequentially fetches whisky, brandy, general distilled spirits,
and liqueur, including every additional result page.

## Data model

```text
jobs → tasks → fetches → items → declarations
```

- `jobs`: requested collection range and aggregate status
- `tasks`: one retryable unit per date
- `fetches`: HTTP request metadata and compressed raw response
- `items`: append-only observations reconciled by RCNO
- `declarations`: one latest source reference and normalization result per RCNO
- `declaration_details`: read view joining source observations and derived values

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

task run -- reference-sync

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

`normalize` processes 100 rows by default. `--rcno` force-normalizes one row
regardless of its current state, while `--dry-run` changes no ledger row,
declaration, lease, or timestamp. States are `PENDING`, `STALE`, `NORMALIZED`,
`PARTIAL`, `REVIEW_REQUIRED`, and `UNPARSED`. Source `items` are never updated
or deleted.
After fixing a system error, recover an RCNO that exhausted its retry limit with
`normalize --rcno RCNO`.

`reference-sync` atomically replaces the local `alcohols`, `distilleries`, and
`regions` mirrors and verifies every source and target column with deterministic
hashes. Run it before normalization. Primary normalization writes ranked
distillery and region candidates, while `match` backfills already-normalized
declarations. Both paths preserve administrator-selected IDs.

## Configuration

Non-secret runtime constants are tracked in `data/config.yaml`. They include the
MFDS web endpoint, fixed liquor targets, QPS, retry delays, worker defaults,
normalization batch/lease constants, and database pool settings. Only the
normalization batch size can be overridden with `normalize --limit`.

Database environment variables are encrypted at
`git.environment-variables/application.go/local.sops.env`:

```text
MYSQL_ROOT_PASSWORD  MYSQL_DATABASE  MYSQL_USER  MYSQL_PASSWORD  MYSQL_DSN
```

OS environment values take precedence over the ignored `.env.local` generated
by `task setup`. Migrations and generated sqlc code live in `git.secrets`.
`reference-sync` additionally requires `BOTTLENOTE_REFERENCE_DSN` as a process
environment variable. Inject it through a secret manager or a non-echoing shell
prompt; do not place the DSN directly in shell history or tracked files.

## Structure

```text
cmd/                         Cobra commands
internal/app/                application wiring
internal/config/             YAML and database environment loading
internal/source/mfdsweb/     HTTP client and HTML parser
internal/usecase/weblist/    collection and RCNO reconciliation
internal/normalization/      pure normalization rules and parsers
internal/matching/           immutable alcohol, distillery, and region matcher
internal/reference/          transactional BottleNote reference synchronization
internal/usecase/normalization/ normalization batch and state transitions
internal/usecase/matching/   matching dry-run and backfill orchestration
internal/store/mysql/        ledger and normalization persistence
data/config.yaml             non-secret runtime constants
```

## Verify

```bash
task check
task test:race
task sqlc:check
task compose:config
```
