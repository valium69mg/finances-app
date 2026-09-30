#!/usr/bin/env sh
# Frees the disk that Docker builds leave behind on the VM: the build cache and images
# that no container uses and that are older than a week. Every deploy rebuilds the api and
# frontend images on the VM, so without this the data volume grows by a few hundred MB per
# deploy for years.
#
# It never touches volumes (postgres-data and minio-data hold the real data) and never
# removes an image that a container, running or stopped, still uses.
#
# Run weekly by the systemd timer installed with deploy/scripts/install-docker-prune.sh.
set -eu

UNTIL="${DOCKER_PRUNE_UNTIL:-168h}"

df_used() { df -h /srv/finances | tail -1 | awk '{print $3 " used of " $2 " (" $5 ")"}'; }

echo "before: $(df_used)"
docker builder prune --force --filter "until=${UNTIL}"
docker image prune --all --force --filter "until=${UNTIL}"
echo "after: $(df_used)"
