#!/usr/bin/env python3
"""Disposable HTTPS WeChat endpoint and quiet production-image session probe."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import ssl
import sys
from urllib.error import HTTPError
from urllib.parse import parse_qs, urlsplit
from urllib.request import build_opener, ProxyHandler, Request


class WeChatHandler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass  # Never log the request target: WeChat query strings contain credentials.

    def do_GET(self):
        target = urlsplit(self.path)
        valid = target.path == '/sns/jscode2session' and parse_qs(target.query, keep_blank_values=True) == {
            'appid': ['wx0000000000000000'], 'secret': ['ci-test-secret'],
            'js_code': ['ci-test-code'], 'grant_type': ['authorization_code'],
        }
        body = json.dumps({'openid': 'ci-only-openid', 'session_key': 'ci-only-session-key'} if valid else {
            'errcode': 40029, 'errmsg': 'invalid test request',
        }).encode()
        self.send_response(200 if valid else 400)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)


class QuietServer(ThreadingHTTPServer):
    def handle_error(self, request, client_address):
        pass  # Do not emit request or client details even on a failed connection.


def make_server(cert, key, address=('0.0.0.0', 443)):
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    context.load_cert_chain(cert, key)
    server = QuietServer(address, WeChatHandler)
    server.socket = context.wrap_socket(server.socket, server_side=True)
    return server


def request_json(base, path, method='GET', payload=None, token=None):
    headers = {'Content-Type': 'application/json'}
    if token is not None:
        headers['Authorization'] = 'Bearer ' + token
    request = Request(base + path, data=json.dumps(payload).encode() if payload is not None else None,
                      headers=headers, method=method)
    try:
        response = build_opener(ProxyHandler({})).open(request, timeout=10)
    except HTTPError as error:
        response = error
    with response:
        return response.status, json.loads(response.read(65537))


def verify_session(base, trusted, request=request_json):
    status, body = request(base, '/api/session', method='POST', payload={'code': 'ci-test-code'})
    if not trusted:
        if status != 401 or not isinstance(body, dict) or body.get('reason') != 'unavailable' or body.get('diagnostic') != 'transport_tls_unknown_authority' or 'token' in body:
            raise RuntimeError('Untrusted CA control failed')
        return
    if status != 200 or not isinstance(body, dict) or not isinstance(body.get('token'), str) or not body['token']:
        raise RuntimeError('Trusted CA session failed')
    status, players = request(base, '/api/players', token=body['token'])
    if status != 200 or not isinstance(players, list):
        raise RuntimeError('Authenticated players failed')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    serve = commands.add_parser('serve')
    serve.add_argument('cert')
    serve.add_argument('key')
    session = commands.add_parser('session')
    session.add_argument('base')
    session.add_argument('trust', choices=('trusted', 'untrusted'))
    args = parser.parse_args()
    try:
        if args.command == 'serve':
            with make_server(args.cert, args.key) as server:
                server.serve_forever()
        else:
            verify_session(args.base, args.trust == 'trusted')
            print('Trusted CA login and authenticated players passed.' if args.trust == 'trusted' else 'Untrusted CA control rejected login without a token.')
    except Exception:
        print('WeChat HTTPS fixture check failed.', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
