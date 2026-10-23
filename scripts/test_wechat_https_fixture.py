import contextlib
import http.client
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import io
import json
from pathlib import Path
import socket
import ssl
import subprocess
import tempfile
import threading
import unittest
from urllib.parse import urlencode


class WeChatHTTPSFixtureTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        path = Path(__file__).parent / 'fixtures' / 'wechat_https.py'
        assert path.is_file(), 'HTTPS fixture must exist'
        spec = importlib.util.spec_from_file_location('wechat_https', path)
        cls.fixture = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.fixture)
        cls.temp = tempfile.TemporaryDirectory()
        cls.root = Path(cls.temp.name)
        commands = [
            ['req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-keyout', 'ca.key', '-out', 'ca.pem', '-subj', '/CN=Disposable WeChat Test CA', '-days', '1'],
            ['req', '-newkey', 'rsa:2048', '-nodes', '-keyout', 'server.key', '-out', 'server.csr', '-subj', '/CN=api.weixin.qq.com'],
            ['x509', '-req', '-in', 'server.csr', '-CA', 'ca.pem', '-CAkey', 'ca.key', '-CAcreateserial', '-out', 'server.pem', '-days', '1', '-extfile', 'server.ext'],
        ]
        (cls.root / 'server.ext').write_text('subjectAltName=DNS:api.weixin.qq.com\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n')
        for command in commands:
            subprocess.run(['openssl'] + command, cwd=cls.root, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def test_https_requires_test_ca_and_exact_hostname_without_logging_requests(self):
        output = io.StringIO()
        with contextlib.redirect_stderr(output), contextlib.redirect_stdout(output):
            server = self.fixture.make_server(str(self.root / 'server.pem'), str(self.root / 'server.key'), ('127.0.0.1', 0))
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                def connect(context, hostname='api.weixin.qq.com'):
                    raw = socket.create_connection(server.server_address, timeout=3)
                    try:
                        return context.wrap_socket(raw, server_hostname=hostname)
                    except Exception:
                        raw.close()
                        raise
                with self.assertRaises(ssl.SSLCertVerificationError):
                    connect(ssl.create_default_context())
                trusted = ssl.create_default_context(cafile=str(self.root / 'ca.pem'))
                with self.assertRaises(ssl.SSLCertVerificationError):
                    connect(trusted, 'wrong-hostname.test')
                query = dict(appid='wx0000000000000000', secret='ci-test-secret', js_code='ci-test-code', grant_type='authorization_code')
                for valid in [True, False]:
                    if not valid:
                        query['secret'] = 'incorrect-do-not-log'
                    with connect(trusted) as connection:
                        connection.sendall(('GET /sns/jscode2session?' + urlencode(query) + ' HTTP/1.1\r\nHost: api.weixin.qq.com\r\nConnection: close\r\n\r\n').encode())
                        response = http.client.HTTPResponse(connection)
                        response.begin()
                        body = json.loads(response.read())
                        self.assertEqual(response.status, 200 if valid else 400)
                        self.assertEqual('openid' in body, valid)
            finally:
                server.shutdown()
                server.server_close()
                thread.join(timeout=3)
        self.assertEqual(output.getvalue(), '')

    def test_network_probe_commands_check_health_authentication_and_partial_audio(self):
        state = {'releaseId': 'test-release', 'players': 401, 'audio': 206, 'bytes': b'ID3x'}
        ranges = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_GET(self):
                if self.path == '/healthz':
                    status, body = 200, json.dumps({'status': 'ok', 'releaseId': state['releaseId']}).encode()
                elif self.path == '/api/players':
                    status, body = state['players'], b'{}'
                elif self.path == '/media/audio/1':
                    ranges.append(self.headers.get('Range'))
                    status, body = state['audio'], state['bytes']
                else:
                    status, body = 404, b'{}'
                self.send_response(status)
                self.send_header('Content-Length', str(len(body)))
                self.end_headers()
                self.wfile.write(body)
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        fixture = Path(__file__).parent / 'fixtures' / 'wechat_https.py'
        base = 'http://127.0.0.1:' + str(server.server_port)
        def run(command):
            return subprocess.run(['python3', str(fixture), command, base, 'test-release'], capture_output=True, text=True, timeout=5)
        try:
            for command in ['ready', 'checks']:
                result = run(command)
                self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(ranges, ['bytes=0-3'])
            for key, bad in [('releaseId', 'old-release'), ('players', 200), ('audio', 200), ('bytes', b'bad'), ('bytes', b'too-long')]:
                previous = state[key]
                state[key] = bad
                result = run('checks')
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(result.stderr, 'WeChat HTTPS fixture check failed.\n')
                state[key] = previous
        finally:
            server.shutdown()
            server.server_close()
            thread.join(timeout=3)

    def test_session_probe_uses_token_only_for_authenticated_players_request(self):
        calls = []
        def request(base, path, method='GET', payload=None, token=None):
            calls.append((path, method, payload, token))
            if path == '/api/session':
                return 200, {'token': 'disposable-token', 'expiresAt': 'test-only'}
            return 200, []
        self.fixture.verify_session('http://127.0.0.1:8080', True, request)
        self.assertEqual(calls, [('/api/session', 'POST', {'code': 'ci-test-code'}, None), ('/api/players', 'GET', None, 'disposable-token')])

    def test_control_requires_unknown_authority_and_never_accepts_token(self):
        expected = {'error': 'test-only', 'reason': 'unavailable', 'diagnostic': 'transport_tls_unknown_authority'}
        self.fixture.verify_session('http://127.0.0.1:8080', False, lambda *args, **kwargs: (401, expected))
        for body in [dict(expected, token='must-not-exist'), dict(expected, diagnostic='transport_dns'), {'reason': 'unavailable'}]:
            with self.assertRaises(RuntimeError):
                self.fixture.verify_session('http://127.0.0.1:8080', False, lambda *args, **kwargs: (401, body))

    def test_trusted_probe_rejects_missing_token_or_protected_players(self):
        with self.assertRaises(RuntimeError):
            self.fixture.verify_session('http://127.0.0.1:8080', True, lambda *args, **kwargs: (200, {}))
        def denied(base, path, **kwargs):
            return (200, {'token': 'disposable-token'}) if path == '/api/session' else (401, {})
        with self.assertRaises(RuntimeError):
            self.fixture.verify_session('http://127.0.0.1:8080', True, denied)


if __name__ == '__main__':
    unittest.main()
