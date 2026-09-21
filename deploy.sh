#!/usr/bin/env bash
# Copy the source to a Docker host over SSH and rebuild the container there.
# Configure DEPLOY_HOST (ssh alias) and DEPLOY_DIR (relative to remote $HOME) in .env or the environment.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] && set -a && . ./.env && set +a
HOST="${DEPLOY_HOST:?set DEPLOY_HOST in .env}"
REMOTE_DIR="${DEPLOY_DIR:-Projects/splitfriends}"
APP_VERSION="${APP_VERSION:-$(date -u +%Y%m%d-%H%M)}"
PORT="${BIND_PORT:-8080}"

echo "==> copying source to $HOST:~/$REMOTE_DIR"
COPYFILE_DISABLE=1 tar --no-xattrs -czf - \
  --exclude ./.git --exclude ./data --exclude './*.db*' \
  --exclude ./web/node_modules --exclude ./web/dist --exclude .DS_Store --exclude '._*' \
  . | ssh "$HOST" "mkdir -p '$REMOTE_DIR' && tar -xzf - -C '$REMOTE_DIR'"

echo "==> building and starting (APP_VERSION=$APP_VERSION)"
ssh "$HOST" "cd '$REMOTE_DIR' && APP_VERSION='$APP_VERSION' docker compose up -d --build --remove-orphans && docker compose ps && docker image prune -f >/dev/null"

echo "==> health"
ssh "$HOST" "curl -fsS --max-time 5 http://127.0.0.1:$PORT/healthz && echo"
