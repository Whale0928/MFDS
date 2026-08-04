# MFDS Import Ledger Collector

[한국어](README.ko.md)

Go collector for the public MFDS imported-food registry. It collects four liquor
categories as one fixed group and preserves request evidence with RCNO-based
reconciliation.

## Features

- `jobs → tasks → fetches → items` ledger
- One date per task across all configured liquor categories
- Raw HTTP evidence and append-only observations
- RCNO count and set validation
- Repeat verification for multi-page results
- MySQL, sqlc, and Cobra CLI

## Requirements

- Go 1.26+
- Task 3.17+
- Docker 28+ with Compose v2
- Access to the private runtime-assets submodule

## Run

```bash
task setup
task compose:up
task migrate
task run -- health
```

Start a bounded collection job:

```bash
task run -- collect \
  --from YYYY-MM-DD \
  --to YYYY-MM-DD \
  --workers 2
```

Configuration precedence:

```text
CLI flags > OS environment > private .env > private YAML > defaults
```

## Verify

```bash
task test
task test:race
task vet
task build
task sqlc:check
```

Runtime configuration, migrations, generated database code, snapshots, and
operational reports are kept in the private submodule. This repository must not
contain live credentials or collected ledger data.
