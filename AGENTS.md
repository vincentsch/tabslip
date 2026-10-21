[Changing Tabslip itself? Read CONTRIBUTING.md](CONTRIBUTING.md)

# Set up Tabslip for a project

Tabslip forwards AI API calls and keeps receipts signed by your Tabslip.
Follow these steps when a user asks you to connect their project. Follow the
host project's rules too. Make a small change they can undo.

The setup prompt is:

```text
Set up Tabslip for this project.
Use https://github.com/vincentsch/tabslip: read its AGENTS.md and follow
the steps. Route this project's AI API calls through it, make one real call,
and show me the receipt.
```

## 1. Find the calls before installing

Read the project instructions, dependencies and AI client creation sites.
Find the current provider, base URL and credentials source without printing
keys or copying them to new files. Report which files or settings need to
change before editing them. If there are no AI calls, explain that and stop.

Look for these client patterns:

| Client | Places to check |
| --- | --- |
| OpenAI | `OpenAI`, `base_url` / `baseURL`, `OPENAI_BASE_URL` |
| Anthropic | `Anthropic`, `base_url` / `baseURL`, `ANTHROPIC_BASE_URL` |
| OpenRouter | OpenAI-compatible clients pointing at `openrouter.ai/api/v1` |
| Vercel AI SDK | Provider factories such as `createOpenAI`, `createAnthropic`, `baseURL` |
| LangChain | Model constructors, `base_url`, `openai_api_base`, client configuration |
| LiteLLM | `api_base`, provider/model selection, proxy config |
| Laravel openai-php / Prism | Service config, provider config, base URL options and env bindings |
| Raw HTTP | URL construction for `/v1/chat/completions`, `/v1/responses`, `/v1/embeddings`, `/v1/messages` |

## 2. Check the binary

Run `tabslip --version`. Only a shell's command-not-found result means it
is missing. Report any other failure and diagnose it; do not reinstall over it.

If missing, install from a Tabslip checkout with `sh <checkout>/install.sh`,
or use:

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/tabslip/main/install.sh | sh
```

The POSIX installer supports Linux and macOS, and Windows through Git Bash.
For native Windows, download the matching `.exe` and `checksums.txt` from the
GitHub release and compare `Get-FileHash -Algorithm SHA256` before using it.
If Go is available, the source fallback is:

```sh
go install github.com/vincentsch/tabslip/cmd/tabslip@latest
```

That source install reports `dev` in v0.1. Release binaries report their
version and commit. Recheck `--version` after installation. Respect an existing `INSTALL_DIR`;
the installer uses it instead of `~/.local/bin`. If the command already resolves,
keep PATH as it is. Otherwise add only the selected install directory to this
process and report that fix. Run `command -v tabslip` (PowerShell:
`(Get-Command tabslip).Source`) and record the resolved executable path.
Report that path in the final answer, rather than assuming the default.
Go installs to `GOBIN` or `$(go env GOPATH)/bin`.
Do not edit shell profiles unless asked.

## 3. Locate the instance

Use the OS defaults unless the user selected an override. Resolve the config
file from `--config`, then `TABSLIP_CONFIG`, then the default below. Record
its absolute path. When an environment or flag selects a location, use it;
do not probe fallback locations under the home directory. `--data-dir` selects a data override; otherwise use the OS
default. Do not create a project-local instance that another session cannot find.

| OS | Default config file | Default data directory |
| --- | --- | --- |
| Linux | `$XDG_CONFIG_HOME/tabslip/config.yaml` or `~/.config/tabslip/config.yaml` | `$XDG_DATA_HOME/tabslip` or `~/.local/share/tabslip` |
| macOS | `~/Library/Application Support/tabslip/config.yaml` | `~/Library/Application Support/tabslip` |
| Windows | `%AppData%\tabslip\config.yaml` | `%LocalAppData%\tabslip` |

Run `tabslip doctor --json` with those settings. Include `--config` and
`--data-dir` when the user selected those flags. Keep the same settings for
later commands. Unreachable preset warnings alone do not prevent setup.
An HTTP answer from an upstream does not validate credentials.

If doctor finds a running Tabslip, compare its `port` check's `ts_…` key id
with the `identity` check's `Database and key pair match: ts_…` id. Reuse only
when they match. `/healthz` returns a tool name, version, key id and recording
counter; it cannot tell you the config or data path. If the identity is missing
or differs, ask for the running instance's data directory. Rerun doctor with
that directory and confirm the match. Never create an identity to make it match.

If another program owns the port, leave it running. Find a free port, then
save `port: <free-port>` in the resolved YAML config, preserving other settings.
Rerun doctor without a temporary port override to check future discovery.
Resolve genuine config, storage or recording failures before continuing.

## 4. Keep the same provider

Keep the existing keys and provider routing. For these providers, use the
corresponding local base URL at the selected port:

| Provider | Client base URL |
| --- | --- |
| OpenAI | `http://127.0.0.1:<port>/openai/v1` |
| OpenRouter | `http://127.0.0.1:<port>/openrouter/v1` |
| Ollama | `http://127.0.0.1:<port>/ollama/v1` |
| Anthropic | `http://127.0.0.1:<port>/anthropic` |

