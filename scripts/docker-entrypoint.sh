#!/bin/sh
set -eu
# A newly mounted persistent disk replaces the image directory and its owner.
if [ "$(id -u)" = 0 ]; then
  mkdir -p /app/data
  chown game:game /app/data
  if [ -f /app/data/save.json ]; then
    chown game:game /app/data/save.json
  fi
  exec su-exec game:game game "$@"
fi
exec game "$@"
