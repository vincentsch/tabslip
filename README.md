# Tollglass

Tollglass forwards AI API calls and keeps signed receipts for OpenAI chat
completions, Responses, embeddings and Anthropic Messages. A receipt records
the requested and reported model, token usage,
timing, body hashes and findings. Model names and usage come from the provider.
The signature shows that the receipt has not changed since your Tollglass signed it.

This is an early source build. The local page and receipt commands
are not available yet. Receipts now include reported or estimated costs.

## Start the gateway

Build with Go 1.27.2. An older Go installation can download the toolchain
with `GOTOOLCHAIN=auto`.

```sh
go build -o bin/tollglass ./cmd/tollglass
./bin/tollglass
```

`tollglass serve` starts the same server. It listens on `127.0.0.1:4545`.
Use `--port 4546` to choose another port. Stop with Ctrl+C or SIGTERM.
Active calls get up to 30 seconds to finish, then queued receipts are drained.

Point your client's base URL at the upstream you use:

| Upstream | Base URL |
| --- | --- |
| OpenAI | `http://127.0.0.1:4545/openai/v1` |
| OpenRouter | `http://127.0.0.1:4545/openrouter/v1` |
| Ollama | `http://127.0.0.1:4545/ollama/v1` |
| Anthropic | `http://127.0.0.1:4545/anthropic` |

All paths pass through. Receipts cover `POST /v1/chat/completions`,
`POST /v1/responses` and `POST /v1/embeddings` on OpenAI-compatible upstreams,
and `POST /v1/messages` on Anthropic upstreams. Tollglass forwards credentials
from your client; it needs no API key
of its own and does not retry provider calls.

```python
from openai import OpenAI

client = OpenAI(base_url="http://127.0.0.1:4545/openai/v1")
response = client.chat.completions.create(
    model="gpt-4.1",
    messages=[{"role": "user", "content": "Hello"}],
    extra_headers={"X-Tollglass-Tag": "my-project"},
)
```

Streaming stays streaming. OpenAI and Ollama streaming chat requests get
`stream_options.include_usage=true` when needed. OpenRouter requests stay
unchanged. Responses, embeddings and Messages requests also stay unchanged.
Receipts record any injection and hash the original client bytes.

Anthropic SDK clients use the base URL without `/v1`:

```python
from anthropic import Anthropic

client = Anthropic(base_url="http://127.0.0.1:4545/anthropic")
with client.messages.stream(
    model="claude-sonnet-4-6",
    max_tokens=256,
    messages=[{"role": "user", "content": "Hello"}],
) as stream:
    for text in stream.text_stream:
        print(text, end="", flush=True)
```

`response.complete` records capture completeness. Fully received Responses
`incomplete` and `failed` results preserve usage and `response.api_status`;
complete provider error events finish capture and record `upstream_error`.
A stream cut off before its terminal event is incomplete.

Signed Anthropic `usage.input_tokens` is the provider's uncached input.
The database summary adds cache reads and total cache writes for display;
if any category is unknown, the total is unknown. One-hour cache writes are
part of total writes and are never counted twice. Embeddings record input
usage with output tokens left null.

## Try the offline demo

```sh
./bin/tollglass demo
```

The demo sends eight simulated calls through the gateway on a free local port.
It includes chat, embeddings and Anthropic Messages, plus a simulated fallback,
a rate limit and a response without usage. It accepts later calls with normal
SDK model names and ignores API keys. It makes no external network requests.

Demo data, keys and prices stay separate from your normal setup. The demo never
reads your normal config. Ctrl+C removes its temporary data; use
`--data-dir ./demo-data` to keep it. `--dry-run` creates no state or listener.
`--json` prints one object after the eight receipts and one signed supplement
are stored. It includes the base URLs, data dir, key id and command examples.
The page and export, key-export and verify commands will be available in a
later source build; their printed examples already use the demo directory.

## How costs work

A receipt uses a provider's reported `usage.cost` when present, including zero.
For direct OpenAI and Anthropic calls, Tollglass also calculates an estimate
from an embedded LiteLLM price snapshot. Lookup stays within that provider's
namespace, preferring the reported model, then the requested model, then names
without a date suffix. A reported cost keeps the estimate beside it for
comparison. Missing prices or usage leave the cost unknown.