The Anthropic SDK appends `/v1/messages`; do not put `/v1` in its base URL.
If the existing URL differs from a preset, add a custom upstream to the
resolved YAML config. Remove only the API `/v1` suffix from its existing URL;
preserve any provider prefix. For example, `https://provider.example/api/v1`
becomes `https://provider.example/api`:

```yaml
upstreams:
  - name: project-provider
    base_url: https://provider.example/api
    api: openai
```

Use a non-conflicting name. Preserve other YAML settings and upstreams;
reuse an entry whose provider root and API already match. Use `api: anthropic`
for Messages. Clients use `/<name>/v1` for OpenAI-compatible APIs and
`/<name>` for Anthropic. Never put keys in the upstream config.

Custom upstreams are read at startup only. Prepare them before starting a new
instance. If someone else started a reused instance and it needs a restart,
ask before restarting or changing its running setup. You may relaunch an
instance this setup started, using its recorded settings.

## 5. Start only if needed

For a new instance, set `DATA_DIR` to the resolved instance data directory,
create it, then launch detached on Linux or macOS:

```sh
mkdir -p "$DATA_DIR"
nohup tabslip serve </dev/null >"$DATA_DIR/serve.log" 2>&1 &
TABSLIP_PID=$!
```

Append explicit `--config` / `--data-dir` flags to `serve` only for selected
flag overrides. Environment-selected config is inherited. Record the PID,
config path and data path, and the full restart command with required env.
Do not rely on an interactive terminal staying open. Wait for
`curl -fsS "http://127.0.0.1:<port>/healthz"` to succeed, then rerun doctor to
confirm the identity. If startup fails, read the diagnostics and report it.

On Windows PowerShell, create `$DataDir` with `New-Item -ItemType Directory
-Force $DataDir`, then use `Start-Process` with the resolved executable path:

```powershell
$Process = Start-Process -FilePath (Get-Command tabslip).Source -ArgumentList 'serve' -WindowStyle Hidden -RedirectStandardOutput "$DataDir\serve.log" -RedirectStandardError "$DataDir\serve-error.log" -PassThru
$Process.Id
```

Add selected flag overrides as separate `-ArgumentList` elements. Record the
PID and settings. Wait for `Invoke-RestMethod http://127.0.0.1:<port>/healthz`.
For a process you started, stop with `kill -TERM <recorded-pid>` on Unix, or
`Stop-Process -Id <recorded-pid>` on Windows. Unix SIGTERM drains pending
receipts; Windows process termination is abrupt, so finish calls first.
Check that the PID still belongs to this instance before stopping it.
Identify a reused server as pre-existing and do not offer an unowned PID to kill.

