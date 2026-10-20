# Tollglass

Tollglass forwards AI API calls and keeps signed receipts for OpenAI chat
completions, Responses, embeddings and Anthropic Messages. A receipt records
the requested and reported model, token usage,
timing, body hashes and findings. Model names and usage come from the provider.
The signature shows that the receipt has not changed since your Tollglass signed it.

Open the local page to browse live calls, receipts, spend and setup.
Receipts include reported or estimated costs and can also be read and exported from the terminal.

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
Use the printed export, key-export and verify commands with the demo directory.
Open the printed Page URL to browse the eight calls. The banner marks all demo data as simulated.

## Open the local page

Open `http://127.0.0.1:4545` after starting the gateway. Use `--open` to
launch your browser when starting it; otherwise Tollglass opens nothing.
The page shares the gateway's port and makes no external requests.

Calls updates live. Filter by tag, upstream, requested or reported model,
status and dates, or show only calls with findings. Search covers receipt ids,
tags, upstreams, models and reported providers. It never searches stored
content. Choose newest first or slowest first, then use Load more for older
rows. Press `/` for search, `j` and `k` to move, Enter to open, and Esc to
clear filters. The theme toggle remembers your choice.

Open a receipt for its paper view, original signed findings, provider
supplement, body hashes and any locally stored content. The SIGNED stamp
checks the base envelope and its supplements with your local public key.
A changed signature, signer or supplement linkage shows INVALID. The
signature does not establish whether a provider's claims are true.
Downloads contain the original content-free envelope. Delete removes the
receipt, supplements and content from this instance; external copies remain.

Spend shows exact reported and estimated subtotals and unknown-cost coverage.
Today starts at UTC midnight. Seven and thirty days include today and the
preceding six or twenty-nine dates. Tag and date filters apply to all tiles,
bars and tables. Delayed provider supplements update the open page too.

Setup has base URLs, SDK examples, the public key and content settings.
Saving writes the selected config file atomically and preserves custom
upstreams. Content can be kept for all calls or selected tags, starting with
calls that begin after saving. Retention changes delete older receipts,
supplements and content immediately, then daily; 0 keeps all receipts.
An explicit `--keep-content` flag disables its corresponding page control.

Pages, live events, downloads and settings accept loopback connections and
local Host headers only, even with `--listen 0.0.0.0`. Proxy routes still use
the chosen listen address. Use an SSH tunnel to view a gateway elsewhere.
Changes need the page's token and a same-origin POST. No CORS access is enabled.

The Setup page displays and copies this prompt from one source constant:

```text
Set up Tollglass for this project.
Use https://github.com/vincentsch/tollglass: read its AGENTS.md and follow
the steps. Route this project's AI API calls through it, make one real call,
and show me the receipt.
```

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
scripts/conformance.sh
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


## Read and share receipts

```sh
./bin/tollglass receipts --tag my-project --since 2026-10-01 --json
./bin/tollglass show rcpt_01JB8Y3Q6V9W2N4K8M0P5R7S1T
./bin/tollglass export --out calls.tollglass.json
./bin/tollglass key export --out tollglass.pub
./bin/tollglass verify calls.tollglass.json --key tollglass.pub
```

Reads work while the gateway runs and after it stops. `--data-dir` selects
local records. Reads never create a key or database. `receipts` shows the
newest 20 calls; `--limit 0` selects all. Combine `--tag`, `--upstream`,
`--model`, `--since` and `--until`. Models match the requested or reported
name exactly, including case. Dates accept RFC 3339 or `YYYY-MM-DD` in UTC;
`--since` includes its boundary and `--until` excludes it.

`show --json` returns the original signed envelope. Human output wraps long
values and includes any provider supplement. Missing receipts exit 5.

Exports select all retained receipts unless filtered. The default bundle
keeps signed receipt and supplement envelopes. `--format jsonl` writes one
unsigned decoded `{receipt, supplements}` record per line. CSV writes one
summary row per call; HTML is a self-contained printable spending statement.
All formats exclude locally stored prompts and answers. CSV and statements
apply supplement costs once and retain unknown values. Statements show exact
reported and estimated subtotals, plus unknown-cost coverage.

`--out` is required and must name a new file. Use `--dry-run` to preview an
export without writing. `--json` keeps command results machine-readable.

Verification works offline without a database. Confirm a sender's public key
or key id through another channel. A bundle's own key does not confirm its
signer. Use `--key file.pub` or `--key-id tg_…`; the flags are alternatives.
Without either, an existing matching local public key confirms the signer.
A single envelope requires `--key`. Exit 0 means valid signatures and a
confirmed signer; 10 means invalid records; 11 means valid signatures with
an unconfirmed or mismatched signer. Usage errors exit 1, runtime errors 2,
and missing files 5. JSON includes per-envelope results and claim limits.
Signatures protect the records; they do not check which model ran.

## Check the setup and prices

`tollglass doctor` checks config, writable storage, the key and database,
the listening port and upstream reachability. Missing initial state is
information. Unreachable presets are warnings; custom upstreams must answer.
Any HTTP response counts as reachable, including an authentication error.
The check sends no credentials and does not validate them. Each failed check
includes a fix. Exit 2 means at least one failed check; `--json` lists results.

`tollglass prices` shows the selected table, date and SHA-256 without a
network request. `tollglass prices update` explicitly downloads LiteLLM's
current table and installs a named snapshot. A failed update keeps the prior
table. `--dry-run` makes no request or change. Restart a running gateway to
use a new snapshot; already signed costs never change. The offline demo
always uses its isolated embedded table.

Run `scripts/conformance.sh` for the CLI contract checks and
`scripts/receipts-smoke.py --binary bin/tollglass` for offline receipt checks.

## License

MIT
