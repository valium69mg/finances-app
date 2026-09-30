#!/usr/bin/env sh
# Installs the weekly Docker cleanup as a systemd timer on the VM. Safe to run again: it
# rewrites the two units and re-enables the timer. Needs root.
#
# Usage (from the repo checkout on the VM): sudo deploy/scripts/install-docker-prune.sh
set -eu

if [ "$(id -u)" -ne 0 ]; then
    echo "run as root: sudo $0" >&2
    exit 1
fi

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT="$REPO/deploy/scripts/docker-prune.sh"
chmod 755 "$SCRIPT"

cat > /etc/systemd/system/finances-docker-prune.service <<EOF
[Unit]
Description=Free Docker build cache and unused images
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=$SCRIPT
EOF

cat > /etc/systemd/system/finances-docker-prune.timer <<'EOF'
[Unit]
Description=Weekly Docker cleanup

[Timer]
OnCalendar=Sun *-*-* 04:00:00
RandomizedDelaySec=30m
Persistent=true

[Install]
WantedBy=timers.target
EOF

systemctl daemon-reload
systemctl enable --now finances-docker-prune.timer
systemctl list-timers finances-docker-prune.timer --no-pager
