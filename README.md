<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/media/wordmark-dark.svg">
    <img src="docs/media/wordmark-light.svg" alt="Tollglass" width="440" height="88">
  </picture>
</p>

<p align="center"><strong>Receipts for your AI API calls.</strong></p>

Tollglass forwards calls to your AI provider and keeps receipts signed by your Tollglass.

<p align="center"><a href="docs/commands.md">Docs</a> · <a href="examples/README.md">Examples</a> · <a href="AGENTS.md">AGENTS.md</a></p>

![The Calls page in dark theme, with eight simulated demo calls, model names, tokens, costs and findings](docs/media/calls-dark.webp)

![A simulated call as a paper receipt in light theme, with its signed stamp and provider details](docs/media/receipt-light.webp)

## Set it up with your coding agent

Copy this into your coding agent inside the project you want to connect:

```text
Set up Tollglass for this project.
Use https://github.com/vincentsch/tollglass: read its AGENTS.md and follow
the steps. Route this project's AI API calls through it, make one real call,
and show me the receipt.
```

The agent finds your AI client, keeps its provider and keys, and saves a small base URL change.
It installs or reuses Tollglass, makes one provider call that may cost money, and shows the receipt and undo steps.
Read [what the agent changes](docs/setup-with-an-agent.md).

## Try it without a key

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/tollglass/main/install.sh | sh
tollglass demo
```

The public installer and release downloads await publication. From a checkout,
use `sh install.sh` with an available release, or [build from source](#install-options).
The shell installer needs a POSIX shell, curl, and `sha256sum` or `shasum`.
It supports Linux, macOS and Windows through Git Bash. Add the printed install
directory to PATH if needed. Release binaries do not need Go.

Open the printed Page URL. The demo sends eight simulated calls through the
same gateway, including a fallback, an error and a call with unknown usage.
It makes no external requests. Later calls use the printed OpenAI `/demo/v1`
or Anthropic `/demo` base URL on a free port, with a synthetic key such as `demo`.
The [examples](examples/README.md) show how. Use the printed data directory for
receipt commands; it is separate from your normal instance and removed on
Ctrl+C. Use `tollglass demo --data-dir ./demo-data` to keep it.

## Connect your app yourself

Install as above, then start the gateway:

```sh
tollglass serve
```

Open `http://127.0.0.1:4545`. Calls updates live; click a call for its paper
receipt, or open Spend for totals and Setup for URLs and content settings.
The theme button remembers your choice. `--open` opens the page on startup.

Change your client's base URL, keeping its existing key and model:

| Provider | Client base URL at the default port |
| --- | --- |
| OpenAI | `http://127.0.0.1:4545/openai/v1` |
| OpenRouter | `http://127.0.0.1:4545/openrouter/v1` |
| Ollama | `http://127.0.0.1:4545/ollama/v1` |
| Anthropic | `http://127.0.0.1:4545/anthropic` |

These snippets make provider calls that may cost money. Python OpenAI reads
`OPENAI_API_KEY` from the environment:

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:4545/openai/v1")
response = client.chat.completions.create(
    model="gpt-4.1", messages=[{"role": "user", "content": "Hello"}])
print(response.choices[0].message.content)
```

Node OpenAI uses the same key variable:

```js
import OpenAI from 'openai';
const client = new OpenAI({baseURL: 'http://127.0.0.1:4545/openai/v1'});
const response = await client.chat.completions.create({
  model: 'gpt-4.1', messages: [{role: 'user', content: 'Hello'}]});
console.log(response.choices[0].message.content);
```

Anthropic reads `ANTHROPIC_API_KEY`. Its SDK appends `/v1/messages`, so leave
`/v1` out of the base URL:

```python
from anthropic import Anthropic
client = Anthropic(base_url="http://127.0.0.1:4545/anthropic")
response = client.messages.create(model="claude-sonnet-4-6", max_tokens=64,
    messages=[{"role": "user", "content": "Hello"}])
print(response.content[0].text)
```

```ts
import Anthropic from '@anthropic-ai/sdk';
const client = new Anthropic({baseURL: 'http://127.0.0.1:4545/anthropic'});
const response = await client.messages.create({model: 'claude-sonnet-4-6',
  max_tokens: 64, messages: [{role: 'user', content: 'Hello'}]});
