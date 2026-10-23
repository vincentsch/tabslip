# Storage and privacy

Tabslip keeps records on your machine. By default these contain metadata
and hashes, without prompt or answer text. There is no telemetry.

## What is stored

Receipts keep metadata about each call, including the model name and usage. They
also keep SHA-256 hashes of the bodies. The [receipt fields](receipts.md) page
lists what they contain. This metadata can reveal information about your work
even without content. A hash can be compared with a known body; it is not
encryption. Headers containing credentials never enter the database or logs.
Provider error messages are excluded from receipt metadata; error types and
codes can remain.

Optional content storage uses a separate local table. Enable it in Setup
for all calls or selected tags, or start with `--keep-content`. The header
shows "Content: not stored", "Content: stored for all calls", or the selected
tags. Changes apply to calls that start afterward. A startup flag locks its
corresponding page setting. Stored request and response content is capped
at 10 MiB per body and may include sensitive text, including provider errors.
Streaming response content is assembled answer text; JSON content is the raw body.

Receipt downloads, signed bundles, CSV, JSONL and HTML exports never contain
stored prompt or answer text. Content is readable only through the local page.

## Files and retention

| OS | Default config | Default data directory |
| --- | --- | --- |
| Linux | `$XDG_CONFIG_HOME/tabslip/config.yaml`, else `~/.config/tabslip/config.yaml` | `$XDG_DATA_HOME/tabslip`, else `~/.local/share/tabslip` |
| macOS | `~/Library/Application Support/tabslip/config.yaml` | `~/Library/Application Support/tabslip` |
| Windows | `%AppData%\tabslip\config.yaml` | `%LocalAppData%\tabslip` |

Config selection is `--config`, then `TABSLIP_CONFIG`, then the OS default. A
missing selected file uses defaults and produces a warning in `serve` or a
`config` warning in doctor. An absent OS-default file is normal on first run.
`--data-dir` overrides the data location. The database is `tabslip.db`; SQLite
also creates WAL files while running. The signing keys are `key.ed25519` and
`key.pub`. Downloaded price snapshots are kept in the same directory. Keep the
private key and database together in backups. Read-only commands do not create
state or apply retention.

Retention defaults to 90 days. It deletes old receipts, supplements and
content at startup and daily. Saving retention in Setup applies it immediately;
`retention_days: 0` keeps records indefinitely. Delete on a receipt page removes
that call and related rows from this instance, without affecting exported
copies. Database row deletion is not a secure disk erasure guarantee.

The demo uses a separate temporary directory and embedded prices. Ctrl+C
removes temporary demo data; an explicit demo data directory remains.

## Network contacts

Calls contact the configured upstream with the client's credentials.
OpenRouter generation lookups use the same authorization in memory after
the call; disable them with `enrich_openrouter: false`. Doctor sends key-free
GETs to upstream roots, with five-second timeouts, and checks loopback health.
Any HTTP answer establishes reachability, without validating credentials.

`tabslip prices update` downloads LiteLLM's public price file explicitly.
Server startup and ordinary read commands do not download prices. The shell
installer downloads binaries and checksums from GitHub. The page has no CDN,
web fonts or external resource requests, and the demo makes no external requests.

Pages, downloads, events and settings accept loopback connections and local
Host headers only. State changes require a same-origin POST and page token.
Proxy routes follow `--listen`; they forward credentials without added
proxy authentication. See [network access](upstreams.md) and the
[command reference](reference/index.md).
