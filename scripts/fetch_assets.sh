#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
source_ref=635746ac5f696b193311bc1a7e8ffac3b252eb75
source_dir='新概念英语第1册美音（MP3+LRC）'
if [[ ! -d New-Concept-English ]]; then
  git clone --filter=blob:none --no-checkout https://github.com/zstbackcourt/New-Concept-English.git New-Concept-English
  git -C New-Concept-English sparse-checkout set --no-cone "/$source_dir/"
  git -C New-Concept-English checkout "$source_ref"
fi
if [[ "$(git -C New-Concept-English rev-parse HEAD)" != "$source_ref" ]]; then
  echo '素材仓库版本不匹配。请保留现有目录，另在全新项目目录中运行本脚本。' >&2
  exit 1
fi
python3 scripts/test_content.py
