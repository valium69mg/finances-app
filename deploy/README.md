# Production deployment (single VM, Cloudflare in front)

One VM runs the whole stack with Docker Compose. Cloudflare (orange-cloud proxy) is the
only public entry point; nginx on the VM terminates TLS with a Cloudflare Origin CA
certificate, serves the frontend and proxies `/api/*` to the backend on the same origin.

```
browser --HTTPS--> Cloudflare --HTTPS (Full strict)--> VM:443 nginx --+--> static SPA
                                                                      +--> /api/* -> api -> postgres
                                                                                        \-> minio (until the S3 cutover)
                                                                                        \-> AWS S3 (after it; instance role)
```

- Only **nginx** publishes ports (80 and 443). `api`, `postgres` and `minio` live on an
  internal Docker network with no route to the internet (the API also joins a second
  network for outbound HTTPS to Resend).
- The dev `docker-compose.yml` at the repo root is unrelated and untouched.
- **Backups:** a nightly `pg_dump` goes to a private S3 bucket (section 11), and the CFDI
  and acuse files move from MinIO to the same bucket in the cutover (section 12). Daily
  EBS snapshots of the data volume (Terraform, DLM) stay as a second net.

Files:

| Path | Purpose |
| --- | --- |
| `docker-compose.prod.yml` | the stack: nginx, api, migrate (one-shot), postgres, minio |
| `prod.env.example` | environment template (placeholders only) |
| `backend.Dockerfile`, `frontend.Dockerfile` | multi-stage, non-root images |
| `nginx/` | nginx.conf, site template, snippets (headers, CSP, proxy), Cloudflare ranges |
| `scripts/refresh-cloudflare-ips.sh` | regenerates `nginx/cloudflare-ips.conf` |
| `scripts/gen-dev-cert.sh` | self-signed certificate for local smoke tests only |
| `scripts/backup-postgres.sh`, `scripts/install-postgres-backup.sh` | nightly Postgres backup to S3 and its systemd timer (section 11) |
| `scripts/restore-postgres.sh` | restore a backup from the laptop (section 11) |

## 1. Prerequisites

- A VM with 2 GB RAM (Linux), Docker Engine and the Compose plugin, `git`, `openssl`, `curl`.
  The stack is capped at about 1.1 GB (nginx 128 MB, api 192 MB, postgres 512 MB,
  minio 256 MB).
- A domain on Cloudflare (this README says `finances.example.com`; nothing in the repo
  hardcodes your real one, it comes from `DOMAIN` in the env file).
- A Resend API key (email verification). `onboarding@resend.dev` only delivers to the Resend
  account owner; verify your own sending domain in Resend for anything else.
- Turn on **two-factor authentication on the Cloudflare account** (Profile > Authentication),
  and on the registrar and the VM provider. Whoever controls the DNS controls the site, the
  origin certificate and the traffic.

## 2. Cloudflare setup (dashboard, once)

1. **DNS**: add an `A` record (and `AAAA` if the VM has IPv6) for your host name pointing to
   the VM public IP, with the **Proxied (orange cloud)** status. Never leave it grey: the
   origin would be exposed and the client IP logic below would not apply.
2. **Origin certificate**: SSL/TLS > Origin Server > *Create Certificate*. Private key type
   RSA 2048 (or ECDSA), hostnames = your host name, validity 15 years. Copy the certificate
   and the private key (the key is shown once) to the VM, see step 3 below.
3. **SSL/TLS mode**: SSL/TLS > Overview > set **Full (strict)**. Not Flexible (plain HTTP to
   the origin) and not Full (it would accept any certificate).
4. **Always Use HTTPS**: SSL/TLS > Edge Certificates > enable *Always Use HTTPS*, set the
   *Minimum TLS Version* to 1.2. nginx already sends HSTS; do not enable
   `includeSubDomains` preload at Cloudflare unless every subdomain is HTTPS.
5. **Authenticated Origin Pulls (optional, recommended)**: SSL/TLS > Origin Server > enable
   *Authenticated Origin Pulls*. Then make nginx require Cloudflare's client certificate
   (see "Authenticated Origin Pulls" below) so nothing but Cloudflare can complete a TLS
   handshake with the origin.
