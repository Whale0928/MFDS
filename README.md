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

task run -- collect \
  --from YYYY-MM-DD \
  --to YYYY-MM-DD \
  --workers 2

task run -- normalize
task run -- normalize --limit 100
task run -- normalize --rcno RCNO
task run -- normalize --dry-run

task run -- sync-company-registry --since YYYY-MM-DD
```

`normalize` processes 100 rows by default. `--rcno` force-normalizes one row
regardless of its current state, while `--dry-run` changes no ledger row,
declaration, lease, or timestamp. States are `PENDING`, `STALE`, `NORMALIZED`,
`PARTIAL`, `REVIEW_REQUIRED`, and `UNPARSED`. Source `items` are never updated
or deleted.
After fixing a system error, recover an RCNO that exhausted its retry limit with
`normalize --rcno RCNO`.

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

The FoodSafetyKorea company registry uses a separate credential:

```text
FOODSAFETYKOREA_API_KEY
```

`sync-company-registry` sequentially collects importer business licenses (C001),
importer closures (I2821), excellent importers (I0250), and administrative
dispositions (I0470). The first run requires `--since`; later runs reuse the
last completed date as an inclusive `CHNG_DT` boundary. Excellent importers
(I0250) has no change-date filter and is fetched in full each time. Request and
row originals are appended to separate raw ledgers without changing `items` or
`declarations`.

No persistent importer-link table is created. When an import detail is opened,
the dashboard queries the current database by exact importer/business name and
returns every license with that name. Official limits remain 1,000 rows per
request and 500 requests per synchronization run.

## Structure

```text
cmd/                         Cobra commands
internal/app/                application wiring
internal/config/             YAML and database environment loading
internal/source/mfdsweb/     HTTP client and HTML parser
internal/source/foodsafetykorea/ FoodSafetyKorea JSON client
internal/usecase/weblist/    collection and RCNO reconciliation
internal/usecase/companyregistry/ incremental company registry collection
internal/normalization/      pure normalization rules and parsers
internal/usecase/normalization/ normalization batch and state transitions
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
