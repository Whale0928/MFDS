# MFDS Import Ledger Collector

[한국어](README.ko.md)

Go CLI that collects the public MFDS imported-liquor ledger and derives
non-destructive normalization results per RCNO. A collection task represents
one date and sequentially mfds_fetches whisky, brandy, general distilled spirits,
and liqueur, including every additional result page.

## Data model

```text
mfds_jobs → mfds_tasks → mfds_fetches → mfds_items → mfds_declarations
```

- `mfds_jobs`: requested collection range and aggregate status
- `mfds_tasks`: one retryable unit per date
- `mfds_fetches`: HTTP request metadata and compressed raw response
- `mfds_items`: append-only observations reconciled by RCNO
- `mfds_declarations`: one latest source reference and normalization result per RCNO
- `mfds_declaration_details`: read view joining source observations and derived values

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

task run -- collect-recent

task run -- normalize
task run -- normalize --limit 100
task run -- normalize --rcno RCNO
task run -- normalize --dry-run
task run -- normalize --force --limit 20000

task run -- match --all --dry-run
task run -- match --all
```

`normalize` processes 100 rows by default. `--rcno` force-normalizes one row
regardless of its current state, while `--dry-run` changes no ledger row,
declaration, lease, or timestamp. States are `PENDING`, `STALE`, `NORMALIZED`,
`PARTIAL`, `REVIEW_REQUIRED`, and `UNPARSED`. Source `mfds_items` are never updated
or deleted.
`--force` marks existing terminal declarations `STALE` once and re-normalizes up
to `--limit` rows. Source data, prior derived values, official importer links,
and manual matching choices remain until replacement results are stored. It cannot
be combined with `--rcno` or `--dry-run`.
`collect-recent` takes no arguments and append-only collects the seven calendar
days ending today in KST. Repeated runs intentionally preserve overlapping
observations. It does not run normalization.
After fixing a system error, recover an RCNO that exhausted its retry limit with
`normalize --rcno RCNO`.

MFDS reads the canonical `alcohols`, `distilleries`, and `regions` tables from
the same BottleNote database. Primary normalization writes ranked distillery
and region candidates, while `match` backfills already-normalized mfds_declarations.
Both paths preserve administrator-selected IDs.

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
by `task setup`. Flyway migrations live in
`git.environment-variables/storage/db/migration`. MFDS-only sqlc schema, queries,
and generated code live in `git.secrets`.

V12 is the historical migration that introduced the importer seed. V13 removes
the retired unmatched-importer queue and records official evidence in
`mfds_importer_rcno_links`. The checksum of an already-applied V12 is not changed.

After a web-ledger job completes, the application sequentially resolves only the
trade names whose RCNO has no evidence yet. One exact domestic-business candidate
is stored and linked as `PAGE_NAME`. For multiple candidates, only a product
gallery detail that exposes both the same RCNO and a matching domestic-business
code is linked as `PAGE_RCNO`. No result or no matching RCNO evidence leaves
`mfds_declarations.importer_id` NULL without creating a separate status or queue.
The immutable source ledger and normalized importer text remain intact.

## Structure

```text
cmd/                         Cobra commands
internal/app/                application wiring
internal/config/             YAML and database environment loading
internal/source/mfdsweb/     HTTP client and HTML parser
internal/source/mfdscompany/ official domestic-business HTML client/parser
internal/usecase/weblist/    collection and RCNO reconciliation
internal/usecase/importerresolution/ official-page RCNO importer resolution
internal/normalization/      pure normalization rules and parsers
internal/matching/           immutable alcohol, distillery, and region matcher
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
