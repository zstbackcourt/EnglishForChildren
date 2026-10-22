#!/usr/bin/env python3
"""Run one password-protected local game and a temporary public tunnel."""
import base64
import fcntl
import json
import os
from pathlib import Path
import queue
import re
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import threading
import time
import urllib.request
import urllib.error

ROOT = Path(__file__).resolve().parent.parent
RUNTIME = ROOT / 'data' / 'share'
CONTROL = str(RUNTIME / 'control.sock')


def load_password(path):
    if path.exists():
        password = path.read_text().strip()
        if len(password) < 16:
            raise ValueError('家庭密码文件为空或过短；请设置至少 16 个字符，或删除文件后重新生成。')
        os.chmod(str(path), 0o600)
        return password
    password = secrets.token_urlsafe(18)
    fd = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as stream:
        stream.write(password + '\n')
    return password


def find_url(line):
    match = re.search(r'https://[a-z0-9-]+\.trycloudflare\.com(?=[\s|/]|$)', line)
    return match.group(0) if match else None


def request_control(action):
    with socket.socket(socket.AF_UNIX) as client:
        client.settimeout(3)
        try:
            client.connect(CONTROL)
            client.sendall(action.encode())
            print(client.recv(8192).decode())
        except (OSError, socket.timeout):
            print('外网分享未运行。')
            return 1
    return 0


def install_stop_signals(stopped):
    names = ('SIGTERM', 'SIGINT', 'SIGHUP')
    for name in names:
        if hasattr(signal, name):
            signal.signal(getattr(signal, name), lambda *_: stopped.set())


