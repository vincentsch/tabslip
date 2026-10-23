# Receipt fields

A receipt is JSON signed by your Tabslip. `show --json` returns an envelope that
holds the receipt and its signature, using the DSSE format. Decode the base64
`payload` to read the fields below. A download keeps the original signed bytes.
Replace `./instance-data` with the data directory Tabslip prints:

```sh
tabslip receipts --data-dir ./instance-data --limit 0 --json
```

Use a returned id with `tabslip show <receipt-id> --data-dir ./instance-data`
for a terminal receipt, or add `--json` for the envelope. Open `/r/<receipt-id>`
on the instance's Page URL for the paper view, signature results and details.

## Signed base fields

Nullable fields are JSON `null` when unknown. Zero means a reported or
measured zero, never a substitute for missing data. Hashes are SHA-256 hex.

| Field | Meaning |
| --- | --- |
| `type`, `id` | `tabslip.receipt/v1` and locally generated `rcpt_` id |
| `instance.key_id`, `instance.tabslip` | Signing key id and Tabslip version |
| `time.started`, `first_byte`, `finished` | Observed UTC timestamps; first byte can be null |
| `time.ttfb_ms`, `total_ms` | Observed milliseconds to first byte and total duration |
| `request.upstream`, `api`, `method`, `path` | Selected route, adapter, HTTP method and API path |
| `request.model`, `stream` | Requested model and streaming flag, nullable |
| `request.params` | Selected parameters, not messages or tool arguments |
| `request.tag` | Optional validated client tag; omitted when absent |
| `request.body_sha256`, `body_bytes` | Original client body hash and byte count before edits |
| `request.modifications` | Recorded edits, such as usage injection |
| `response.status`, `complete` | Observed HTTP status (nullable) and capture completeness |
| `response.api_status` | Responses API terminal status, nullable |
| `response.id`, `model`, `provider` | Provider-reported identifiers, nullable |
| `response.finish_reason`, `system_fingerprint` | Provider-reported fields, nullable |
| `response.error` | Null or `{type, code}`; no provider error message |
| `response.body_sha256`, `body_bytes` | Observed response bytes hash and size |
| `usage.source` | `reported` or `unknown` |
| `usage.input_tokens`, `output_tokens` | Reported input/output; embeddings have no output count |
| `usage.cached_input_tokens`, `cache_write_tokens` | Cache reads and total cache writes, nullable |
| `usage.cache_write_1h_tokens` | One-hour subset of cache writes, nullable |
| `usage.reasoning_tokens` | Reported reasoning subset of output, nullable |
| `cost.source`, `amount`, `currency` | `reported`, `estimated` or `unknown`; decimal string and currency, nullable |
| `cost.estimate`, `price_table` | Estimate retained alongside reported cost and attempted table version, nullable |
| `findings` | Array of `{code, level, text}` |

The `request.params` object keeps selected generation settings, such as
`temperature` and token limits. It can also keep routing options and the number
of tools. It excludes messages and tool arguments.

Tabslip hashes the body bytes it receives. For streams, it hashes the forwarded
bytes rather than reconstructing answer text. Metadata parsing is capped at 10
MiB per JSON value or SSE event; forwarding and hashing continue when metadata
cannot be decoded.

For Anthropic, signed `input_tokens` is uncached input. Displays add cache
reads and total cache writes. If any category is unknown, the displayed input
total is unknown. One-hour writes are already inside total writes. Signed
JSON and decoded JSONL keep the raw categories used for pricing.
`response.complete` describes capture, not successful model output: fully
received Responses `incomplete` and `failed` events can be complete captures.

## JSON example

This made-up OpenRouter example shows decoded JSON before supplemental cost
arrives. It has no signature. A stored receipt has full ids and hashes where
this example uses shortened values:

```json
{
  "type": "tabslip.receipt/v1",
  "id": "rcpt_…",
  "instance": {"key_id": "ts_…", "tabslip": "dev"},
  "time": {"started": "2026-10-21T09:00:00.000Z", "first_byte": "2026-10-21T09:00:00.030Z", "finished": "2026-10-21T09:00:00.050Z", "ttfb_ms": 30, "total_ms": 50},
  "request": {"upstream": "openrouter", "api": "openai.chat", "method": "POST", "path": "/v1/chat/completions", "model": "openai/gpt-4.1", "stream": false, "params": {}, "tag": "example", "body_sha256": "…", "body_bytes": 74, "modifications": []},
  "response": {"status": 200, "complete": true, "api_status": null, "id": "gen-example-unpriced", "model": "openai/gpt-4.1", "provider": null, "finish_reason": "stop", "system_fingerprint": null, "error": null, "body_sha256": "…", "body_bytes": 300},
  "usage": {"source": "reported", "input_tokens": 12, "output_tokens": 4, "cached_input_tokens": 0, "cache_write_tokens": null, "cache_write_1h_tokens": null, "reasoning_tokens": null},
  "cost": {"source": "unknown", "amount": null, "currency": null, "estimate": null, "price_table": null},
  "findings": [{"code": "price_unknown", "level": "observed", "text": "The cost is unknown."}]
}
```

## Findings

| Code | Level | Meaning |
| --- | --- | --- |
| `model_alias_resolved` | reported | A dated version of the requested model; informational, does not activate the findings filter |
| `model_differs` | reported | Requested and reported model differ |
| `fallback_used` | reported | Provider reports a later fallback model or multiple attempts |
| `usage_missing` | observed | No reported usage |
| `price_unknown` | observed | No reported cost or usable estimate |
| `stream_interrupted` | observed | Stream lacks its terminal event or client disconnected |
| `upstream_error` | observed | HTTP error or provider error event/object |
| `transport_error` | observed | Connection, TLS, read or early cancellation failure |
| `metadata_unavailable` | observed | Metadata could not be parsed within the limit |

Expected unknown cost on demo, Ollama and custom upstreams does not itself
produce `price_unknown`.

These describe reported values and observed failures; none checks the model itself.

## Signed supplements and displayed totals

OpenRouter can add a second DSSE envelope with payload type
`application/vnd.tabslip.supplement+json`. Its `type` is
`tabslip.supplement/v1`; `receipt_id` and `receipt_sha256` bind it to the
base payload. `source` is `openrouter.generation` and `fetched` is its time.
Nullable `provider`, `cost`, `data_region`, and `native_tokens.prompt` and
`completion` come from generation details. `currency` accompanies cost.
`attempts` lists reported provider/status pairs; `findings` carries additional
findings. Missing lists are empty. Provider details can arrive after the call.

The signed base receipt never changes. The page and unsigned exports count
supplement cost once. They keep the existing provider when the supplement's
provider is null.

When a supplement supplies a missing cost, `price_unknown` is marked resolved.
The original finding stays signed and visible. The receipt page lists findings
from the base receipt first, then from supplements. Each finding names its
signed source. Findings from different sources stay separate even when their
codes match. Resolved findings stay out of the Calls badge and findings filter.

The receipt page and the terminal `show` command verify the original receipt
and every attached supplement before showing SIGNED. An invalid receipt or
supplement produces INVALID with the failed check; `show` exits 10.
`show --json` still returns only the original receipt envelope. If the receipt
or any attached supplement fails verification, it prints diagnostics on stderr
and exits 10, even when the base envelope alone is valid.
Stored data that cannot be decoded is a read error (exit 2). A missing receipt
uses exit 5.
See [verification](verify.md), [costs](costs.md) and the
[command reference](reference/index.md).
