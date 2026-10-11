#!/usr/bin/env bash
# Restores a Postgres backup from the S3 bucket into a TARGET database. Run it from the
# owner's laptop with ADMIN AWS credentials (the VM role cannot read backups). It never
# touches the production database unless you point TARGET_DATABASE_URL at it.
#
# Usage:
#   aws s3 ls s3://$BUCKET/backups/daily/          # pick a dump
#   BUCKET=<bucket> TARGET_DATABASE_URL=postgres://user:pass@localhost:5442/finances_restore \
#       deploy/scripts/restore-postgres.sh backups/daily/2026-10-10.dump
#
# Options (environment):
#   BUCKET               the bucket name (terraform output data_bucket_name). Required.
#   TARGET_DATABASE_URL  database to restore INTO; it must already exist (CREATE DATABASE).
#                        Required. Its contents are replaced (--clean --if-exists).
#   AWS_REGION           default us-east-2.
#   RESTORE_YES=1        skip the confirmation prompt.
#
# pg_restore runs locally when installed, otherwise in a postgres container on the host
# network (needs docker). Use a client at least as new as the server that made the dump.
set -euo pipefail
umask 077

key="${1:-}"
[ -n "$key" ] || { echo "usage: $0 <object key, e.g. backups/daily/2026-10-10.dump>" >&2; exit 2; }
case "$key" in backups/*.dump) ;; *) echo "the key must look like backups/daily/YYYY-MM-DD.dump" >&2; exit 2 ;; esac
: "${BUCKET:?set BUCKET to the bucket name (terraform output data_bucket_name)}"
: "${TARGET_DATABASE_URL:?set TARGET_DATABASE_URL to the database to restore into}"
region="${AWS_REGION:-us-east-2}"
PG_IMAGE="${RESTORE_PG_IMAGE:-postgres:18.6}"

command -v aws > /dev/null || { echo "the aws CLI is required (admin credentials)" >&2; exit 1; }

# Show where the data is going and ask, so a wrong URL is caught before it is replaced.
target_label="$(printf '%s' "$TARGET_DATABASE_URL" | sed -E 's#^[a-z]+://[^@]*@##; s#\?.*$##')"
echo "restore s3://$BUCKET/$key -> $target_label (existing objects are dropped)"
if [ "${RESTORE_YES:-}" != "1" ]; then
    printf 'type the target database name to continue: '
    read -r answer
    [ "$answer" = "${target_label##*/}" ] || { echo "aborted" >&2; exit 1; }
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
file="$work/restore.dump"

aws s3 cp --region "$region" --only-show-errors "s3://$BUCKET/$key" "$file"
echo "downloaded $(stat -c %s "$file") bytes"

if command -v pg_restore > /dev/null; then
    run_restore() { pg_restore --clean --if-exists --no-owner --no-privileges -d "$TARGET_DATABASE_URL" "$file"; }
    pg_restore --list "$file" > /dev/null
else
    echo "pg_restore not found: using $PG_IMAGE in docker"
    run_restore() {
        docker run --rm --network host -v "$work:/work:ro" "$PG_IMAGE" \
            pg_restore --clean --if-exists --no-owner --no-privileges \
            -d "$TARGET_DATABASE_URL" /work/restore.dump
    }
    docker run --rm -v "$work:/work:ro" "$PG_IMAGE" pg_restore --list /work/restore.dump > /dev/null
fi

run_restore
echo "restore finished into $target_label"
