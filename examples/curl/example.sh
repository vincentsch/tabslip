#!/bin/sh
set -eu
: "${OPENAI_API_KEY:?Set OPENAI_API_KEY (demo for the offline demo)}"
: "${OPENAI_BASE_URL:?Set OPENAI_BASE_URL to the printed URL}"
# The model is fixed so the JSON works with only POSIX shell and curl.
set -- -fsS "${OPENAI_BASE_URL%/}/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" -H 'Content-Type: application/json'
if [ -n "${TOLLGLASS_TAG:-}" ]; then
  set -- "$@" -H "X-Tollglass-Tag: $TOLLGLASS_TAG"
fi
curl "$@" -d '{"model":"gpt-4.1","max_completion_tokens":64,"messages":[{"role":"user","content":"Say hello in one short sentence."}]}'
printf '\n'