## 6. Save the routing and make one call

Use the project's normal environment configuration or a small reversible SDK
option. Keep a default that can be restored. Persist the setting or code in
the project; an export confined to your shell is insufficient. Document how
to load any saved configuration from a fresh shell without a new routing
override. Keep keys in their existing source. Add a lasting project/client tag
only if requested.

Tell the user the check call may cost money. The setup prompt authorizes one
provider call, so do not ask again. Use the existing project command or one
minimal request with its own client and key. If there is no usable key, stop
real-provider setup and offer `tabslip demo` as a simulated alternative.
Do not use credentials from another project.

For this call only, set a unique `X-Tabslip-Tag`, such as
`setup-check-<random>`. Use the SDK's per-request header option or a temporary
setting. Restore the previous tag afterward. Do not send another call to
compensate for a missing receipt without diagnosing it first.

## 7. Find the receipt and explain what changed

Poll for up to 30 seconds, using the selected instance directory:

```sh
tabslip receipts --tag <tag> --data-dir <instance-dir> --json
```

Wait a second between empty results. Once a matching receipt arrives, run:

```sh
tabslip show <receipt-id> --data-dir <instance-dir>
```

Confirm the intended upstream, successful call and expected response. A
failed-call receipt alone is not success. Give the receipt id and local page
URL, including `/r/<receipt-id>`. If no match arrives, report incomplete setup
and use doctor to diagnose health, config, routing and recording failures.
Do not claim success when a required check failed.

Read the running `/setup` page (local HTTP is sufficient) and its content
header. Report the actual content state, including per-tag storage. A reused
instance may already store content. Never enable storage unless asked, and
do not change existing content settings.

Finish with the saved env/code/config changes, the data directory and process
ownership. Give exact undo steps, including each changed default and custom
upstream entry. Preserve pre-existing settings when undoing. Explain how to
restart the instance with its recorded settings, or undo routing first.
While the app points at Tabslip, calls fail if it stops, including after
reboot. This guide does not install a service manager or edit unrelated code.
Reporting changes, optional tags and the authorized call does not require a
reply. Ask only when needed for missing project/key information, instance
identity or restart authority, or the host project's own rules.

## Containers and servers

On Linux, an app container can use `--network host` with the loopback URLs
above. With bridge networking, add `--add-host=host.docker.internal:host-gateway`
and use `http://host.docker.internal:<port>/<upstream>/v1` (omit `/v1` for
Anthropic). The hostname alone is insufficient: Tabslip must listen on an
address reachable from the container, such as the host bridge address or
`--listen 0.0.0.0`. Confirm the route from inside the container. Ask before
changing someone else's running instance.

The reachable proxy forwards credentials without added authentication. Limit
access with host networking rules. The web page stays loopback-only even when
the proxy listens elsewhere. See [upstream guidance](docs/upstreams.md).

Tabslip can run beside an app on a server. Point that app at the server's
local gateway, and view its page through SSH:

```sh
ssh -N -L 4545:127.0.0.1:4545 user@server
```

Open `http://127.0.0.1:4545` locally. The local port must equal the remote
Tabslip port because the page checks the port in the browser's Host header.
Use the selected port on both sides if it differs from 4545, and in the local
page URL. That local port must be free. Stop the tunnel with Ctrl+C.

## Costs, storage and stopping

Tabslip has no usage charge; your provider still charges for its API calls.
The offline demo uses simulated calls and needs no key. By default receipts
keep metadata and hashes, with no prompt or answer text. Optional content
storage stays local; exports exclude it. Signatures show records have not
changed since your Tabslip signed them. Provider model names and usage
remain the provider's claims.

Stop a foreground server with Ctrl+C. For a detached server you own, use the
recorded PID as described above. Restore the app's original base URL before
stopping if the app must keep working. Keep the database and its signing key
together; stopping does not delete them.