6. **Caching**: leave the defaults. The API and the HTML shell answer `no-store`; hashed
   `/assets/*` are cached for a year by the browser and Cloudflare. `/manifest.webmanifest`
   answers `no-store` and the home-screen icon PNGs cache for one day. Do not add a
   "cache everything" rule.
7. **Optional**: Security > Bots / WAF rules on top; they do not replace the limits below.

## 3. VM preparation

```sh
# Firewall / security group: allow 443 (and 80 only if you want the HTTP redirect) from the
# Cloudflare ranges ONLY, plus SSH from your own address.
```

Restrict the inbound rule to Cloudflare's ranges (`nginx/cloudflare-ips.conf` lists them, or
https://www.cloudflare.com/ips-v4 and /ips-v6). With a cloud **security group** this is one
rule per range. On the VM itself note that **Docker publishes ports through iptables and
bypasses `ufw`/firewalld rules**: filter in the provider's security group, or add rules to
the `DOCKER-USER` chain, for example:

```sh
for r in $(curl -fsS https://www.cloudflare.com/ips-v4); do
  sudo iptables -I DOCKER-USER -p tcp --dport 443 -s "$r" -j ACCEPT
done
sudo iptables -A DOCKER-USER -p tcp --dport 443 -j DROP   # evaluated after the ACCEPTs
```