def terminate(child):
    if child.poll() is not None:
        return
    try:
        os.killpg(child.pid, signal.SIGTERM)
    except ProcessLookupError:
        pass
    try:
        child.wait(timeout=5)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(child.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        child.wait()


def main():
    if len(sys.argv) > 1 and sys.argv[1] in ('status', 'stop'):
        return request_control(sys.argv[1])
    os.umask(0o077)
    RUNTIME.mkdir(parents=True, exist_ok=True)
    with open(RUNTIME / 'lock', 'w') as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print('外网分享已经运行。使用 ./share.sh status 查看。', file=sys.stderr)
            return 1
        return serve()


def serve():
    port = int(os.environ.get('SHARE_PORT', '8080'))
    if not 1 <= port <= 65535:
        raise ValueError('SHARE_PORT 必须在 1–65535 之间')
    with socket.socket() as probe:
        probe.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            probe.bind(('127.0.0.1', port))
        except OSError:
            print('端口 %s 已占用，请先停止旧游戏服务；不能同时写同一个家庭存档。' % port, file=sys.stderr)
            return 1
    cloud = os.environ.get('CLOUDFLARED') or shutil.which('cloudflared')
    if not cloud:
        candidate = ROOT / '.tools' / 'cloudflared' / 'cloudflared'
        if candidate.is_file():
            cloud = str(candidate)
    if not cloud:
        print('请先执行 bash scripts/install_cloudflared.sh', file=sys.stderr)
        return 1
    if sys.platform == 'darwin':
        check = subprocess.run(['codesign', '--verify', '--strict', cloud],
                               stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, universal_newlines=True)
        if check.returncode:
            raise RuntimeError('cloudflared 的 macOS 签名校验失败；请从官方重新安装有效版本。未开启外网访问。')
    password = load_password(RUNTIME / 'password.txt')
    children = []
    stopped = threading.Event()
    install_stop_signals(stopped)
    info = {'status': 'starting', 'url': None, 'localUrl': 'http://127.0.0.1:%s' % port,
            'username': 'family', 'passwordFile': str(RUNTIME / 'password.txt')}
    control = socket.socket(socket.AF_UNIX)
    if os.path.exists(CONTROL):
        os.unlink(CONTROL)
    control.bind(CONTROL)
    control.listen(2)
    control.settimeout(0.2)
    logs = []
    try:
        def log_file(name):
            stream = open(RUNTIME / name, 'w')
            logs.append(stream)
            return stream

        env = dict(os.environ, GAME_PASSWORD=password)
        server = subprocess.Popen([str(ROOT / 'bin' / 'english-adventure'), '-root', str(ROOT),
                                   '-addr', '127.0.0.1:%s' % port], env=env,
                                  stdout=log_file('server.log'), stderr=subprocess.STDOUT,
                                  start_new_session=True)
        children.append(server)
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(60):
            if stopped.is_set():
                return 0
            if server.poll() is not None:
                raise RuntimeError('游戏启动失败，请查看 data/share/server.log')
            try:
                with opener.open(info['localUrl'] + '/api/health', timeout=1) as response:
                    if response.status == 200:
                        break
            except (OSError, urllib.error.URLError):
                stopped.wait(0.2)
        else:
            raise RuntimeError('游戏启动超时')
        try:
            opener.open(info['localUrl'] + '/api/players', timeout=3)
            raise RuntimeError('拒绝开启隧道：本地角色接口没有密码保护')
        except urllib.error.HTTPError as error:
            if error.code != 401:
                raise
        auth = base64.b64encode(('family:' + password).encode()).decode()
        request = urllib.request.Request(info['localUrl'] + '/api/players',
                                         headers={'Authorization': 'Basic ' + auth})
        with opener.open(request, timeout=3) as response:
            if response.status != 200:
                raise RuntimeError('家庭密码验证失败')
        # Explicit empty config isolates Quick Tunnel from existing account configs.
        config = RUNTIME / 'tunnel.yml'
        config.write_text('{}\n')
        tunnel = subprocess.Popen([cloud, 'tunnel', '--config', str(config), '--no-autoupdate',
                                   '--url', info['localUrl'], '--protocol', 'http2'],
                                  stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                                  universal_newlines=True, start_new_session=True)
        children.append(tunnel)
        tunnel_log = log_file('tunnel.log')
        lines = queue.Queue()
        def read_output():
            for line in tunnel.stdout:
                tunnel_log.write(line)
                tunnel_log.flush()
                lines.put(line)
        reader = threading.Thread(target=read_output, daemon=True)
        reader.start()
        if sys.platform == 'darwin' and shutil.which('caffeinate'):
            children.append(subprocess.Popen(['caffeinate', '-i', '-w', str(os.getpid())],
                                             start_new_session=True))
        print('游戏已启用家庭密码，正在建立外网隧道……', flush=True)
        started = time.monotonic()
        while not stopped.is_set():
            if server.poll() is not None or tunnel.poll() is not None:
                raise RuntimeError('游戏或隧道退出（game=%s, tunnel=%s），请查看 data/share 下的日志' %
                                   (server.poll(), tunnel.poll()))
            while not lines.empty():
                line = lines.get_nowait()
                url = find_url(line)
                if url and not info['url']:
                    info.update(url=url, status='connecting')
                    print('分享地址：' + url + '\n用户名：family\n密码文件：' + info['passwordFile'] +
                          '\n请等待隧道连接，并从外网检查后分享。停止：./share.sh stop', flush=True)
                if 'Registered tunnel connection' in line:
                    info['status'] = 'running'
                    print('隧道已连接。', flush=True)
            if info['status'] != 'running' and time.monotonic() - started > 120:
                raise RuntimeError('120 秒内未接通隧道，请查看 tunnel.log 检查网络')
            (RUNTIME / 'status.json').write_text(json.dumps(info, ensure_ascii=False, indent=2) + '\n')
            try:
                client, _ = control.accept()
            except socket.timeout:
                continue
            with client:
                client.settimeout(1)
                try:
                    action = client.recv(32).decode()
                    if action == 'stop':
                        client.sendall('正在停止隧道和游戏；家庭存档保留。'.encode())
                        stopped.set()
                    else:
                        client.sendall(json.dumps(info, ensure_ascii=False, indent=2).encode())
                except (OSError, UnicodeError):
                    pass
        return 0
    finally:
        for child in reversed(children):
            terminate(child)
        if 'reader' in locals():
            reader.join(timeout=2)
        for stream in logs:
            stream.close()
        control.close()
        if os.path.exists(CONTROL):
            os.unlink(CONTROL)
        info['status'] = 'stopped'
        (RUNTIME / 'status.json').write_text(json.dumps(info, ensure_ascii=False, indent=2) + '\n')
        print('外网分享已停止，存档仍保存在 data/save.json。', flush=True)


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, RuntimeError, OSError) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
