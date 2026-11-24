#!/usr/bin/env bash
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
# Stops what scripts/smoke/server.sh started, by the PIDs it wrote, and nothing else.
SMOKE_OUT=${SMOKE_OUT:-$PWD/smoke-out}
for f in server catalog; do
  pid=$(cat "$SMOKE_OUT/$f.pid" 2>/dev/null) || continue
  kill "$pid" 2>/dev/null
  for _ in $(seq 1 20); do kill -0 "$pid" 2>/dev/null || break; sleep 0.5; done
  kill -9 "$pid" 2>/dev/null
  rm -f "$SMOKE_OUT/$f.pid"
done
true
