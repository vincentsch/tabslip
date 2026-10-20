# Verify receipts offline

Verification checks signatures and signer identity without contacting a
provider. It also explains what the signed records can tell you.

## Export a bundle

Use the source instance's data directory. Here `./instance-data` is an
illustrative directory; replace it with the real directory, including the
printed directory when using the demo. Output files must not exist.

```sh
tollglass export --data-dir ./instance-data --out calls.tollglass.json
tollglass key --data-dir ./instance-data
tollglass key export --data-dir ./instance-data --out tollglass.pub
```

Export previews the count, range, tags, models, upstreams and excluded content.
Add `--dry-run` to preview without writing. Filters include `--tag`, `--model`,
`--upstream`, `--since` and `--until`; dates accept RFC 3339 or YYYY-MM-DD
at UTC midnight. Since is inclusive; until is exclusive. No bounds selects
all retained receipts. Reads work while running and after shutdown and create no state.

Bundles contain original DSSE envelopes (signed payloads and signatures),
signed supplements and a convenient copy of the public key. The embedded key
alone does not establish who signed. Confirm the public key, fingerprint or
key id with the sender through a separate channel.

## Check the signer

After confirming `tollglass.pub` independently:

```sh
tollglass verify calls.tollglass.json --key tollglass.pub
```

Alternatively use `--key-id` with the independently confirmed `tg_…` id.
Replace the illustrative id rather than copying it literally. The key id is
derived from the actual public key, not trusted from a label. For a bundle,
the embedded public key supplies the bytes when only its id is confirmed.
Without flags, an existing matching local key can confirm the signer.
Explicit `--key` or `--key-id` takes precedence over the local key, and the
two flags are alternatives. Select `--data-dir` when comparing a particular local key.

A single receipt download is a DSSE envelope, not a bundle. It needs the
public key bytes through `--key`, even when you know the key id:

```sh
tollglass verify receipt.json --key tollglass.pub --json
```

For example, download `receipt.json` from a receipt's local page, or save
`tollglass show <receipt-id> --data-dir ./instance-data --json` to that file,
replacing `<receipt-id>` with a listed id. Single envelopes do not include supplements.

## The three answers

The signature result counts valid receipts and supplements or identifies
invalid records. Supplement digests must bind to a receipt in the bundle;
changed payloads, wrong keys and inconsistent signer labels are invalid.
The signer result says whether an independently selected or existing matching
local key confirms the signer. The claims answer says model names, token
counts and provider cost come from the provider; Tollglass did not check the model itself.

| Exit | Meaning |
| --- | --- |
| 0 | Valid signatures and confirmed signer |
| 10 | At least one invalid signature or record |
| 11 | Valid signatures, signer unconfirmed or mismatched |
| 1 | Usage error |
| 2 | Runtime error |
| 5 | File not found |

JSON provides the same three answers and individual envelope results.
A signature shows the record has not changed since your Tollglass signed it.
It does not show the provider told the truth or stop the key owner inventing a record.

## Other export formats

`--format csv` and `--format jsonl` are unsigned exports. JSONL contains one
decoded `{receipt, supplements}` object per line. HTML is a printable statement
with totals, unknown-cost coverage and a compact call table, without individual
receipt SVGs. These formats combine supplement cost once; they cannot be
verified as signed bundles. No export includes local content.
See the [command reference](reference/index.md).
