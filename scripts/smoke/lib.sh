#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# What every smoke script shares: one folder per run with a log for each step, and a summary that
# says which step failed and where its log and picture are.
#   . scripts/smoke/lib.sh; step "Sign in"; ...; ok; finish
# SMOKE_OUT is the run's folder (default ./smoke-out); a step's log is $SMOKE_OUT/NN-name.log.

SMOKE_OUT=${SMOKE_OUT:-$PWD/smoke-out}
mkdir -p "$SMOKE_OUT"
SMOKE_OUT=$(cd "$SMOKE_OUT" && pwd)
export SMOKE_OUT
_n=$(find "$SMOKE_OUT" -maxdepth 1 -name '[0-9][0-9]-*.log' 2>/dev/null | wc -l)
_step=
_fails=0

# step "<name>": starts a step; what follows writes to its log through $LOG.
step() {
  [ -n "$_step" ] && ok
  _n=$((_n + 1))
  _step=$1
  LOG=$SMOKE_OUT/$(printf '%02d' "$_n")-$(tr -c 'A-Za-z0-9\n' - <<<"$1" | tr 'A-Z' 'a-z' | tr -s - | sed 's/-$//').log
  export LOG
  : >"$LOG"
  echo "::group::$1"
  echo "--- $1" >>"$SMOKE_OUT/summary.txt"
}

ok() {
  [ -z "$_step" ] && return
  echo "::endgroup::"
  echo "ok    $_step" | tee -a "$SMOKE_OUT/summary.txt"
  _step=
}

# fail "<why>": the step failed. The log's tail goes to the job's output, with a GitHub annotation.
fail() {
  echo "::endgroup::"
  echo "FAIL  $_step: $*" | tee -a "$SMOKE_OUT/summary.txt"
  echo "::error title=Smoke: $_step::$* (log: $(basename "$LOG"))"
  tail -40 "$LOG" 2>/dev/null | sed 's/^/    /'
  if [ -s "$SMOKE_OUT/server.log" ]; then echo "    the server's output, last lines:"; tail -15 "$SMOKE_OUT/server.log" | sed 's/^/    /'; fi
  _fails=$((_fails + 1))
  _step=
  [ "${SMOKE_KEEP_GOING:-}" = 1 ] || finish
}

# run <command...>: runs it into the step's log, and fails the step if it fails.
run() { "$@" >>"$LOG" 2>&1 || fail "$(basename "$1") exited $?"; }

# until_ok <seconds> <what> <command...>: repeats a command until it succeeds.
until_ok() {
  local secs=$1 what=$2 i
  shift 2
  local last=
  for ((i = 0; i < secs; i++)); do
    last=$("$@" 2>&1) && { [ -n "$last" ] && echo "$last" >>"$LOG"; return 0; }
    sleep 1
  done
  echo "$last" >>"$LOG"
  fail "$what: not after ${secs}s"
  return 1
}

finish() {
  [ -n "$_step" ] && ok
  echo
  cat "$SMOKE_OUT/summary.txt" 2>/dev/null | grep -E '^(ok|FAIL)'
  if [ "$_fails" -gt 0 ]; then
    echo "$_fails step(s) failed. Logs and pictures: $SMOKE_OUT"
    exit 1
  fi
  exit 0
}

# api <method> <path> [json]: the server's API as the signed-in administrator ($WMS_URL, $WMS_TOKEN).
api() {
  local m=$1 p=$2 body=${3:-}
  curl -sS -m 60 -X "$m" -H "Authorization: Bearer ${WMS_TOKEN:-}" -H 'Content-Type: application/json' \
    ${body:+--data "$body"} "$WMS_URL$p"
}

# json <expression>: reads one value from JSON on stdin (python3, which every runner has).
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print($1)"; }
