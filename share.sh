#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
if [[ "${1:-}" == status || "${1:-}" == stop ]]; then
  exec python3 scripts/share.py "$1"
fi
if [[ $# != 0 ]]; then
  echo '使用：./share.sh [status|stop]' >&2
  exit 1
fi
if command -v go >/dev/null 2>&1; then
  go_cmd=go
elif [[ -x .tools/go/bin/go ]]; then
  go_cmd=.tools/go/bin/go
else
  echo '需要 Go 1.22+，请先安装 Go。' >&2
  exit 1
fi
export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/english-adventure-go-cache}"
mkdir -p bin
"$go_cmd" build -o bin/english-adventure ./cmd/server
exec python3 scripts/share.py
