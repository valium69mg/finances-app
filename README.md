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

## Layout

```
backend/     Go API (hexagonal, one module per bounded context)
migrations/  SQL migrations (golang-migrate)
```
