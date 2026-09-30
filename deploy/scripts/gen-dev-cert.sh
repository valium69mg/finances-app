#!/usr/bin/env sh
# Generates a SELF-SIGNED certificate for local smoke tests of the production stack.
# Never use it in production: there, use a Cloudflare Origin CA certificate (deploy/README.md).
#
# Usage: deploy/scripts/gen-dev-cert.sh [output-dir] [hostname]
#   output-dir defaults to deploy/certs, hostname to localhost.
# Writes origin.pem and origin.key, the names docker-compose.prod.yml expects.
set -eu

DIR="${1:-$(dirname "$0")/../certs}"
HOST="${2:-localhost}"

mkdir -p "$DIR"
openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
    -subj "/CN=$HOST" \
    -addext "subjectAltName=DNS:$HOST" \
    -keyout "$DIR/origin.key" -out "$DIR/origin.pem" 2>/dev/null

# The nginx container runs as a non-root user (uid 101), so the key must be readable by
# it. Acceptable for a throwaway dev key; a real key stays 0400 owned by uid 101.
chmod 644 "$DIR/origin.pem" "$DIR/origin.key"
echo "wrote $DIR/origin.pem and $DIR/origin.key (self-signed, CN=$HOST, valid 30 days)"
