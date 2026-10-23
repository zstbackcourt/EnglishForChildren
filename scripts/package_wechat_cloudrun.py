#!/usr/bin/env python3
"""Build a source-only CloudRun zip; assets are fetched at pinned commits by Docker."""
import argparse
import json
from pathlib import Path
import zipfile


def package(root, output):
    root = root.resolve()
    selected = ['go.mod', 'go.sum', 'Dockerfile', 'deploy/wechat/cloudrun-entrypoint.sh', 'content/book1.json']
    for prefix in ('cmd/wechat-server', 'internal/game'):
        selected.extend(str(p.relative_to(root)) for p in sorted((root / prefix).glob('*.go')))
    for rel in selected:
        p = root / rel
        for candidate in (p, *p.parents):
            if candidate == root:
                break
            if candidate.is_symlink():
                raise ValueError('拒绝符号链接 symlink: ' + rel)
        if not p.is_file():
            raise ValueError('缺少构建源文件: ' + rel)
    if not any(p.startswith('cmd/wechat-server/') for p in selected):
        raise ValueError('缺少 wechat-server 入口')
    output.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as archive:
        for rel in sorted(selected):
            info = zipfile.ZipInfo(rel, (2026, 10, 23, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o100644 << 16
            archive.writestr(info, (root / rel).read_bytes())
    return {'archive': str(output.resolve()), 'files': len(selected), 'bytes': output.stat().st_size,
            'deployed': False}


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parent.parent)
    parser.add_argument('--output', type=Path, default=Path('tmp/wechat-cloudrun-source.zip'))
    args = parser.parse_args()
    try:
        print(json.dumps(package(args.root, args.output), ensure_ascii=False))
    except (OSError, ValueError) as error:
        parser.exit(1, str(error) + '\n')
