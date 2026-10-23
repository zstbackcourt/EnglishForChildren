#!/usr/bin/env bash
# Exercise the real production image without touching cloud resources or real secrets.
set -euo pipefail
image=${1:?Usage: test_wechat_image.sh IMAGE}
suffix="${GITHUB_RUN_ID:-$$}-${GITHUB_RUN_ATTEMPT:-1}"
network="wechat-ci-$suffix"
database="wechat-db-$suffix"
server="wechat-api-$suffix"
scratch=$(mktemp -d)
cleanup() {
  docker rm -f "$server" "$database" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
docker network create "$network" >/dev/null
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
docker run -d --name "$server" --network "$network" -p 127.0.0.1::8080 \
  -e WECHAT_APP_ID=wx1f23b102bcb34ed7 -e WECHAT_APP_SECRET=ci-no-real-login \
  -e WECHAT_SESSION_KEY=ci-only-signing-key-at-least-thirty-two-characters \
  -e MYSQL_ADDRESS="$database:3306" -e MYSQL_USERNAME=english_ci \
  -e MYSQL_PASSWORD=disposable-ci-only -e MYSQL_DATABASE=english_ci "$image" >/dev/null
port=$(docker port "$server" 8080/tcp | sed 's/.*://')
base="http://127.0.0.1:$port"
ready=false
for ((i=0; i<30; i++)); do
  if curl --fail --silent --max-time 2 "$base/healthz" -o "$scratch/health.json"; then ready=true; break; fi
  sleep 1
done
if [[ "$ready" != true ]]; then docker logs "$server"; exit 1; fi
node -e 'const fs=require("fs"),state=JSON.parse(fs.readFileSync(process.argv[1])); if(state.status!=="ok"||state.releaseId!==fs.readFileSync(process.argv[2],"utf8").trim())process.exit(1)' "$scratch/health.json" deploy/wechat/release-id.txt
[[ "$(curl --silent --max-time 10 -o /dev/null -w '%{http_code}' "$base/api/players")" == 401 ]]
[[ "$(curl --silent --max-time 10 -r 0-3 -o "$scratch/audio" -w '%{http_code}' "$base/media/audio/1")" == 206 ]]
[[ "$(wc -c < "$scratch/audio" | tr -d ' ')" == 4 ]]
printf 'Production image started with MySQL; health, authentication and audio Range passed.\n'
