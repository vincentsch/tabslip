# Tollglass

Tollglass forwards AI API calls and keeps signed receipts for OpenAI chat
completions, Responses, embeddings and Anthropic Messages. A receipt records
the requested and reported model, token usage,
timing, body hashes and findings. Model names and usage come from the provider.
The signature shows that the receipt has not changed since your Tollglass signed it.

This is an early source build. The local page and receipt commands
are not available yet. Cost is recorded as unknown.

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
The script checks streaming and JSON calls, embeddings, byte hashes, stored
usage, signatures and shutdown. It uses local fakes and synthetic credentials.

## License

MIT
