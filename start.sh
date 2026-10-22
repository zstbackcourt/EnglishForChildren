#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")"
if command -v go >/dev/null 2>&1; then
  go_cmd=go
elif [[ -x .tools/go/bin/go ]]; then
  go_cmd=.tools/go/bin/go
else
  echo '未找到 Go。请安装 Go 1.22 或更新版本：https://go.dev/dl/' >&2
  exit 1
fi
export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/english-adventure-go-cache}"
mkdir -p bin
"$go_cmd" build -o bin/english-adventure ./cmd/server
exec bin/english-adventure "$@"
