#!/bin/sh
set -eu
# Cloud containers are ephemeral: never silently fall back to local JSON saves.
export WECHAT_STORAGE=mysql
missing=''
for name in WECHAT_APP_ID WECHAT_APP_SECRET WECHAT_SESSION_KEY MYSQL_ADDRESS MYSQL_USERNAME MYSQL_PASSWORD MYSQL_DATABASE; do
  if [ -z "$(printenv "$name" 2>/dev/null || true)" ]; then
    missing="$missing $name"
  fi
done
if [ -n "$missing" ]; then
  printf 'Missing required cloud environment variables:%s\n' "$missing" >&2
  printf 'Configure these names in the CloudRun service. Values are never logged.\n' >&2
  exit 64
fi
if [ "${1:-}" = '--check-config' ]; then
  printf 'Required environment variables are present. Database connectivity is not checked.\n'
  exit 0
fi
export WECHAT_RELEASE_ID="$(cat /app/deploy/wechat/release-id.txt)"
exec /usr/local/bin/wechat-server -root /app -addr "0.0.0.0:${PORT:-8080}"
