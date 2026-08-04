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
- SOPS 3.11+ and an authorized age key
- Access to both private submodules

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
Database connection values: OS environment > decrypted `.env.local`

Fixed runtime values: `data/config.yaml`
```

## Verify

```bash
task test
task test:race
task vet
task build
task sqlc:check
```

Database connection environment variables are encrypted at
`git.environment-variables/application.go/local.sops.env`. `task setup` decrypts
them into the ignored `.env.local` file. Migrations, generated database code,
snapshots, and operational reports remain in the `git.secrets` submodule.
Non-secret runtime constants are tracked in `data/config.yaml` and are not overridden by CLI flags or environment variables.
