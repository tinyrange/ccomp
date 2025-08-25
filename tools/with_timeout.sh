#!/usr/bin/env bash
set -euo pipefail

# Usage: with_timeout.sh <seconds> <cmd> [args...]
# Runs command and enforces a timeout. On timeout, sends TERM then KILL.
# Returns 124 on timeout (matching GNU timeout convention), otherwise
# returns the command's exit code.

if [[ $# -lt 2 ]]; then
  echo "usage: with_timeout.sh <seconds> <cmd> [args...]" >&2
  exit 2
fi

timeout_sec="$1"; shift

flag_file=$(mktemp)
cleanup() { rm -f "$flag_file" >/dev/null 2>&1 || true; }
trap cleanup EXIT

"$@" &
cmd_pid=$!

(
  sleep "$timeout_sec"
  if kill -0 "$cmd_pid" 2>/dev/null; then
    echo timeout > "$flag_file"
    kill -TERM "$cmd_pid" 2>/dev/null || true
    # give it a short grace period
    sleep 0.1
    kill -KILL "$cmd_pid" 2>/dev/null || true
  fi
) &
killer_pid=$!

set +e
wait "$cmd_pid"; status=$?
set -e

# stop killer if still running
kill -TERM "$killer_pid" 2>/dev/null || true
wait "$killer_pid" 2>/dev/null || true

if [[ -s "$flag_file" ]]; then
  # timed out
  exit 124
fi

exit "$status"
