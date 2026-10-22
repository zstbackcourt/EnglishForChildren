#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
version=2026.10.3
# macOS uses Homebrew's packaged binary; the upstream 2026.10.3 tgz failed
# codesign verification on the validation machine despite a matching asset digest.
if [[ "$(uname -s)" == Darwin ]]; then
  if ! command -v brew >/dev/null 2>&1; then
    echo '请先按 https://brew.sh 安装 Homebrew，再重试本脚本。' >&2
    exit 1
  fi
  brew install cloudflared
  cloud_path="$(brew --prefix)/bin/cloudflared"
  codesign --verify --strict "$cloud_path"
  "$cloud_path" --version
  exit 0
fi
case "$(uname -s)-$(uname -m)" in
  Linux-aarch64|Linux-arm64) asset=cloudflared-linux-arm64; digest=89297ebfb9e8c99c0dc013a1140601453cf3798d8306c5591651f4a3f5210be8 ;;
  Linux-x86_64) asset=cloudflared-linux-amd64; digest=053b80b0a23031a038fdbf3c073f59f82b8f1f152aa67971d7383ffafc0dbab0 ;;
  *) echo '此脚本支持 macOS/Linux 的 arm64 和 amd64。' >&2; exit 1 ;;
esac
mkdir -p .tools/cloudflared
stage=$(mktemp -d .tools/cloudflared/install.XXXXXX)
trap 'rm -rf -- "$stage"' EXIT
curl -fL --connect-timeout 20 --max-time 300 --retry 2 \
  "https://github.com/cloudflare/cloudflared/releases/download/$version/$asset" -o "$stage/$asset"
python3 - "$stage/$asset" "$digest" <<'PY'
import hashlib, pathlib, sys
actual = hashlib.sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest()
if actual != sys.argv[2]:
    sys.exit('下载文件校验失败，不安装。')
print('SHA256 校验通过。')
PY
case "$asset" in
  *.tgz) tar -xzf "$stage/$asset" -C "$stage" ;;
  *) mv "$stage/$asset" "$stage/cloudflared" ;;
esac
chmod +x "$stage/cloudflared"
mv "$stage/cloudflared" .tools/cloudflared/cloudflared
.tools/cloudflared/cloudflared --version
