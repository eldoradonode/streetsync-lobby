#!/usr/bin/env bash
# Runs the load test against a LOCAL server and a THROWAWAY database. Nothing here touches production.
#
#   scripts/loadtest-local.sh smoke            # 100 clients, 20 seconds
#   scripts/loadtest-local.sh staged           # 100, 500, 1000, 3000 (long)
#   STAGES=100,500 STAGE_DURATION=45s scripts/loadtest-local.sh custom
#
# Needs a PostgreSQL you can create databases on. Set PGADMIN_URL (default below) to a superuser or CREATEDB role.
# The database it creates is called streetsync_load and is dropped and re-created each run.
set -euo pipefail
cd "$(dirname "$0")/.."
MODE="${1:-smoke}"
PGADMIN_URL="${PGADMIN_URL:-postgres://lobby:lobby@127.0.0.1:5432/postgres?sslmode=disable}"
DB_NAME="streetsync_load"
case "$DB_NAME" in streetsync_load*) ;; *) echo "refusing: database name must start with streetsync_load"; exit 1;; esac
PORT="${PORT:-18080}"
BASE="${PGADMIN_URL%%\?*}"; BASE="${BASE%/*}"
DB_URL="${BASE}/${DB_NAME}?sslmode=disable"
case "$PGADMIN_URL" in *127.0.0.1*|*localhost*|*@10.*|*@192.168.*) ;; *) echo "refusing: PGADMIN_URL must be a local or private database"; exit 1;; esac
export GOFLAGS="${GOFLAGS:--mod=mod}"
go build -o bin/lobby . && go build -o bin/loadtest ./loadtest
psql "$PGADMIN_URL" -qc "DROP DATABASE IF EXISTS ${DB_NAME}" -c "CREATE DATABASE ${DB_NAME}"
ulimit -n 65535 2>/dev/null || ulimit -n "$(ulimit -Hn)"
DATABASE_URL="$DB_URL" PORT="${PORT}" ALLOWED_ORIGINS="*" REQUIRE_PASSWORD=0 bin/lobby > loadtest/server.log 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT
for i in $(seq 1 40); do curl -fs "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1 && break; sleep 0.5; done
ARGS=(-url "ws://127.0.0.1:${PORT}/ws" -server-pid "$SRV" -db-url "$DB_URL")
case "$MODE" in
  smoke)  ARGS+=(-smoke);;
  staged) ARGS+=(-stages "${STAGES:-100,500,1000,3000}" -stage-duration "${STAGE_DURATION:-90s}");;
  custom) ARGS+=(-stages "${STAGES:?set STAGES}" -stage-duration "${STAGE_DURATION:-60s}");;
  *) echo "mode: smoke | staged | custom"; exit 1;;
esac
bin/loadtest "${ARGS[@]}" "${@:2}"
