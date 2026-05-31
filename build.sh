#!/usr/bin/env bash
# Build and run reds with its SWIM/Solace target reader.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

if [[ -f .env ]]; then
    set -o allexport
    # shellcheck disable=SC1091
    . ./.env
    set +o allexport
fi

JAR="swim/smes/target/faascope-stdds-1.0-SNAPSHOT.jar"
WS_PORT="${WS_PORT:-8080}"
export WS_PORT

mkdir -p build

echo "[build] Building reds (Go frontend)..." >&2
go build -o build/reds ./cmd/reds

echo "[build] Building SMES reader..." >&2
mvn -q -f swim/smes/pom.xml package

STALE_PIDS="$(lsof -ti "tcp:${WS_PORT}" -sTCP:LISTEN 2>/dev/null || true)"
if [[ -n "$STALE_PIDS" ]]; then
    echo "[run] Killing stale listener(s) on :$WS_PORT - $STALE_PIDS" >&2
    kill $STALE_PIDS 2>/dev/null || true
    sleep 0.3
    kill -9 $STALE_PIDS 2>/dev/null || true
fi

echo "[run] Starting SWIM/Solace consumer on websocket port $WS_PORT..." >&2
WS_PORT="$WS_PORT" java -jar "$JAR" &
CONSUMER_PID=$!

cleanup() {
    [[ -n "${CONSUMER_PID:-}" ]] || return 0
    kill "$CONSUMER_PID" 2>/dev/null || true
    for _ in 1 2 3 4 5; do
        kill -0 "$CONSUMER_PID" 2>/dev/null || return 0
        sleep 0.1
    done
    kill -9 "$CONSUMER_PID" 2>/dev/null || true
    wait "$CONSUMER_PID" 2>/dev/null || true
}
trap cleanup EXIT INT TERM HUP

for _ in 1 2 3 4 5 6 7 8 9 10; do
    if lsof -ti "tcp:${WS_PORT}" -sTCP:LISTEN >/dev/null 2>&1; then
        break
    fi
    sleep 0.2
done

echo "[run] Launching reds..." >&2
./build/reds
