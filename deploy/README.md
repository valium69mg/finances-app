# Production deployment (single VM, Cloudflare in front)

One VM runs the whole stack with Docker Compose. Cloudflare (orange-cloud proxy) is the
only public entry point; nginx on the VM terminates TLS with a Cloudflare Origin CA
certificate, serves the frontend and proxies `/api/*` to the backend on the same origin.

```
browser --HTTPS--> Cloudflare --HTTPS (Full strict)--> VM:443 nginx --+--> static SPA
                                                                      +--> /api/* -> api -> postgres
                                                                                        \-> minio
```

- Only **nginx** publishes ports (80 and 443). `api`, `postgres` and `minio` live on an
  internal Docker network with no route to the internet (the API also joins a second
  network for outbound HTTPS to Resend).
- The dev `docker-compose.yml` at the repo root is unrelated and untouched.
- **Backups are out of scope for now** (deferred with the MinIO to S3 migration). The data
  lives in two Docker volumes (`postgres-data`, `minio-data`): until backups exist, a lost
  VM disk is lost data. Take a VM or volume snapshot with your provider.

Files:

| Path | Purpose |
| --- | --- |
| `docker-compose.prod.yml` | the stack: nginx, api, migrate (one-shot), postgres, minio |
| `prod.env.example` | environment template (placeholders only) |
| `backend.Dockerfile`, `frontend.Dockerfile` | multi-stage, non-root images |
| `nginx/` | nginx.conf, site template, snippets (headers, CSP, proxy), Cloudflare ranges |
| `scripts/refresh-cloudflare-ips.sh` | regenerates `nginx/cloudflare-ips.conf` |
| `scripts/gen-dev-cert.sh` | self-signed certificate for local smoke tests only |

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
   `/assets/*` are cached for a year by the browser and Cloudflare. Do not add a
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

## 8. Edge behavior

| What | Setting |
| --- | --- |
| Rate limit `/api/auth/*` | 20 req/min per real client IP, burst 10, then `429 {"error":"rate_limited"}` |
| Rate limit other `/api/*` | 20 req/s, burst 40 (burst 10 on the upload routes) |
| Rate limit static app | 20 req/s, burst 60 |
| Connections | 30 per IP |
| Body size | 1 MiB; `POST /api/invoices/{id}/issue` and `/documents` up to 13 MiB (10 MiB PDF + 1 MiB XML + overhead); larger -> `413 {"error":"request_too_large"}` |
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
- Invoice files answer `503 storage_unavailable`: MinIO is unhealthy (`dcp ps`); it recovers
  on its own once it is back.
