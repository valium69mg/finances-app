# finances-app

Personal finances app (single owner today). Go backend (hexagonal), React + TypeScript + Vite + Tailwind frontend, PostgreSQL, MinIO (S3 API) for CFDI files. `PLAN.md` is the source of truth for decisions, modules and phases: read the relevant row before changing a module, and record new decisions there.

## Layout

- `backend/` Go: `cmd/` (api, seed, import-config, import-movements, import-bills, healthcheck), `internal/<module>/{domain,app,adapters}` per module, `internal/platform/*` shared pieces (config, cors, clientip, httpmw, httpjson, postgres, health).
- `frontend/` React app; e2e in `frontend/e2e` (Playwright, mocks in `e2e/helpers.ts`).
- `migrations/` golang-migrate SQL (up + down). `deploy/` production compose stack, nginx, runbook. `infra/` Terraform (AWS + Cloudflare).

## Conventions

- Code, comments, tests, commits, docs: English. UI copy: Spanish. Official terms (CFDI, RFC, SAT, RESICO, IVA, ISR) stay as-is.
- Conventional commits, no AI attribution lines. Split work into focused commits (domain, repo, http, frontend, docs).
- Money is a decimal string end to end. User-supplied decimals go through `ledger.CheckAmount` before any compare or round.
- New module: mirror an existing one (`income`, `savings`, `bills`): domain validation, app service with ports, postgres and http adapters, tests at each layer, route wiring in `cmd/api/main.go`, frontend page on the existing tab in `modules.ts`, shared helpers (`movementErrors`, `Modal`, money formatting), unit and Playwright coverage, PLAN.md row.
- `movements` is one shared table (Ingreso, Gasto, Ahorro); each module reads and writes only its own kind.

## Commands

- `make db-up` (Postgres 5442 + MinIO 9100), `make run`, `make test`, `make fe-dev`. Load `.env` first (`set -a; . ./.env; set +a`).
- `make test` skips the DB tests; `make test-integration` runs everything against the dev Postgres via `TEST_DATABASE_URL`. A green `make test` does not prove the repositories, so run the integration target before reporting.
- Frontend: `npx tsc --noEmit`, `npx vitest run`, `npx playwright test`. There is no lint script.
- Migrations: check `migrate ... version` first and apply with `migrate ... up N`. Do not run a blind `make migrate-up` while another session may be adding migrations.

## Gotchas

- Restart the dev API by PID (`ss -ltnp | rg :8080`, kill it and its parents). Never `pkill -f`: the pattern matches your own shell.
- After adding a module, the running dev API still serves the old routes (404s): restart it.
- Playwright reuses whatever runs on :5173; stale vite gives "X is not defined". Touch `frontend/src/App.tsx` or restart vite.
- `bat`, `fd`, `eza`, `sd` and `brew` are not installed here: use `rg`, the Read/Edit/Write tools and plain `git`. Writes to `.env*` paths can be denied, which is why the prod template is `deploy/prod.env.example`.
- Another session may be working in the same working copy: run `git log` and `git status` before assuming state.
- After restoring a database dump, never run `/seed` (the dump already holds the admin user).

## Public repository rules

The GitHub repo is PUBLIC. Never commit secrets, tokens, private keys, dumps, `.env*`, Terraform state or variables, the real hostname, IPs or the AWS account id. Ignored on purpose: `infra/.keys/`, `infra/terraform.tfvars`, `infra/backend.hcl`, `*.tfstate*`, `deploy/.env.prod`. Keep the examples (`*.example`) placeholder-only. Never paste a secret in chat; move it by file or pipe without printing it.

## Production

- Runs on one AWS VM (Docker Compose, `deploy/docker-compose.prod.yml`) behind the Cloudflare orange-cloud proxy, created with Terraform in `infra/` (see `infra/README.md` and `deploy/README.md`). Production is the source of truth for data: do not enter real data in the dev database.
- Update: push to GitHub, then on the VM `git pull` and `docker compose --env-file deploy/.env.prod -f deploy/docker-compose.prod.yml up -d --build --wait`. The SSH port is closed by default: reopen it with `enable_ssh = true` in `infra/terraform.tfvars` and an apply (the admin IP is auto-detected), or use SSM Session Manager once it exists.
- Terraform: the Cloudflare token is read per command from a 600-permission file outside the repo and never printed. Always `plan` first, read the summary, and apply only with explicit user approval (resources are billable). Plan files contain secrets: write them with `umask 077` outside the repo and delete them after the apply. A transient `checkip.amazonaws.com` timeout during plan means re-run the plan. The Cloudflare token needs Zone > SSL and Certificates > Edit to create the origin certificate.
- Backups are deliberately out of scope until the MinIO to S3 migration; daily EBS snapshots of the data volume are the stopgap.
- Manual upkeep: refresh Cloudflare IP ranges every few months (`deploy/scripts/refresh-cloudflare-ips.sh`), confirm domain auto-renew, reboot for new kernels, bump pinned Docker images by hand.

## Workflow

- Delegate multi-file work to one writer subagent with a precise brief; verify its claims against the code and run the real tests before reporting.
- Ask before pushing and before any billable or outward-facing action. Report failures and skipped checks plainly.