(Persist it with your distribution's mechanism, and do the same with `ip6tables` for IPv6.)
The real client address must reach nginx: with the default Docker setup it does for IPv4
(check the nginx access log shows a Cloudflare or client address, not `172.x`).

Install the origin certificate where `TLS_CERT_DIR` points:

```sh
sudo mkdir -p /etc/finances/certs
sudo install -m 644 origin.pem /etc/finances/certs/origin.pem
# nginx runs as uid 101 inside the container and must read the key:
sudo install -m 400 -o 101 -g 101 origin.key /etc/finances/certs/origin.key
```

## 4. Configuration

```sh
git clone <repo> finances-app && cd finances-app
cp deploy/prod.env.example deploy/.env.prod
chmod 600 deploy/.env.prod
$EDITOR deploy/.env.prod        # fill EVERY value
```

`deploy/.env.prod` is gitignored. Generate each secret with `openssl rand -hex 32` (use hex
or alphanumeric characters: passwords go into connection URLs). Every secret in the example
contains `CHANGE_ME`: the API **refuses to start** while one is left (and `docker compose`
refuses to start while a required variable is missing). Variables:

| Variable | Meaning |
| --- | --- |
| `DOMAIN` | public host name, no scheme (nginx `server_name`, redirects) |
| `APP_BASE_URL` | optional, defaults to `https://$DOMAIN` (email links, CORS origin) |
| `POSTGRES_*`, `MINIO_ROOT_*`, `JWT_SECRET`, `RESEND_API_KEY`, `RESEND_FROM` | secrets and sender |
| `REMINDERS_ENABLED`, `TZ_NAME`, `DISK_ALERT_PCT` | optional: email reminders on/off (default `true`), zone that defines "today" (default `America/Mexico_City`), used-disk percent that triggers the alert (default `80`); the disk is measured through the empty `/srv/finances/probe` directory mounted at `/probe` |
| `S3_ENDPOINT`, `S3_USE_SSL`, `S3_REGION`, `S3_BUCKET`, `S3_USE_IAM` | object storage; MinIO inside the stack by default. At the cutover set `s3.us-east-2.amazonaws.com`, `true`, `us-east-2`, the bucket from `terraform output data_bucket_name` and `true` (instance role, no keys; section 12) |
| `BACKUP_STATUS_PATH` | optional: `/backup/last-success` arms the backup alert and the "Último respaldo" line of the Sistema tab (set it after the first backup succeeded); empty = off |
| `TLS_CERT_DIR` | host directory holding `origin.pem` and `origin.key` |
| `HTTP_PORT`, `HTTPS_PORT` | published ports, default 80 and 443 |
| `TRUSTED_PROXIES` | peers whose `X-Real-IP` the API believes; default is the nginx container |

The API runs with `APP_ENV=production`: it requires an explicit `https` `APP_BASE_URL`,
rejects placeholder secrets and logs JSON lines.

Use this shorthand for the commands below:

```sh
alias dcp='docker compose --env-file deploy/.env.prod -f deploy/docker-compose.prod.yml'
```

## 5. First start

```sh
dcp up -d --build --wait
dcp ps
curl -fsS https://finances.example.com/api/healthz      # {"status":"ok"}
```

Startup order is enforced with health checks: `postgres` (healthy) -> `migrate` (runs
`golang-migrate up` over `migrations/` and exits) -> `api` (healthy) -> `nginx` (healthy).
If `migrate` fails, the API does not start; read `dcp logs migrate`.

Create the admin user once (random password, unverified; nothing is seeded from env vars):

```sh
dcp exec api /seed
```

Then open the site, enter the admin email and follow the verification email to set the
password (see PLAN.md section 5).

## 6. Updating

```sh
git pull
dcp up -d --build --wait      # rebuilds changed images; `migrate` re-runs and applies new migrations
```

nginx configuration is bind-mounted, so config-only changes need `dcp restart nginx`.
To re-run the migrations alone: `dcp run --rm migrate` (its default command is `up`). To
check the version or roll one migration back (deliberate and manual) pass the full
arguments:

```sh
set -a; . deploy/.env.prod; set +a
URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@postgres:5432/$POSTGRES_DB?sslmode=disable"
dcp run --rm migrate -path=/migrations -database "$URL" version
dcp run --rm migrate -path=/migrations -database "$URL" down 1
```

## 7. Logs, health and operations

```sh
dcp logs -f nginx api           # nginx access log (path only, no query strings), api JSON log
dcp ps                          # health of every container
dcp exec postgres psql -U finances finances
dcp restart api
```

- Container logs rotate (3 files of 10 MB each).
- The API logs one JSON line per request: method, path (no query), route pattern, status,
  size, duration, client IP and request id (`X-Request-ID`, shared with nginx's `id=`).
  Tokens, passwords and bodies are never logged. `/healthz` probes log at debug level.
- Stopping the API (`docker stop`) drains in-flight requests for up to 10 s.
- **Docker cleanup:** every deploy rebuilds images on the VM, so the build cache and old
  images pile up on the data volume. Install the weekly cleanup once, from the checkout on
  the VM: `sudo deploy/scripts/install-docker-prune.sh` (Sundays 04:00 server time; it removes
  build cache and unused images older than a week, never volumes). Check it with
  `systemctl list-timers finances-docker-prune.timer` and `journalctl -u finances-docker-prune`.
  Run it by hand with `sudo systemctl start finances-docker-prune.service`.

## 8. Edge behavior

| What | Setting |
| --- | --- |
| Rate limit `/api/auth/*` | 20 req/min per real client IP, burst 10, then `429 {"error":"rate_limited"}` |
| Rate limit other `/api/*` | 20 req/s, burst 40 (burst 10 on the upload routes) |
| Rate limit static app | 20 req/s, burst 60 |
| Connections | 30 per IP |
| Body size | 1 MiB; `POST /api/invoices/{id}/issue` and `/documents` and `PUT /api/tax-filing/{period}/documents/{kind}` up to 13 MiB (10 MiB file + 1 MiB XML + overhead); larger -> `413 {"error":"request_too_large"}` |
| Timeouts | headers 10 s, body 30 s (60 s uploads), upstream 5 s connect and 60 s read |
| Headers | CSP without `unsafe-inline`/`unsafe-eval`, HSTS (1 year), nosniff, `Referrer-Policy: no-referrer`, Permissions-Policy, `frame-ancestors 'none'`, COOP/CORP |
| Unknown Host or SNI | connection closed / TLS handshake refused (only `DOMAIN` is served) |

These limits sit in front of the application limits (per-email login, verification email,
identify, refresh, verify; see PLAN.md section 5), which stay in the API.

### Client IP and the trusted proxy

nginx resolves the client address with the real_ip module: `CF-Connecting-IP` is honored
**only when the TCP peer is a Cloudflare range** (`nginx/cloudflare-ips.conf`); from any other
peer the header is ignored, so a request that reaches the origin directly cannot pick its own
rate-limit bucket. nginx then *overwrites* `X-Real-IP` and `X-Forwarded-For` towards the API
with that address and drops `CF-Connecting-IP`, `True-Client-IP` and `Forwarded`. The API
believes `X-Real-IP` only when the direct peer is inside `TRUSTED_PROXIES` (the nginx
container, fixed at `172.29.42.10`); anything else uses the socket peer. An empty
`TRUSTED_PROXIES` (the default outside this stack) trusts nothing.

**Refresh the Cloudflare ranges occasionally** (every few months, or when Cloudflare
announces a change). An outdated list means new Cloudflare ranges are not trusted and their
clients share the Cloudflare edge address as their IP:

```sh
deploy/scripts/refresh-cloudflare-ips.sh
git diff deploy/nginx/cloudflare-ips.conf     # review, commit
dcp restart nginx
```

### Authenticated Origin Pulls

```sh
curl -fsSo /etc/finances/certs/authenticated_origin_pull_ca.pem \
  https://developers.cloudflare.com/ssl/static/authenticated_origin_pull_ca.pem
cp deploy/nginx/optional/authenticated-origin-pulls.conf.example \
   deploy/nginx/optional/authenticated-origin-pulls.conf
dcp restart nginx
```

Enable the feature in the Cloudflare dashboard as well. From then on a direct request to the
origin without Cloudflare's client certificate fails the TLS handshake.

## 9. Local smoke test of this stack

Runs the same stack on other ports, with its own project name, volumes and throwaway
secrets, and a self-signed certificate. It does not touch the dev containers.

```sh
deploy/scripts/gen-dev-cert.sh /tmp/finances-certs localhost
cat > /tmp/smoke.env <<'EOF'            # any editor works; placeholder values are rejected
COMPOSE_PROJECT_NAME=finances-smoke
DOMAIN=localhost
POSTGRES_PASSWORD=<random hex>
MINIO_ROOT_USER=smokeminio
MINIO_ROOT_PASSWORD=<random hex>
JWT_SECRET=<openssl rand -hex 32>
RESEND_API_KEY=re_smoke_key
TLS_CERT_DIR=/tmp/finances-certs
HTTP_PORT=8081
HTTPS_PORT=8443
EOF
docker compose --env-file /tmp/smoke.env -f deploy/docker-compose.prod.yml up -d --build --wait
curl -k https://localhost:8443/api/healthz
docker compose --env-file /tmp/smoke.env -f deploy/docker-compose.prod.yml down -v   # also deletes its volumes
```

From a non-Cloudflare peer (your own machine) `CF-Connecting-IP` and `X-Forwarded-For` are
ignored, so the IP the API logs is the socket peer.

## 10. Troubleshooting

- `api` exits at start: `dcp logs api`. The message names the variable and the rule, never the
  value (missing, too short, placeholder, `APP_BASE_URL` not https).
- `nginx` unhealthy: `dcp logs nginx`; usually a missing `origin.pem`/`origin.key`, or a key
  not readable by uid 101.
- 525/526 from Cloudflare: the origin certificate is missing, expired or not for this host
  name, or the SSL mode does not match (must be Full strict with an Origin CA certificate).
- 521: the VM firewall blocks Cloudflare, or nginx is down.
- Everyone shares one IP in the API log and hits `429` together: `TRUSTED_PROXIES` does not
  contain nginx's address (keep the default), or the Cloudflare list is stale.
- Invoice files answer `503 storage_unavailable`: MinIO is unhealthy (`dcp ps`), or with S3
  the instance role cannot reach the bucket (`dcp logs api`; check `S3_BUCKET`, that the
  IMDS hop limit is 2 and that the bucket exists). It recovers on its own once it is back.
- The nightly backup failed: `journalctl -u finances-postgres-backup.service -n 50`.
  Typical causes: the postgres container is down, `S3_BUCKET` in `.env.prod` is not the AWS
  bucket, or the aws-cli image could not be pulled. The owner also gets an email once a
  day while the last success is older than 36 hours.

## 11. Backups (Postgres to S3)

Nightly at 03:00 `America/Mexico_City` a systemd timer runs `scripts/backup-postgres.sh`:

1. `pg_dump -Fc` inside the running postgres container, written to a temp file in
   `/srv/finances/backup-tmp` (the data volume, never the 12 GB root disk; root-only, removed
   at the end). It is a file and not a pipe on purpose: the VM role can only **put** backups,
   it cannot delete them, so a truncated dump has to be caught before the upload.
2. `pg_restore --list` over the dump (a dump that cannot be read is not uploaded).
3. Upload to `s3://<bucket>/backups/daily/YYYY-MM-DD.dump`, and on day 1 of the month also to
   `backups/monthly/YYYY-MM.dump`, with the instance role (no keys).
4. Write `/srv/finances/backup/last-success` (atomic rename): line 1 the UTC timestamp, line 2
   `size_bytes=N` (the dump size; the API also accepts the old timestamp-only file).

Any failure exits non-zero: the unit fails, the journal has the reason and `last-success` is
not touched. Retention (bucket lifecycle, Terraform): daily dumps 30 days, monthly 365 days.

**aws-cli choice.** The VM has no AWS CLI and Ubuntu 24.04 has no apt package for it, so the
script runs the official `amazon/aws-cli` image pinned by tag **and** digest
(`2.37.12@sha256:ce44...`, multi-arch, arm64 ok). It needs no host install, pins the version
and gets the instance role through IMDSv2 from the default bridge network (hop limit 2). The
cost is one image pull on the first run (about 140 MB on the data volume's containerd store).
Bump it by hand like the other pinned images (`BACKUP_AWS_CLI_IMAGE` overrides it).

**The VM cannot read or delete backups.** The role has `PutObject` on `backups/*` and an
explicit Deny for read and delete there. Restores and any cleanup use your admin AWS
credentials from your laptop.

### Install (VM, once, after the cutover step "switch to S3")

```sh
sudo deploy/scripts/install-postgres-backup.sh           # units, directories, enable the timer
sudo systemctl start finances-postgres-backup.service    # first run by hand
journalctl -u finances-postgres-backup.service -n 30 --no-pager
cat /srv/finances/backup/last-success
systemctl list-timers finances-postgres-backup.timer
```

Requires `S3_BUCKET` in `deploy/.env.prod` to be the AWS bucket (the script refuses the MinIO
default). Then verify from the laptop: `aws s3 ls s3://<bucket>/backups/daily/`.

### Alert and Sistema tab

Set `BACKUP_STATUS_PATH=/backup/last-success` in `.env.prod` and `dcp up -d --wait api`
**after** the first backup succeeded. The API mounts `/srv/finances/backup` read-only and:

- emails the owner (`reminder_log` key `backup:YYYY-MM-DD`, at most once per day, from 08:00
  local time) when the file is missing, unreadable or older than 36 hours;
- emails the owner "Respaldo de la base de datos completado" (date/time in Mexico City and the
  dump size) once per successful backup: `reminder_log` key `backup-done:<UTC timestamp>`, sent
  at the first tick after 08:00 local time (the 03:00 backup is announced at 08:00). Only the
  latest success is announced, and only if it is under 36 hours old, so enabling the setting
  never sends a burst for old backups. The VM script never holds the Resend key;
- shows "Último respaldo" (time in Mexico City and Al día / Atrasado / Sin registro) on the
  Sistema tab.

Leave it empty in development (no file, no alert, no completion email). Enabling it before the
first backup sends a "no hay registro" email; enabling it after the first backup sends one
"completado" email for that backup.

### Restore (from the laptop)

You need the AWS CLI with admin credentials, and `pg_restore` 18 or newer (or docker: the
script falls back to a `postgres:18.6` container on the host network when the local client
is missing or older, since an older `pg_restore` cannot read the server's dumps).
First drill (2026-10-10): the restored copy matched production row for row (movements,
invoices, documents, users) at migration 26.

```sh
BUCKET=<terraform output -raw data_bucket_name>      # from infra/
aws s3 ls s3://$BUCKET/backups/daily/
aws s3 ls s3://$BUCKET/backups/monthly/

# target database must already exist; its contents are replaced
BUCKET=$BUCKET TARGET_DATABASE_URL='postgres://USER:PASSWORD@HOST:PORT/DBNAME' \
  deploy/scripts/restore-postgres.sh backups/daily/2026-10-10.dump
```

The script downloads the dump to a private temp dir, checks it with `pg_restore --list`, asks
you to type the target database name and runs `pg_restore --clean --if-exists --no-owner
--no-privileges`. Dumps are plain `-Fc` archives of the whole database, so every table,
including `reminder_log` and `schema_migrations`, comes back as it was.

**Restore drill against the dev database (do it once after the first backup, then each
quarter).** It never touches production:

```sh
set -a; . ./.env; set +a
docker exec finances-app-postgres-1 psql -U "$POSTGRES_USER" -d postgres \
  -c 'CREATE DATABASE finances_restore_drill'
BUCKET=$BUCKET \
  TARGET_DATABASE_URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@localhost:5442/finances_restore_drill" \
  deploy/scripts/restore-postgres.sh backups/daily/YYYY-MM-DD.dump
docker exec finances-app-postgres-1 psql -U "$POSTGRES_USER" -d finances_restore_drill \
  -c 'select count(*) from movements' -c 'select max(version) from schema_migrations'
docker exec finances-app-postgres-1 psql -U "$POSTGRES_USER" -d postgres \
  -c 'DROP DATABASE finances_restore_drill'
```

Compare the counts with production (`dcp exec postgres psql ...`). A drill that restores and
shows the expected row counts is the only proof the backups work.

**Disaster restore into production** (new VM or lost database): download the dump on the
laptop, copy it to the VM, stop the API (`dcp stop api`), create an empty database in the
postgres container (`createdb`), run `docker exec -i <postgres> pg_restore --no-owner
--clean --if-exists -U finances -d finances < dump`, then `dcp up -d --wait`. The API
re-runs `migrate`, which is a no-op when the dump is current.

## 12. MinIO to S3 cutover (runbook, do not run until the owner approves)

Order matters; each stage lists its rollback. Documents live under the prefixes
`invoices/` and `tax-filings/`; the bucket also receives `backups/`. `<bucket>` is the
generated name (`terraform output -raw data_bucket_name`), never written in the repository.
On the VM use these shorthands (`dcp` as in section 4):

```sh
cd ~/finances-app
IMG=amazon/aws-cli:2.37.12@sha256:ce44b800602264c12298bbf3399055a4cefdb9ed4a098abe564afe26e7071bce
BUCKET=<bucket>
```

**1. Terraform (laptop, owner approval).** `cd infra && terraform plan` should show **8 to
add, 1 to change, 0 to destroy**: the bucket and its configuration (public access block,
ownership, encryption, versioning, lifecycle, policy), the role policy `data-bucket-access`
and the in-place change of the instance's `http_put_response_hop_limit` 1 -> 2 (no restart,
no replacement). Apply only after reading the plan: `terraform apply`. Then
`terraform output -raw data_bucket_name`.
*Rollback:* nothing uses the bucket yet. Set the hop limit back to 1 and apply if desired;
the bucket is `prevent_destroy` (empty it and remove the lifecycle block on purpose to delete).

**2. Deploy the code with IAM support, still on MinIO.** `git pull && dcp up -d --build --wait`.
`S3_USE_IAM` defaults to false, so nothing changes. Check `dcp ps` and open a document.
*Rollback:* `git checkout <previous commit>` and `dcp up -d --build --wait`.

**3. Check the role from a container.**

```sh
docker run --rm $IMG sts get-caller-identity          # shows the instance role, not an error
echo ok | docker run --rm -i $IMG s3 cp - s3://$BUCKET/invoices/.cutover-check
docker run --rm $IMG s3 rm s3://$BUCKET/invoices/.cutover-check
```

If `sts` fails with a credentials error the hop limit is still 1: check the apply in step 1.
*Rollback:* none needed.

**4. Copy the existing objects (MinIO -> bucket), twice.** The MinIO store is not plain
files, so the objects are exported through its S3 API into a private directory on the data
volume, then synced to the bucket with the role. The first pass can run while the app is in
use; the second one (step 5) catches whatever was uploaded in between.

```sh
# .env.prod is read by Compose, not by the shell (a value like "Name <addr>" breaks
# sourcing it), so read only the three values needed
envval() { grep -m1 "^$1=" deploy/.env.prod | cut -d= -f2- | sed -e "s/^[\"']//" -e "s/[\"']$//"; }
export AWS_ACCESS_KEY_ID="$(envval MINIO_ROOT_USER)" AWS_SECRET_ACCESS_KEY="$(envval MINIO_ROOT_PASSWORD)"
MINIO_BUCKET="$(envval S3_BUCKET)"; MINIO_BUCKET="${MINIO_BUCKET:-finances-invoices}"   # still the MinIO bucket here
# run docker without sudo (ubuntu is in the docker group): sudo drops the -e variables
sudo install -d -m 700 -o ubuntu -g ubuntu /srv/finances/migration

# a) MinIO -> directory (keys are passed through the environment, not the command line)
docker run --rm --network finances-prod_backend \
  -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_DEFAULT_REGION=us-east-1 \
  -v /srv/finances/migration:/export $IMG \
  --endpoint-url http://minio:9000 s3 sync --only-show-errors "s3://$MINIO_BUCKET" /export
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY

# b) directory -> bucket with the instance role, one prefix at a time
for prefix in invoices tax-filings; do
  [ -d "/srv/finances/migration/$prefix" ] || continue
  docker run --rm -e AWS_DEFAULT_REGION=us-east-2 -v /srv/finances/migration:/export:ro $IMG \
    s3 sync --only-show-errors "/export/$prefix" "s3://$BUCKET/$prefix/"
done
```

(`finances-prod_backend` is the network of the default `COMPOSE_PROJECT_NAME`.) Verify object
count **and** bytes on both sides; they must be equal:

```sh
# MinIO side: what the export holds
sudo find /srv/finances/migration -type f | wc -l
sudo find /srv/finances/migration -type f -printf '%s\n' | awk '{s+=$1} END {print s}'
# Bucket side, with the role (it may list the document prefixes only)
for prefix in invoices tax-filings; do
  docker run --rm -e AWS_DEFAULT_REGION=us-east-2 $IMG s3 ls "s3://$BUCKET/$prefix/" --recursive --summarize | tail -2
done
```

Add the two prefixes' "Total Objects" and "Total Size". From the laptop with admin
credentials the whole-bucket figure is `aws s3 ls s3://$BUCKET --recursive --summarize`
(it also counts `backups/`). Also compare against the database: `select count(*) from
invoice_documents` plus `tax_filing_documents` equals the object count.
*Rollback:* none needed (MinIO is untouched; delete the copied keys with admin credentials
if you want to start over).

**5. Switch the API to S3.** Run step 4a and 4b once more right before this, then edit
`deploy/.env.prod` and uncomment/set the five lines (the MinIO keys stay, they are ignored):

```
S3_ENDPOINT=s3.us-east-2.amazonaws.com
S3_USE_SSL=true
S3_REGION=us-east-2
S3_BUCKET=<bucket>
S3_USE_IAM=true
```

Then `dcp up -d --wait api`. `dcp logs api | head` must show no "object storage" warning.
*Rollback:* put the five lines back (comment them out; `S3_BUCKET` back to the MinIO bucket
or unset) and `dcp up -d --wait api`. Anything uploaded after the switch exists only in S3:
copy it back first (`aws s3 sync` from the bucket) if you roll back after real uploads.

**6. Verify with real documents.** In the app download an existing CFDI XML or PDF (an issued
invoice) and an acuse; then upload a new file (for example a payment comprobante on a
filing) and confirm from the laptop that the key appeared:
`aws s3 ls s3://$BUCKET/tax-filings/ --recursive`. Delete nothing yet.
*Rollback:* as step 5.

**7. Install the backup timer and run it once.** Section 11: `sudo
deploy/scripts/install-postgres-backup.sh`, start the service by hand, check the journal,
then from the laptop `aws s3 ls s3://$BUCKET/backups/daily/`. Run the restore drill. Only then
set `BACKUP_STATUS_PATH=/backup/last-success` in `.env.prod` and `dcp up -d --wait api`.
*Rollback:* `sudo systemctl disable --now finances-postgres-backup.timer`, empty
`BACKUP_STATUS_PATH`.

**8. Remove MinIO (separate, later, owner confirms).** Wait until the app has run on S3 for a
while and you are sure nothing is missing. The change is prepared as one separate commit on
the branch `remove-minio-after-cutover` (not merged into `main` on purpose, so a routine
`git pull` cannot drop MinIO early): it removes the `minio` service and the API dependency on
it, makes the S3 variables required, drops `MINIO_ROOT_*` from the compose file and the
env example, and keeps the `minio-data` volume declared so it is **not** deleted. Apply it
(`git merge remove-minio-after-cutover`), then `dcp up -d --remove-orphans`. The MinIO data
stays on disk (`docker volume ls | grep minio-data`, plus `/srv/finances/migration`) until
you delete it by hand: `docker volume rm finances-prod_minio-data` and
`sudo rm -rf /srv/finances/migration` (the export holds every document in clear text, remove
it once the bucket is verified). Local development keeps MinIO in the dev compose.
*Rollback:* before the volume is removed, `git revert` the merge and restore the MinIO
variables; documents uploaded since the cutover must be synced back from the bucket first.
