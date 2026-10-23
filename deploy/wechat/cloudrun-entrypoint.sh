#!/bin/sh
set -eu
# Cloud containers are ephemeral: never silently fall back to local JSON saves.
export WECHAT_STORAGE=mysql
: "${WECHAT_APP_ID:?Configure WECHAT_APP_ID}"
: "${WECHAT_APP_SECRET:?Configure WECHAT_APP_SECRET}"
: "${WECHAT_SESSION_KEY:?Configure WECHAT_SESSION_KEY}"
: "${MYSQL_ADDRESS:?Configure MYSQL_ADDRESS}"
: "${MYSQL_USERNAME:?Configure MYSQL_USERNAME}"
: "${MYSQL_PASSWORD:?Configure MYSQL_PASSWORD}"
: "${MYSQL_DATABASE:?Configure an existing MYSQL_DATABASE}"
exec /usr/local/bin/wechat-server -root /app -addr "0.0.0.0:${PORT:-8080}"
