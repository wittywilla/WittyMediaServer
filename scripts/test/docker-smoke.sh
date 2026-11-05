#!/bin/bash
# Runs an image the way docs/DOCKER.md says, and checks what it promises: locked down
# as any user, the health check, the hardware report, a clear message when /config
# can't be written, no second server from a misspelt command, and that every compose
# file in deploy/docker is valid.
#   scripts/test/docker-smoke.sh <image>
set -uo pipefail
IMG=${1:?usage: docker-smoke.sh <image>}
cd "$(dirname "$0")/../.."
fails=0
ok() { echo "ok:   $*"; }
bad() { echo "FAIL: $*"; fails=$((fails + 1)); }
hard=(--read-only --tmpfs /tmp --cap-drop ALL --security-opt no-new-privileges:true)
name=wms-smoke-$$
vol=wms-smoke-$$
cleanup() { docker rm -f "$name" "$name-bad" >/dev/null 2>&1; docker volume rm "$vol" >/dev/null 2>&1; rm -rf "${bad_dir:-}"; }
trap cleanup EXIT

# 1. Locked down, as a user the image has never heard of, on a new named volume.
docker run -d --name "$name" "${hard[@]}" --user 4242:4242 -p 127.0.0.1::9797 -v "$vol":/config "$IMG" >/dev/null
state=""
for _ in $(seq 1 90); do
  state=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null)
  [ "$state" = healthy ] && break
  [ "$(docker inspect -f '{{.State.Running}}' "$name")" = true ] || break
  sleep 1
done
if [ "$state" = healthy ]; then ok "healthy, read-only, no capabilities, as 4242:4242"; else bad "health: ${state:-none}"; docker logs "$name" 2>&1 | tail -20; fi
port=$(docker port "$name" 9797/tcp | head -n1 | sed 's/.*://')
if curl -sf "http://127.0.0.1:$port/api/health" | grep -q '"ok":true'; then ok "answers on the published port"; else bad "no answer on port $port"; fi
if docker logs "$name" 2>&1 | grep -q "no accounts yet"; then ok "first start asks for setup"; else bad "no setup message"; fi
if docker exec "$name" sh -c 'touch /usr/local/bin/x' 2>/dev/null; then bad "the root file system is writable"; else ok "the root file system is read-only"; fi

# 2. The hardware report, with no graphics passed in.
out=$(docker exec "$name" wittymediaserver hardware 2>&1); code=$?
if [ $code = 1 ] && grep -q "No graphics device was passed to the container" <<<"$out"; then ok "hardware report"; else bad "hardware report (exit $code): $out"; fi

# 3. A misspelt command doesn't start a second server.
docker exec "$name" wittymediaserver hardwar >/dev/null 2>&1; code=$?
if [ $code = 2 ]; then ok "a misspelt command is refused"; else bad "a misspelt command exited $code"; fi

# 4. What's in the image.
if docker exec "$name" sh -c 'command -v wget || command -v curl' >/dev/null 2>&1; then bad "an HTTP client is in the image"; else ok "no HTTP client in the image"; fi
if docker exec "$name" test -f /usr/share/doc/ffmpeg/copyright; then ok "copyright files kept"; else bad "copyright files missing"; fi
if docker exec "$name" sh -c 'ls /usr/share/man/man1 2>/dev/null | grep -q .'; then bad "manuals are in the image"; else ok "no manuals"; fi

# 5. A /config the user can't write: it stops, and says why.
bad_dir=$(mktemp -d); chmod 0755 "$bad_dir"
docker run --name "$name-bad" "${hard[@]}" --user 4243:4243 -v "$bad_dir":/config "$IMG" >/dev/null 2>&1; code=$?
msg=$(docker logs "$name-bad" 2>&1)
if [ $code != 0 ] && grep -q "runs as user 4243 (group 4243) and can't write /config" <<<"$msg"; then ok "an unwritable /config is explained"; else bad "unwritable /config (exit $code): $msg"; fi

# 6. Every compose file in deploy/docker, and the one at the top.
for f in deploy/docker/compose*.yml docker-compose.yml; do
  if docker compose -f "$f" config -q 2>/dev/null; then ok "valid: $f"; else bad "invalid: $f"; docker compose -f "$f" config -q; fi
done

echo
[ $fails = 0 ] && echo "all passed" || echo "$fails failed"
exit $((fails > 0))
