#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d)
demo_pid=
cleanup() {
  if [[ -n "$demo_pid" ]]; then kill -TERM "$demo_pid" 2>/dev/null || true; wait "$demo_pid" 2>/dev/null || true; fi
  rm -rf "$work"
}
trap cleanup EXIT INT TERM
CGO_ENABLED=0 go build -o "$work/tollglass" ./cmd/tollglass
"$work/tollglass" demo --data-dir "$work/data" --json >"$work/startup.json" 2>"$work/demo.log" &
demo_pid=$!
for ((i=0;i<300;i++)); do
  [[ -s "$work/startup.json" ]] && break
  kill -0 "$demo_pid" 2>/dev/null || { cat "$work/demo.log" >&2; exit 1; }
  sleep 0.1
done
[[ -s "$work/startup.json" ]] || { echo 'Demo did not become ready.' >&2; exit 1; }
# No self-update, destructive delete, authentication or ambiguous-name resolution
# commands exist. Those scorer fixtures do not apply to this local receipt tool.
go run github.com/vincentsch/rungrad/cmd/rungrad@v0.3.2 score "$work/tollglass" \
  --manifest auto --strict \
  --read "receipts --data-dir $work/data" \
  --mutate "export --data-dir $work/data --out $work/x.tollglass.json" \
  --not-found "show rcpt_00000000000000000000000000 --data-dir $work/data"
