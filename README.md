# finances-app

Personal finance web app for a Mexican RESICO freelancer. See [PLAN.md](PLAN.md) for goals,
architecture and phases.

## Requirements

- Go
- Docker with Compose
- [`migrate`](https://github.com/golang-migrate/migrate) CLI

## Run locally

Create a gitignored `.env` at the repo root with these variables (use your own values):

```
POSTGRES_USER=finances
POSTGRES_PASSWORD=<password>
POSTGRES_DB=finances
MINIO_ROOT_USER=<user>
MINIO_ROOT_PASSWORD=<password, min 8 chars>
DATABASE_URL=postgres://<user>:<password>@localhost:5442/<db>?sslmode=disable
HTTP_ADDR=:8080
RESEND_API_KEY=                # optional for now; used by the email alerts (phase 7)
```

Then:

```sh
make db-up                  # PostgreSQL (localhost:5442) and MinIO (9100 API, 9101 console)
make migrate-up             # apply migrations
make run                    # start the API
curl -i localhost:8080/healthz
```

The Makefile reads `.env`, so `DATABASE_URL` and the Postgres credentials must stay in sync.

### One-time imports

The legacy CLI data in `~/finances` is loaded with idempotent commands that refuse to run on
non-empty data unless `FORCE=1` (which appends, never deletes):

```sh
make import-config          # settings from ~/finances/config.json
make import-movements       # movements from ~/finances/movimientos.csv
make import-bills           # the five Servicios bills from the note of the Servicios category
```

`make import-bills` creates monthly bills for megacable, agua, luz, telcel and gas LP, due on the
1st of next month as a placeholder: adjust the real due days in the app.

### Invoice document storage

The API stores issued CFDI files (XML, PDF) in S3-compatible storage (MinIO locally) and
creates the private bucket on startup, so MinIO must be up (`make db-up`). Optional `.env` keys,
all with local defaults:

```
S3_ENDPOINT=localhost:9100        # host:port, no scheme
S3_BUCKET=finances-invoices
S3_REGION=us-east-1
S3_USE_SSL=false
S3_ACCESS_KEY=                    # defaults to MINIO_ROOT_USER
S3_SECRET_KEY=                    # defaults to MINIO_ROOT_PASSWORD
```

The S3 adapter integration test is skipped unless `TEST_S3_ENDPOINT`, `TEST_S3_ACCESS_KEY` and
`TEST_S3_SECRET_KEY` are set (for example to the MinIO root credentials).

## Layout

```
backend/     Go API (hexagonal, one module per bounded context)
migrations/  SQL migrations (golang-migrate)
```
