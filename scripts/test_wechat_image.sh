#!/usr/bin/env bash
# Exercise the real production image without touching cloud resources or real secrets.
set -euo pipefail
umask 077
image=${1:?Usage: test_wechat_image.sh IMAGE}
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
fixture="$script_dir/fixtures/wechat_https.py"
release_id=$(cat "$script_dir/../deploy/wechat/release-id.txt")
suffix="${GITHUB_RUN_ID:-$$}-${GITHUB_RUN_ATTEMPT:-1}"
network="wechat-ci-$suffix"
database="wechat-db-$suffix"
server="wechat-api-$suffix"
control="wechat-control-$suffix"
mock="wechat-https-$suffix"
scratch=$(mktemp -d)
cleanup() {
  docker rm -f "$server" "$control" "$mock" "$database" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
python3 "$script_dir/test_wechat_https_fixture.py"
mkdir "$scratch/ca" "$scratch/tls"
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$scratch/ca.key" \
  -out "$scratch/ca/ci-ca.pem" -subj '/CN=Disposable WeChat Test CA' -days 1 >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -keyout "$scratch/tls/server.key" \
  -out "$scratch/server.csr" -subj '/CN=api.weixin.qq.com' >/dev/null 2>&1
printf 'subjectAltName=DNS:api.weixin.qq.com\nbasicConstraints=CA:FALSE\nextendedKeyUsage=serverAuth\n' > "$scratch/server.ext"
openssl x509 -req -in "$scratch/server.csr" -CA "$scratch/ca/ci-ca.pem" -CAkey "$scratch/ca.key" \
  -CAserial "$scratch/ca.srl" -CAcreateserial -out "$scratch/tls/server.pem" \
  -days 1 -extfile "$scratch/server.ext" >/dev/null 2>&1
# The production nonroot user receives only the public test CA, never private keys.
chmod 755 "$scratch/ca"
chmod 644 "$scratch/ca/ci-ca.pem"
docker network create --internal "$network" >/dev/null
docker run -d --name "$mock" --network "$network" --network-alias api.weixin.qq.com \
  -v "$fixture:/fixture.py:ro" -v "$scratch/tls:/tls:ro" \
  python:3.13-alpine python /fixture.py serve /tls/server.pem /tls/server.key >/dev/null
ready=false
for ((i=0; i<30; i++)); do
  if docker exec "$mock" python -c 'import socket; socket.create_connection(("127.0.0.1",443),2).close()' >/dev/null 2>&1; then ready=true; break; fi
  sleep 1
done
if [[ "$ready" != true ]]; then printf 'HTTPS fixture did not start.\n' >&2; exit 1; fi
docker run -d --name "$database" --network "$network" \
  -e MYSQL_ROOT_PASSWORD=disposable-ci-root -e MYSQL_DATABASE=english_ci \
  -e MYSQL_USER=english_ci -e MYSQL_PASSWORD=disposable-ci-only mysql:8.0.33 >/dev/null
ready=false
for ((i=0; i<60; i++)); do
  if docker exec "$database" mysql -uenglish_ci -pdisposable-ci-only -e 'SELECT 1' english_ci >/dev/null 2>&1; then
    ready=true; break
  fi
  sleep 2
done
if [[ "$ready" != true ]]; then docker logs "$database"; exit 1; fi
start_server() {
  local name=$1
  shift
  docker run -d --name "$name" --network "$network" \
    -v "$scratch/ca:/app/cert:ro" \
    -e WECHAT_APP_ID=wx0000000000000000 -e WECHAT_APP_SECRET=ci-test-secret \
    -e WECHAT_SESSION_KEY=ci-only-signing-key-at-least-thirty-two-characters \
    -e MYSQL_ADDRESS="$database:3306" -e MYSQL_USERNAME=english_ci \
    -e MYSQL_PASSWORD=disposable-ci-only -e MYSQL_DATABASE=english_ci "$@" "$image" >/dev/null
  base="http://$name:8080"
  ready=false
  for ((i=0; i<30; i++)); do
    if docker exec "$mock" python /fixture.py ready "$base" "$release_id" >/dev/null 2>&1; then ready=true; break; fi
    sleep 1
  done
  if [[ "$ready" != true ]]; then docker logs "$name"; exit 1; fi
}
# A separate process avoids Go's cached root pool; the same CA is mounted but excluded.
start_server "$control" -e SSL_CERT_DIR=/etc/ssl/certs
docker exec "$mock" python /fixture.py session "$base" untrusted
docker rm -f "$control" >/dev/null
# Do not override SSL_CERT_DIR: this must exercise the production Dockerfile default.
start_server "$server"
docker exec "$mock" python /fixture.py checks "$base" "$release_id"
docker exec "$mock" python /fixture.py session "$base" trusted
printf 'Production image with MySQL passed health, CA controls, login, authentication and audio Range.\n'
