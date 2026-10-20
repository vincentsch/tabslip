# Start using Tollglass

Tollglass forwards AI API calls and keeps signed receipts on your computer.
You can [ask your coding agent to connect a project](setup-with-an-agent.md)
or follow the steps below. The [FAQ](faq.md) covers costs, storage and stopping.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/tollglass/main/install.sh | sh
tollglass --version
```

The installer needs a POSIX shell, curl, and `sha256sum` or `shasum`.
It supports Linux, macOS and Windows through Git Bash. Add the printed install
directory to PATH if needed. Release binaries do not need Go or another runtime.

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
separately.

With GitHub CLI installed, check the download's build provenance separately
from the installer's checksum comparison:

```sh
gh attestation verify tollglass-<os>-<arch> --repo vincentsch/tollglass
```

Replace `<os>` with `linux`, `darwin` or `windows`, and `<arch>` with `amd64`
or `arm64`. Add `.exe` for Windows, for example `tollglass-windows-amd64.exe`.
See [release checks](../RELEASE.md) for the publication and provenance procedure.

Source installation needs the Go version in [go.mod](../go.mod).
`GOTOOLCHAIN=auto` can download the toolchain:

```sh
go install github.com/vincentsch/tollglass/cmd/tollglass@latest
```

A source install reports `dev`. Go installs in `GOBIN` or `$(go env GOPATH)/bin`.
See [CONTRIBUTING.md](../CONTRIBUTING.md) to build from a checkout.

## Try it without a key

```sh
tollglass demo
```

Open the printed Page URL. The demo sends eight simulated calls and a signed
supplement through the gateway. It makes no external requests and prints
ready-to-run export and verify commands. Use its printed data directory for
[receipt commands](verify.md), while the demo is still running.
Ctrl+C removes this temporary directory. To keep the receipts instead:

```sh
tollglass demo --data-dir ./demo-data
```

Each demo chooses a free port. For SDK checks, replace the preset URLs below
with its printed OpenAI URL ending in `/demo/v1` or Anthropic URL ending in
`/demo`. Use the synthetic key `demo`. Preset routes such as `/openai/v1`
belong to `serve`; they do not select the demo upstream.

## Connect your app

After installing, start the gateway:

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
See [custom upstreams, Docker and remote pages through SSH](upstreams.md).

A saved environment file needs to be loaded by your app in each fresh shell.
For example, if your project already uses a POSIX `.env` file:

```sh
set -a
. ./.env
set +a
```

Keep keys in their existing source. Add the base URL to that configuration,
or keep the SDK option shown above in your code. The examples use explicit
SDK options; Python OpenAI and Node OpenAI also accept `OPENAI_BASE_URL`.
See [runnable examples](../examples/README.md) for curl and environment files.
For receipt fields, see [receipts](receipts.md); for problems, see
[troubleshooting](troubleshooting.md).
