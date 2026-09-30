# finances-app — Project Plan

Personal finance web app for a Mexican RESICO freelancer. Built from scratch as a
successor to the `~/finances` Python CLI (`fin.py`).

**Language rule (DECIDED):** the product is in Spanish; the codebase is in English.
- Spanish: everything the user sees in the frontend (UI copy, labels, messages, emails) and catalog/data values (categories, types, states, instrument types, payment methods), e.g. `KindIncome Kind = "Ingreso"`.
- English: identifiers (variables, functions, types, packages, constants' names), code comments, tests, docs, commit messages, API field names and file names.

Status: **planning**. No code exists yet. Decisions marked **OPEN** must be answered by
the owner before the affected work starts. Nothing is defaulted silently.

## 1. Goals

- Single-user web app (v1), run locally.
- One module per former Claude skill, each with its own tab.
- Dashboard as the main screen.
- Later: deploy to AWS, infrastructure provisioned with Terraform. Not part of v1.
- Out of scope for v1: the owner's wife / multi-user, wishlist, allowance tracking.

## 2. Stack (proposed, owner-confirmed items only)

| Layer | Choice | Status |
|---|---|---|
| Backend | Go | confirmed |
| Database | PostgreSQL | confirmed |
| Frontend | installable web app (PWA) with React + TypeScript, built with Vite | confirmed; React Router, TanStack Query, Tailwind **DECIDED** |
| Email | Resend | confirmed; first used for login email verification (phase 3), later for bill reminders (phase 7) |
| Infra | AWS + Terraform | confirmed; deferred until the app is ready |
| Local runtime | Docker Compose for PostgreSQL | confirmed; host port 5442 mapped to container 5432 (5432 is taken by another local instance); PostgreSQL 18 (latest stable major; 19 is still beta). The compose file must define health checks (`pg_isready`) and dependent services wait on them |

**Design rule:** every UI design decision (layout, components, color, typography, states)
goes through the `ui-ux-pro-max` skill (plugin `ui-ux-pro-max@ui-ux-pro-max-skill`).

## 3. Architecture

Hexagonal, one module per bounded context:

```
finances-app/
  backend/
    cmd/api/                 # entrypoint
    internal/
      <module>/
        domain/              # pure logic, no I/O
        app/                 # use cases
        adapters/
          http/              # handlers
          postgres/          # repositories
  frontend/
  migrations/
  infra/                     # Terraform (later)
```

Rules:

- Domain code is pure functions plus entities. No database, no HTTP.
- Money is an exact decimal (the `BigDecimal` equivalent), never `float64`. **DECIDED:** `shopspring/decimal` in Go, `NUMERIC` in PostgreSQL, and decimal strings in the JSON API. Rounding rules per calculation (e.g. SAT rounding) are defined explicitly and tested.
- Every tax and split calculation has tests with hand-computed cases from real numbers.

## 4. Modules (one tab each)

| Tab | Purpose (from the former skill) |
|---|---|
| Dashboard (main) | Monthly summary: budget vs. actual per category, available money, emergency fund progress, tax and payment status. Replaces `/finanzas`. |
| Income | Register income (Sezzle in USD, client B in MXN) with the split rule for extra income. Replaces `/ingreso`. Storage: the shared `movements` table, only `kind = 'Ingreso'` (`backend/internal/income`). Amount rule **DECIDED:** an income amount must be greater than zero (domain plus CHECK, migration 000007); expenses too, only Ahorro may be negative. Defaults as in `fin.py`: payment method Transferencia, currency MXN, date today, USD rate from `fx_rate_applied`; an empty category is inferred from the description (Ingreso categories only). Split rule **DECIDED:** preview-only, matching `fin.py`: for the category "Contrato extra" the response carries a non-persisted `split` (SAT reserve, emergency fund, investments with the per-instrument breakdown, aguinaldo and vacation) computed on `amount_mxn`; the user records the Ahorro rows themselves. Auto-creating Ahorro rows was **rejected**. Sueldo gets no split. Creating or updating an income also returns the month's income total and the RESICO ISR estimate (rate, estimated ISR, whether the rate went up and the previous rate; null when the tax settings are incomplete). Endpoints: `POST /income`, `GET /income?month=&limit=`, `PUT /income/{id}`, `DELETE /income/{id}`, `GET /income/infer-category?description=`. |
| Expenses | Register expenses with category inference. Replaces `/gasto`. Storage **DECIDED:** a single shared `movements` table (migration 000006) holds Ingreso, Gasto and Ahorro; Expenses only reads and writes `kind = 'Gasto'`, and Income and Savings reuse the same table and repo (`backend/internal/ledger`). Amount rule **DECIDED:** an expense amount must be greater than zero (enforced in the domain and by a CHECK); only Ahorro may be negative (withdrawals). USD movements store the rate used, and `amount_mxn` is `amount * rate` rounded to cents. Defaults as in `fin.py`: payment method Débito, currency MXN, date today, USD rate from `fx_rate_applied`. Creating or updating an expense returns budget feedback for its month (spent vs. budget, over-budget flag, null budget when the category has none). Endpoints: `POST /expenses`, `GET /expenses?month=&limit=`, `PUT /expenses/{id}`, `DELETE /expenses/{id}`, `GET /expenses/infer-category?description=`. |
| Savings | All savings: emergency fund, aguinaldo and vacation, future expenses, investments and SAT reserve. Replaces `/ahorro` and `/portafolio` (the Portfolio tab is dropped). Instrument tracking **DECIDED:** each savings entry picks an instrument (e.g. GBM liquidity, CETES 28/91, VOO, VXUS), and instrument valuations can be recorded to show current value and gains. Valuations are entered manually; no external price API. Details **OPEN**. |
| Invoices | Create invoices, update their status, upload issued CFDI documents. Replaces `/factura`. |
| Tax Filing | Compute the monthly RESICO tax. Optionally record it as an expense when paid, or mark it pending. Replaces `/declaracion`. |
| Filed Records | History of filed declarations and payments. Replaces `/declarado`. |
| Month Close | Monthly snapshot: over-budget categories, suggestion for where to put the leftover money (emergency fund first, then investments), budget-adjustment hints for deviations over 20%. Stored in the database. Replaces `/cierre-mes`. |
| Bills & Subscriptions | Recurring bills and subscriptions (e.g. monthly water bill) with periodic reminders. A bill occurrence can be marked paid, which registers an expense. Later: a scheduled job that emails about bills that are about to expire or were never registered. Recurrence **DECIDED:** weekly, biweekly, monthly, bimonthly, yearly (no custom intervals). Amount **DECIDED:** a bill has either a fixed amount or no amount. When marking it paid, the expense amount defaults to the bill's amount when it has one, and can always be overridden (discounts, price changes). Overdue occurrences **DECIDED:** stay pending and flagged overdue until paid or manually skipped (skip registers no expense; no auto-skip). Reminder lead time **DECIDED:** configurable per bill. Category **DECIDED:** each bill has a default expense category, changeable when marking it paid. Occurrences **DECIDED:** only the next occurrence exists at a time; paying or skipping it generates the following one. Module design is otherwise settled. |
| Settings | Categories, budgets, clients, instruments, tax parameters (today's `config.json`). Also carries the investment pause plan (`inversiones_pausa` **DECIDED:** carried forward as configuration: paused months, normal budget, resume month, monthly future-expenses plan). Pause rule **DECIDED:** for paused months the Inversiones budget is 0 and Gastos futuros is the plan amount of that month; from the resume month Inversiones returns to its normal budget; the extra-income split is NOT changed. Implemented as per-month budget overrides through `CategoryBudget(cat, overrides)` (`GET /settings/budgets?month=YYYY-MM` returns the resolved budgets). |

Note: there is no separate Portfolio module. **DECIDED:** Savings covers every savings
category (emergency fund, aguinaldo and vacation, future expenses, investments, SAT reserve).

## 5. Auth (v1)

- Login page. Single admin user, email `carlostranquilino.cr@gmail.com`.
- Seeding **DECIDED:** the admin is seeded into PostgreSQL with only that email and a random password (stored as a bcrypt hash). No password or hash comes from `.env`, and no plaintext credential is ever in source or git history. The seed marks the user **unverified** (flag on the user row). A `.env.example` with placeholders is committed; the real `.env` is not.
- Email verification **DECIDED:** while the unverified flag is on, logging in leads to a forced flow: the app sends the user an email through Resend to verify the address and change the password. Completing it clears the flag. Resend API key comes from `RESEND_API_KEY` (in `.env`, placeholder in `.env.example`).
- First credential **DECIDED:** the admin never needs the random password. Verification is triggered from the identify step (below), not from the login form.
- Email-first login **DECIDED (supersedes the earlier single-form flow):** two steps.
  1. `POST /auth/identify {email}` returns `200 {"status":"password_required"}` when the account exists and is verified, and `200 {"status":"verification_sent"}` when it is unverified OR unknown. Unverified accounts get the verification email in the background; unknown emails send nothing but look identical to unverified. Invalid email format: `400 invalid_email`. Rate limited: `429 rate_limited`.
  2. Verified users enter the password: `POST /auth/login {email,password}` only authenticates. Unknown, unverified and wrong-password all return `401 invalid_credentials`; login never sends email and no longer has a 202 path.
- Account enumeration **DECIDED (accepted by the owner):** identify discloses that an email is a verified account. Nothing else leaks (unknown and unverified are indistinguishable). This is accepted in exchange for a login page that no longer asks for a password that would be ignored.
- Rate limits **DECIDED** (app layer, in-memory fixed windows, per process; client IP is `RemoteAddr` only, forwarding headers are never trusted):

  | Limit | Scope | Budget | Counts |
  | --- | --- | --- | --- |
  | identify | per IP | 10 per 15 min | every identify call; deliberately NO per-email limit (it would let anyone lock the owner out) |
  | verification email | per email | 1 per 60 s and 5 per hour | emails actually sent; when exceeded the email is silently not sent, the response stays `verification_sent` and it is logged at Info; suppressed requests do not consume the other budget |
  | login | per email 5, per IP 20 | per 15 min | FAILED attempts only (wrong password, unknown or unverified email); successes and identify calls never consume it; the budget is checked before the password is verified and `429` is returned when exhausted |

  Accepted trade-off: 5 failed logins lock that email for 15 minutes from every IP, so an attacker can delay the owner's password step (not step one, and not verification emails). Revisit with per-IP-and-email keys or a CAPTCHA if it becomes a problem.
- Verification token **DECIDED:** 32 random bytes (base64url), stored SHA-256 hashed, 1 hour lifetime, single use.
- Resend sender **DECIDED:** `onboarding@resend.dev` for now, overridable via `RESEND_FROM`; a custom domain comes later.
- Unverified users **DECIDED:** never receive a session, so there is nothing an unverified session can access.
- Session mechanism **DECIDED:** JWT access token (15 min) plus an opaque refresh token, both returned in the login response body. No cookies.
- Refresh flow **DECIDED:** using a refresh token issues a new JWT and a new refresh token, and invalidates the used one (rotation). Refresh tokens are stored hashed in PostgreSQL.
- Refresh token lifetime **DECIDED:** 7 days.
- JWT signing **DECIDED:** HS256. The secret comes from configuration, never from source.
- Client-side token storage **DECIDED:** localStorage for both tokens. Accepted risk: an XSS bug could read them, so the frontend must avoid `dangerouslySetInnerHTML`, ship a strict Content-Security-Policy, and keep dependencies minimal.
- Refresh token reuse **DECIDED:** presenting an already-used refresh token revokes all of the user's refresh tokens.
- Logout **DECIDED:** `POST /auth/logout` revokes the presented refresh token; the client clears localStorage.

## 6. Data model (draft, to be designed)

Core tables: movements, categories, budgets, clients, instruments, valuations,
invoices (+ uploaded documents), tax filings, month closes, settings.
Document storage for uploaded CFDIs **DECIDED:** S3-compatible object storage. MinIO runs in Docker Compose locally (with a health check), and the app talks to it through the S3 API so the same code targets AWS S3 later. Only metadata (key, name, content type, size, checksum) lives in PostgreSQL. MinIO host ports **DECIDED:** 9100 (S3 API) and 9101 (web console).

## 7. Phases

1. **Foundation:** repo layout, Go module, Postgres via local runtime, migrations, config loading, health endpoint.
2. **Domain port:** pure Go domain for summary, split, portfolio, invoice math, RESICO calculation, with tests.
3. **Auth + shell:** login, session, admin seed with random password and unverified flag, email verification and password change flow via Resend, app layout with tabs.
4. **Modules, in this order (DECIDED):** Settings, Expenses, Income, Savings, Dashboard, Invoices, Tax Filing, Filed Records, Bills & Subscriptions, Month Close.
5. **PWA:** manifest, service worker, installability.
6. **Hardening:** tests across modules, backups, error handling. Edge proxy **DECIDED:** nginx in Docker Compose (with a health check) serves the frontend and reverse-proxies `/api/*` to the backend on the same origin, so CORS is only needed in development. nginx owns per-IP `limit_req` on all routes, body size limits, timeouts, TLS and security headers/CSP. The per-email login limit stays in the app (nginx cannot read the JSON body). Because the backend ignores `X-Forwarded-For`, it must trust that header only when it comes from nginx (or drop its per-IP limit), otherwise every client shares one IP bucket. On AWS the ALB/CloudFront/WAF play this role (phase 8). Also in this phase: rate limit `/auth/refresh` and `/auth/verify`.
7. **Email reminders (later, after the app works locally):** scheduled job for expiring and unregistered bills. Email provider **DECIDED:** Resend. Schedule **OPEN**.
8. **AWS + Terraform (later):** network, database, compute, secrets, HTTPS, backups.

## 8. Open decisions

1. ~~Money representation~~ **DECIDED:** exact decimal (`shopspring/decimal` + `NUMERIC`).
2. ~~Frontend framework~~ **DECIDED:** React + TypeScript. Build tool **DECIDED:** Vite. Routing, data fetching and styling still **OPEN**.
3. ~~How to run locally~~ **DECIDED:** Docker Compose. Host port **DECIDED:** 5442 (container stays 5432). Postgres **DECIDED:** 18 (exact image tag pinned when the compose file is written). Health checks are mandatory in `docker-compose.yml`.
4. ~~Password storage~~ **DECIDED:** admin seeded with a random password (bcrypt hash in PostgreSQL), unverified flag, forced email verification and password change via Resend; nothing seeded from env vars. ~~Session mechanism~~ **DECIDED:** JWT 15 min + rotating refresh token in response body. Remaining auth details listed in section 5.
5. ~~Module build order~~ **DECIDED** (see phase 4).
6. ~~CFDI upload storage~~ **DECIDED:** MinIO in Docker Compose (S3 API). Host ports **DECIDED:** 9100 (API), 9101 (console).
7. ~~How the non-emergency savings categories are shown~~ **DECIDED:** Portfolio tab dropped; Savings covers all savings categories.
8. ~~Official Mexican terms~~ **DECIDED:** CFDI, RFC, SAT, RESICO, IVA, ISR, aguinaldo and similar official terms stay as-is everywhere (UI text is Spanish, code and docs English; see the language rule at the top).
9. ~~Data import~~ **DECIDED:** seed from `~/finances/config.json` with a one-time import (categories, budgets, clients, instruments, tax parameters, split rules). Import mechanics **DECIDED:** `make import-config` (`backend/cmd/import-config`) reads `~/finances/config.json`, maps the Spanish keys to the domain and writes every settings section in one transaction. It is idempotent: it refuses to run on non-empty settings unless `FORCE=1`. Only configuration is imported (the file's `fx_rate_source`, `first_month` and free-text `*_nota` keys are not modelled, except the notes of the issuer and the pause). The single existing movement (an Ingreso) **DECIDED:** imported with the Income module by `make import-movements` (`backend/cmd/import-movements`, reads `~/finances/movimientos.csv`, keeps `creado` as `created_at`, recomputes `amount_mxn` rounded to cents so it is 60020.27 instead of the CSV's 60020.2742). It is idempotent: it refuses to run on a non-empty `movements` table unless `FORCE=1` (which appends, never deletes). Done on the dev database.
10. ~~Branch name~~ **DECIDED:** `main` (already renamed). Remote hosting (GitHub or other) still **OPEN**, deferred until the first push.

## 9. Reference

Source of truth for current behavior: `~/finances/fin.py` and `~/finances/config.json`.
