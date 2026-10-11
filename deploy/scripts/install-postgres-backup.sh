#!/usr/bin/env sh
# Installs the nightly Postgres backup as a systemd timer on the VM (03:00
# America/Mexico_City). Safe to run again: it rewrites the two units and re-enables the
# timer. Needs root. Same pattern as install-docker-prune.sh.
#
# Usage (from the repo checkout on the VM): sudo deploy/scripts/install-postgres-backup.sh
# Run the first backup by hand:               sudo systemctl start finances-postgres-backup.service
# Follow it:                                  journalctl -u finances-postgres-backup.service -f
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "run as root: sudo $0" >&2
    exit 1
fi

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$REPO/deploy/scripts/backup-postgres.sh"
chmod 755 "$SCRIPT"

# /srv/finances/backup holds only the status file (mounted read-only into the API);
# /srv/finances/backup-tmp holds the dump while it is being made and is private to root.
mkdir -p /srv/finances/backup /srv/finances/backup-tmp
chmod 755 /srv/finances/backup
chmod 700 /srv/finances/backup-tmp

cat > /etc/systemd/system/finances-postgres-backup.service <<EOF2
[Unit]
Description=Back up Postgres to S3
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=$SCRIPT
TimeoutStartSec=1h
EOF2

cat > /etc/systemd/system/finances-postgres-backup.timer <<'EOF2'
[Unit]
Description=Nightly Postgres backup to S3

[Timer]
OnCalendar=*-*-* 03:00:00 America/Mexico_City
RandomizedDelaySec=2m
Persistent=true

[Install]
WantedBy=timers.target
EOF2

systemctl daemon-reload
systemctl enable --now finances-postgres-backup.timer
systemctl list-timers finances-postgres-backup.timer --no-pager
