#!/usr/bin/env bash

set -euo pipefail

colors=(31 32 33 34 35 36 91 92 93 94 95 96)
declare -A tail_pids
declare -A grain_colors
color_index=0

cleanup() {
  for pid in "${tail_pids[@]:-}"; do
    kill "$pid" 2>/dev/null || true
  done
  for pid in "${tail_pids[@]:-}"; do
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

spawn_tail() {
  local grain_id="$1"
  local log_path="$2"
  local prefix="${grain_id:0:6}"
  local color

  if [ -n "${grain_colors[$grain_id]+set}" ]; then
    color="${grain_colors[$grain_id]}"
  else
    color="${colors[$((color_index % ${#colors[@]}))]}"
    grain_colors[$grain_id]="$color"
    color_index=$((color_index + 1))
  fi

  (
    # Keep the formatter in this shell so it can terminate and reap its tail.
    # Killing a shell around a pipeline does not kill the pipeline's children.
    tail_pid=
    cleanup_tail() {
      if [ -n "$tail_pid" ]; then
        kill "$tail_pid" 2>/dev/null || true
        wait "$tail_pid" 2>/dev/null || true
      fi
    }
    trap cleanup_tail EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    exec {log_fd}< <(exec tail -n 0 -F "$log_path" 2>/dev/null)
    tail_pid=$!
    while IFS= read -r line <&"$log_fd"; do
      printf '\r\033[K\033[%sm[%s]\033[0m %s\n' "$color" "$prefix" "$line"
    done
  ) &
  tail_pids[$grain_id]=$!
}

while true; do
  for log_path in /opt/sandstorm/var/sandstorm/grains/*/log; do
    [ -f "$log_path" ] || continue
    grain_id="$(basename "$(dirname "$log_path")")"
    if [ -n "${tail_pids[$grain_id]+set}" ] && kill -0 "${tail_pids[$grain_id]}" 2>/dev/null; then
      continue
    fi
    spawn_tail "$grain_id" "$log_path"
  done
  sleep 1
done
