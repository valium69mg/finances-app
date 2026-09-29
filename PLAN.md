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
| Income | Register income (Sezzle in USD, client B in MXN) with the automatic split rule. Replaces `/ingreso`. |
| Expenses | Register expenses with category inference. Replaces `/gasto`. |
| Savings | All savings: emergency fund, aguinaldo and vacation, future expenses, investments and SAT reserve. Replaces `/ahorro` and `/portafolio` (the Portfolio tab is dropped). Instrument tracking **DECIDED:** each savings entry picks an instrument (e.g. GBM liquidity, CETES 28/91, VOO, VXUS), and instrument valuations can be recorded to show current value and gains. Valuations are entered manually; no external price API. Details **OPEN**. |
| Invoices | Create invoices, update their status, upload issued CFDI documents. Replaces `/factura`. |
| Tax Filing | Compute the monthly RESICO tax. Optionally record it as an expense when paid, or mark it pending. Replaces `/declaracion`. |
| Filed Records | History of filed declarations and payments. Replaces `/declarado`. |
| Month Close | Monthly snapshot: over-budget categories, suggestion for where to put the leftover money (emergency fund first, then investments), budget-adjustment hints for deviations over 20%. Stored in the database. Replaces `/cierre-mes`. |
| Bills & Subscriptions | Recurring bills and subscriptions (e.g. monthly water bill) with periodic reminders. A bill occurrence can be marked paid, which registers an expense. Later: a scheduled job that emails about bills that are about to expire or were never registered. Recurrence **DECIDED:** weekly, biweekly, monthly, bimonthly, yearly (no custom intervals). Amount **DECIDED:** a bill has either a fixed amount or no amount. When marking it paid, the expense amount defaults to the bill's amount when it has one, and can always be overridden (discounts, price changes). Overdue occurrences **DECIDED:** stay pending and flagged overdue until paid or manually skipped (skip registers no expense; no auto-skip). Reminder lead time **DECIDED:** configurable per bill. Category **DECIDED:** each bill has a default expense category, changeable when marking it paid. Occurrences **DECIDED:** only the next occurrence exists at a time; paying or skipping it generates the following one. Module design is otherwise settled. |
| Settings | Categories, budgets, clients, instruments, tax parameters (today's `config.json`). Also carries the investment pause plan (`inversiones_pausa` **DECIDED:** carried forward as configuration: paused months, normal budget, resume month, monthly future-expenses plan). How the pause affects budgets and the split is **OPEN**: `fin.py` does not read it, so the behavior must be specified before Settings is built. |

Note: there is no separate Portfolio module. **DECIDED:** Savings covers every savings
category (emergency fund, aguinaldo and vacation, future expenses, investments, SAT reserve).

## 5. Auth (v1)

- Login page. Single admin user, email `carlostranquilino.cr@gmail.com`.
- Seeding **DECIDED:** the admin is seeded into PostgreSQL with only that email and a random password (stored as a bcrypt hash). No password or hash comes from `.env`, and no plaintext credential is ever in source or git history. The seed marks the user **unverified** (flag on the user row). A `.env.example` with placeholders is committed; the real `.env` is not.
- Email verification **DECIDED:** while the unverified flag is on, logging in leads to a forced flow: the app sends the user an email through Resend to verify the address and change the password. Completing it clears the flag. Resend API key comes from `RESEND_API_KEY` (in `.env`, placeholder in `.env.example`).
- First credential **DECIDED:** the admin never needs the random password. Submitting the login form for an unverified user sends the verification email (verify the address and set a new password) instead of authenticating. The response is identical whether or not the email exists or is verified, so it cannot be used to probe accounts, and sending is rate limited.
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
6. **Hardening:** tests across modules, backups, error handling.
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
9. ~~Data import~~ **DECIDED:** seed from `~/finances/config.json` with a one-time import (categories, budgets, clients, instruments, tax parameters, split rules). Whether the single existing movement is imported too, and the import mechanics, are **OPEN**.
10. ~~Branch name~~ **DECIDED:** `main` (already renamed). Remote hosting (GitHub or other) still **OPEN**, deferred until the first push.

## 9. Reference

Source of truth for current behavior: `~/finances/fin.py` and `~/finances/config.json`.
