#!/usr/bin/env bash

set -euo pipefail

if [ "${1:-}" != "--" ]; then
  echo "usage: dev-with-tail.sh -- <command> [args...]" >&2
  exit 2
fi
shift

if [ "$#" -eq 0 ]; then
  echo "dev-with-tail.sh: missing command after --" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tailer_script="$script_dir/grain-log-tailer.sh"
if [ ! -f "$tailer_script" ]; then
  echo "dev-with-tail.sh: missing $tailer_script" >&2
  exit 1
fi

cleanup() {
  if [ -n "${command_pid:-}" ]; then
    kill "$command_pid" 2>/dev/null || true
  fi
  if [ -n "${grain_tail_pid:-}" ]; then
    kill "$grain_tail_pid" 2>/dev/null || true
    wait "$grain_tail_pid" 2>/dev/null || true
  fi
  if [ -n "${command_pid:-}" ]; then
    wait "$command_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

sudo -u sandstorm bash "$tailer_script" &
grain_tail_pid=$!

# Waiting on a background job lets Bash handle signals immediately. Preserve
# stdin, which Bash otherwise redirects to /dev/null for asynchronous commands.
"$@" <&0 &
command_pid=$!
status=0
wait "$command_pid" || status=$?
command_pid=
exit "$status"