Estimates charge cached input and Anthropic cache writes at their own rates.
Anthropic input is already uncached; reasoning tokens are already part of
OpenAI output. Embeddings are priced from input alone. Calculations use exact
decimals and store 12 fractional digits with half-up rounding. Each attempted
direct-provider lookup records the table version, including a price miss.
OpenRouter, Ollama and custom upstreams receive no price-table estimates.

For OpenRouter, Tollglass fetches generation details after the response finishes.
It tries up to three times within ten seconds, using the client's authorization
in memory. `enrich_openrouter: false` disables this. Failed lookups leave the
receipt usable and do not affect the client response. Generation details form
a separate signed supplement with provider attempts, native tokens and region.
Its cost takes precedence in the database summary and is counted once. The
original signed receipt stays unchanged.

Server startup never downloads prices. The committed snapshot comes from
LiteLLM revision `4818b767d8b4c4e5fa9885ab953d6f5cfe7c7ca5`.
Developers can reproduce it with `go generate ./internal/prices`.
The runtime price-update command will be available in a later source build.

## Configuration and local data

Tollglass reads `--config`, then `TOLLGLASS_CONFIG`, then
`config.yaml` under your OS config directory's `tollglass` folder.
Missing files use defaults. Existing malformed YAML fails. Flags override file
settings. Preview a start without creating files or opening a listener:

```sh
./bin/tollglass serve --config config.yaml --dry-run --json
```

Custom upstreams add names; they cannot replace presets. Their base URL omits
`/v1`. For example:

```yaml
listen: 127.0.0.1
port: 4545
keep_content: false
keep_content_tags: []
retention_days: 90
upstreams:
  - name: local-test
    base_url: http://127.0.0.1:4598
    api: openai
    inject_usage: false
```

Use `http://127.0.0.1:4545/local-test/v1` for this example. Custom OpenAI-compatible
upstreams inject usage by default; set `inject_usage: false` to disable it.
Tags accept 1–64 letters, digits, dots, underscores or hyphens. Tollglass reads
and removes `X-Tollglass-*` headers before forwarding.

The data directory contains `tollglass.db`, `key.ed25519` and `key.pub`.
On Linux it defaults to `$XDG_DATA_HOME/tollglass`, or
`~/.local/share/tollglass`. macOS uses `~/Library/Application Support/tollglass`;
Windows uses `%LocalAppData%\tollglass`. Override it with `--data-dir`.
Back up the private key with the database. If the key is missing or differs
from the database identity, restore the original key or use a new data directory.

Content is off by default. Receipts contain selected parameters and hashes,
never prompt or answer text. `--keep-content`, `keep_content: true`, or
`keep_content_tags` stores content in a separate local table, capped at 10 MiB
per body. Provider error messages appear only in that local content when enabled.
Credentials from headers never enter storage or logs. Metadata extraction is
limited to 10 MiB per JSON value or SSE event; forwarding and hashing continue
when parsing fails. Retention deletes receipts and their content after 90 days;
`retention_days: 0` keeps them indefinitely.

`GET /healthz` accepts loopback connections and returns `tool`, `version`,
`key_id` and `unrecorded_calls`. A full recording queue or failed write leaves
the client call intact and increases that counter. The counter resets on restart.
`--json` prints one startup object. `--version` includes version, commit and Go version.

## Check a source build

```sh
go test ./...
go test -race ./...
go vet ./...
gofmt -l cmd internal
go run github.com/vincentsch/rungrad/cmd/rungrad@v0.3.2 score ./bin/tollglass --strict
```

Tests use local fake servers and the documented fixtures in `testdata/`.
For an SDK check, install `openai`, `anthropic` and `cryptography` in a temporary
Python environment and `openai` plus `@anthropic-ai/sdk` in a temporary Node
environment, then run `scripts/sdk-smoke.py` with that Python interpreter,
`--binary` pointing
to the built binary and `--node-module` pointing to the installed Node `openai`
package, and `--anthropic-node-module` pointing to `@anthropic-ai/sdk`.
Run `scripts/demo-smoke.py --binary bin/tollglass` with the same Python
environment to check the built demo, signatures, totals and cleanup.
The script checks streaming and JSON calls, embeddings, byte hashes, stored
usage, signatures and shutdown. It uses local fakes and synthetic credentials.

## License

MIT
