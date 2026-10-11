#!/usr/bin/env bash
# Nightly Postgres backup, run on the VM by the systemd timer installed with
# deploy/scripts/install-postgres-backup.sh (03:00 America/Mexico_City).
#
# 1. pg_dump -Fc from the running postgres container into a temp file on the DATA volume
#    (never the 12 GB root disk). A file, not a pipe: the VM role can only PUT backups, it
#    cannot delete them, so a truncated dump must be caught BEFORE it is uploaded.
# 2. Verify the dump (pg_restore --list reads its table of contents).
# 3. Upload to s3://<bucket>/backups/daily/YYYY-MM-DD.dump, and on day 1 of the month
#    also to backups/monthly/YYYY-MM.dump, with the instance role (no keys) through a
#    pinned amazon/aws-cli container.
# 4. Write the success to $BACKUP_DIR/last-success: line 1 the UTC timestamp, line 2
#    "size_bytes=N" (the dump size). The API reads it read-only: it emails the owner
#    "backup completed" for each new success and warns when it is older than 36 hours.
#
# Any failure exits non-zero (the systemd unit fails and the journal has the reason) and
# leaves last-success untouched. The temp dump is always removed.
#
# Settings come from deploy/.env.prod (S3_BUCKET, S3_REGION, COMPOSE_PROJECT_NAME) with
# optional overrides: BACKUP_ENV_FILE, BACKUP_DIR, BACKUP_TMP_DIR, BACKUP_AWS_CLI_IMAGE.
set -euo pipefail
umask 077

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
ENV_FILE="${BACKUP_ENV_FILE:-$REPO/deploy/.env.prod}"
COMPOSE_FILE="$REPO/deploy/docker-compose.prod.yml"
BACKUP_DIR="${BACKUP_DIR:-/srv/finances/backup}"
TMP_DIR="${BACKUP_TMP_DIR:-/srv/finances/backup-tmp}"
# Pinned by tag and by the multi-arch manifest digest (arm64 VM).
AWS_CLI_IMAGE="${BACKUP_AWS_CLI_IMAGE:-amazon/aws-cli:2.37.12@sha256:ce44b800602264c12298bbf3399055a4cefdb9ed4a098abe564afe26e7071bce}"
TZ_NAME="America/Mexico_City"

log() { printf '%s backup: %s\n' "$(date -u +%FT%TZ)" "$*"; }
die() { log "ERROR: $*" >&2; exit 1; }

env_value() { # env_value KEY -> last value of KEY in the env file, without quotes
    sed -n "s/^$1=//p" "$ENV_FILE" | tail -n 1 | sed -e 's/^"//' -e 's/"$//' -e "s/^'//" -e "s/'\$//"
}

dump=""
cleanup() {
    rc=$?
    [ -n "$dump" ] && rm -f "$dump"
    if [ "$rc" -ne 0 ]; then
        log "FAILED with exit code $rc; last-success was not updated" >&2
    fi
}
trap cleanup EXIT

[ -r "$ENV_FILE" ] || die "cannot read $ENV_FILE"
BUCKET="$(env_value S3_BUCKET)"
REGION="$(env_value S3_REGION)"
REGION="${REGION:-us-east-2}"
# The development default is the MinIO bucket: refuse it, backups only go to AWS S3.
case "$BUCKET" in
    "" | finances-invoices) die "S3_BUCKET in $ENV_FILE is not the AWS bucket (terraform output data_bucket_name)" ;;
esac

mkdir -p "$BACKUP_DIR" "$TMP_DIR"
chmod 755 "$BACKUP_DIR"
chmod 700 "$TMP_DIR"

compose() { docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"; }
cid="$(compose ps -q postgres)"
[ -n "$cid" ] || die "the postgres container is not running"

today="$(TZ=$TZ_NAME date +%F)"   # YYYY-MM-DD in Mexico City
month="${today%-*}"               # YYYY-MM
day="${today##*-}"                # DD

dump="$(mktemp "$TMP_DIR/pg-$today.XXXXXX.dump")"
name="$(basename "$dump")"

log "dumping the database from the postgres container"
docker exec "$cid" sh -c 'exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc' > "$dump"
size="$(stat -c %s "$dump")"
[ "$size" -gt 1024 ] || die "the dump is suspiciously small ($size bytes)"

log "verifying the dump ($size bytes)"
docker exec -i "$cid" pg_restore --list < "$dump" > /dev/null || die "pg_restore cannot read the dump"

# The instance role reaches IMDSv2 from the default bridge network (hop limit 2).
aws_cp() { # aws_cp <destination key>
    docker run --rm --cap-drop ALL --security-opt no-new-privileges \
        -e AWS_DEFAULT_REGION="$REGION" \
        -v "$TMP_DIR:/data:ro" \
        "$AWS_CLI_IMAGE" s3 cp --only-show-errors --no-progress "/data/$name" "s3://$BUCKET/$1"
}

log "uploading backups/daily/$today.dump"
aws_cp "backups/daily/$today.dump"
if [ "$day" = "01" ]; then
    log "day 1: uploading backups/monthly/$month.dump"
    aws_cp "backups/monthly/$month.dump"
fi

# Atomic replace (write + rename on the same directory): the API never reads half a file.
status="$BACKUP_DIR/last-success"
printf '%s\nsize_bytes=%s\n' "$(date -u +%FT%TZ)" "$size" > "$status.tmp"
chmod 644 "$status.tmp"
mv -f "$status.tmp" "$status"
log "OK: backups/daily/$today.dump ($size bytes), status written to $status"