console.log(response.content);
```

Save the URL in your project's normal environment or SDK configuration.
Tollglass forwards the client's credentials and does not retry provider calls.
Restore the original URL before stopping Tollglass if the app must keep working.
It must keep running, including after reboot; installation does not add a service.
See [custom upstreams, Docker and remote pages through SSH](docs/upstreams.md).

## What a receipt tells you

A receipt records requested and reported models, provider, tokens, cost,
timing, body hashes and findings. Model names, provider names, token counts
and provider-reported cost come from the provider. Tollglass labels them
reported. Times, sizes, hashes and errors Tollglass measured itself are observed.
Missing usage is unknown, never zero.

Cost is reported, estimated from a versioned price table, or unknown.
Direct OpenAI and Anthropic calls can have estimates; OpenRouter can add a
signed generation supplement. Supplements can update displayed cost and
provider without changing the signed base receipt. See [costs](docs/costs.md)
and the [field reference](docs/receipts.md).

The signature shows the receipt was not changed after your Tollglass signed it.
It does not show that the provider told the truth, and it does not stop the
owner of the key from making up a receipt. Tollglass did not check the model itself.

## Share and check receipts

Use your instance's data directory. For a demo, copy the printed directory;
the following POSIX commands ask for it rather than assuming a path:

```sh
printf 'Instance data directory: '
read -r DATA_DIR
tollglass receipts --data-dir "$DATA_DIR" --limit 0
tollglass export --data-dir "$DATA_DIR" --out calls.tollglass.json
tollglass key export --data-dir "$DATA_DIR" --out tollglass.pub
tollglass key --data-dir "$DATA_DIR"
```

Confirm the public key or its fingerprint with the instance owner through a
separate channel before trusting it. Once confirmed:

```sh
tollglass verify calls.tollglass.json --data-dir "$DATA_DIR" --key tollglass.pub
```

Verification answers three questions: are the signatures valid, is the signer
confirmed, and what can the receipts tell you? Exit 0 means valid signatures
and a confirmed signer, 10 means invalid records, and 11 means valid signatures
with an unconfirmed signer. An existing matching local key can confirm the
signer; explicit signer flags take precedence. [Offline verification](docs/verify.md)
also covers single envelopes and confirming a key id.

Bundles keep signed envelopes. CSV, JSONL and HTML exports are unsigned.
HTML is a printable statement with a call table. Output files must be new.

## What stays local

By default, Tollglass stores metadata and hashes, without prompt or answer
text. Optional content storage stays local, for all calls or selected tags;
the page header shows the current setting. Content never enters receipt
downloads or exports, even when stored locally. API keys pass through and are
never stored. Retention defaults to 90 days; 0 keeps records indefinitely.

There is no telemetry. Network contacts are your upstream, OpenRouter's
optional generation lookup, explicit price downloads and doctor reachability
checks. Startup does not download prices. Read [privacy and local paths](docs/privacy.md)
before sharing metadata or enabling content storage.

## Compatibility

All upstream paths forward. Receipt capture covers these POST endpoints:

| API | JSON | Streaming | Captured fields when provided |
| --- | --- | --- | --- |
| `/v1/chat/completions` | Yes | SSE | Model, id, provider, finish reason, fingerprint, input/output/cache/reasoning tokens, reported cost |
| `/v1/responses` | Yes | SSE | Model, id, API status, usage, errors; terminal incomplete/failed status preserved |
| `/v1/embeddings` | Yes | No | Model and input usage; output tokens stay unknown |
| Anthropic `/v1/messages` | Yes | SSE | Model, id, stop reason, uncached input, cache reads/writes and output tokens |

All receipts include observed status, timing, sizes and hashes. Interrupted
streams mark capture incomplete. Direct OpenAI and Ollama streaming chat can
inject `stream_options.include_usage=true`; OpenRouter requests remain unchanged.
Presets are OpenAI, Anthropic, OpenRouter and Ollama; custom upstreams can use
either API shape. This describes API coverage, not testing against live providers.

The local SDK checks exercised Python openai and anthropic, Node openai and
Anthropic TypeScript. [Examples](examples/README.md#tested-dependencies) record
the versions. They use the demo or local fake upstreams, with synthetic keys.
Framework discovery advice in AGENTS.md is not a tested integration claim.

## Commands and docs

`serve`, `demo`, `receipts`, `show`, `export`, `verify`, `key`, `prices` and
`doctor` are covered in the [command guide](docs/commands.md).

## Install options

The installer verifies SHA-256 checksums before replacing the binary.
`INSTALL_DIR` selects its destination (default `~/.local/bin`);
`TOLLGLASS_VERSION` selects a release tag. From a checkout:

```sh
TOLLGLASS_VERSION=v0.1.0 sh install.sh
```

For native Windows, download your architecture's `.exe` and `checksums.txt`
from [Releases](https://github.com/vincentsch/tollglass/releases). In PowerShell,
for an amd64 download in the current directory:

```powershell
Get-FileHash -Algorithm SHA256 .\tollglass-windows-amd64.exe
Get-Content .\checksums.txt | Select-String 'tollglass-windows-amd64.exe'
.\tollglass-windows-amd64.exe --version
.\tollglass-windows-amd64.exe demo
```

Compare the full hash before running the executable. Use the arm64 filename
on ARM Windows. A checksum detects changed downloads; confirm the publisher
separately. Native Windows and macOS execution await publication checks.

With GitHub CLI installed, check the download's build provenance separately
from the installer's checksum comparison:

```sh
gh attestation verify tollglass-<os>-<arch> --repo vincentsch/tollglass
```

Replace `<os>` with `linux`, `darwin` or `windows`, and `<arch>` with `amd64`
or `arm64`. Add `.exe` for Windows, for example `tollglass-windows-amd64.exe`.
See [release checks](RELEASE.md) for the publication and provenance procedure.

Source builds need Go 1.27.2; `GOTOOLCHAIN=auto` can download the toolchain.
From a checkout:

```sh
make build
./bin/tollglass --version
make test race lint conformance docs-check
```

Or install from source:

```sh
go install github.com/vincentsch/tollglass/cmd/tollglass@latest
```

A plain source install reports version `dev`. Go installs in `GOBIN` or
`$(go env GOPATH)/bin`. `make build-all VERSION=0.1.0 COMMIT=<commit>` creates
six release binaries and `checksums.txt` in `dist/`; replace `<commit>` with
the build revision. `make docs` regenerates only `docs/reference/`.

## Contributing and security

Read [CONTRIBUTING.md](CONTRIBUTING.md) to change Tollglass. Report security
issues through the channel described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE).
