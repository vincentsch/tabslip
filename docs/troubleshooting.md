# Troubleshooting

Run doctor with the same config and data directory as your server. For
example, replace these illustrative paths with the selected instance paths:

```sh
tabslip doctor --config ./config.yaml --data-dir ./instance-data --json
```

Doctor reports `ok`, `info`, `warn` and `fail`. Exit 2 means a failed check.
Unreachable presets alone are warnings. It does not create an identity or
repair files, and an HTTP answer does not validate credentials.

## Config and storage

If a selected config file is missing, Tabslip names it and uses defaults.
Correct `--config` or `TABSLIP_CONFIG`, or create the selected YAML file.
Doctor reports this as a warning; it does not fail solely for a missing file.

| Doctor message | What to do |
| --- | --- |
| `config file is malformed; check its YAML settings` | Correct the YAML in the selected config; check `--config` and `TABSLIP_CONFIG`. |
| `Data location is not writable.` | Correct permissions or select a writable `--data-dir`; check disk space too. |
| `Key and database are not created yet. Start Tabslip when ready.` | Normal before first start; no repair is needed. |
| `Public key is missing or invalid.` | Restore the original pair with the database, or select a new data directory. |
| `Cannot read the database.` | Restore the database or select a new directory; retain the original for recovery. |
| `Database and instance key do not match.` | Restore the matching signing key. Never create a replacement identity over existing receipts. |

Keep `key.ed25519`, `key.pub` and the database together. Do not delete them to
silence a diagnostic. A running instance's port check names its key id;
compare it with `Database and key pair match: ts_…`. A mismatch means the
selected data directory may belong to another instance.

## Port and upstream access

`Cannot use port 4545; another program may be listening.` means the selected
address could not be bound and no matching Tabslip health response was found.
Leave the other process running. Select a free port and save it in your
resolved config, preserving the other settings:

```yaml
port: 4546
```

Rerun doctor without a temporary port flag, then use that port in your app's
base URL and the browser. `No HTTP answer within the reachability check.`
means the upstream timed out or could not be contacted. Check its root URL
and network; start Ollama if needed. Custom upstream failures need fixing;
presets you do not use can remain unreachable. `HTTP 401 received. Credentials
were not checked.` still means reachable. Use the project's key for a separate
call to test authorization, with the provider's normal cost.

## A call has no receipt

Check the client's persisted base URL from a fresh shell. Supported capture
paths are listed in [upstreams](upstreams.md); other paths forward without
receipts. Demo paths are `/demo/v1` and `/demo` at the printed free port.
Clear page filters and use `receipts --data-dir ./instance-data --limit 0`
to check the same database. A unique `X-Tabslip-Tag` helps match a call;
wait up to 30 seconds for recording before retrying.

Check `/healthz` on the selected port and rerun doctor. The
`unrecorded_calls` counter and doctor's `N calls could not be recorded.`
mean a full recording queue or failed write. Inspect server diagnostics,
free disk space and fix directory permissions before sending more calls.
Forwarding may succeed while recording fails. The counter resets on restart;
do not treat a restart as recovery of missing records.

Unknown usage means the provider omitted it or metadata could not be parsed.
It does not mean zero. A stream that ends before its terminal event is marked
incomplete. See [findings](receipts.md#findings). If an OpenRouter supplement
is absent, its lookup may have failed; the base receipt stays usable.

## The app fails after Tabslip stops

Restart with the recorded config, data directory and required environment,
or restore the original provider URL first. Installation does not add a
service manager. For container reachability, remote pages and Host-header
checks, see [upstreams](upstreams.md). For command flags, use the
[command reference](reference/index.md).
