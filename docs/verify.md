# Verify receipts offline

Verification checks signatures and signer identity without contacting a
provider. It also explains what the signed records can tell you.

## Export a bundle

Replace `./instance-data` with the source instance's data directory. The demo
prints its directory when it starts. Quote paths with spaces, for example
`--data-dir "./my instance"`. Export and key export refuse existing output
files; choose a new filename or move the earlier file first.

```sh
tabslip export --data-dir ./instance-data --out calls.tabslip.json
tabslip key --data-dir ./instance-data
tabslip key export --data-dir ./instance-data --out tabslip.pub
```

Export previews which calls it will include and which content it leaves out. Add
`--dry-run` to preview without writing. Filters include `--tag`, `--model`,
`--upstream`, `--since` and `--until`; dates accept RFC 3339 or YYYY-MM-DD at
UTC midnight. Since is inclusive; until is exclusive. No bounds selects all
retained receipts. Reads work while running and after shutdown and create no
state.

Bundles contain original DSSE envelopes (signed payloads and signatures),
signed supplements and a convenient copy of the public key. The embedded key
alone does not establish who signed. Confirm the public key, fingerprint or
key id with the sender through a separate channel.

## Check the signer

After confirming `tabslip.pub` independently:

```sh
tabslip verify calls.tabslip.json --key tabslip.pub
```

Alternatively use `--key-id` with the independently confirmed `ts_…` id.
Replace the illustrative id rather than copying it literally. The key id is
derived from the actual public key, not trusted from a label. For a bundle,
the embedded public key supplies the bytes when only its id is confirmed.
Without flags, an existing matching local key can confirm the signer.
Explicit `--key` or `--key-id` takes precedence over the local key, and the
two flags are alternatives. Select `--data-dir` when comparing a particular local key.

A single receipt download is a DSSE envelope, not a bundle. It needs the
public key bytes through `--key`, even when you know the key id:

```sh
tabslip verify receipt.json --key tabslip.pub --json
```

For example, download `receipt.json` from a receipt's local page, or save
`tabslip show <receipt-id> --data-dir ./instance-data --json` to that file,
replacing `<receipt-id>` with a listed id. Single envelopes do not include supplements.

## The three answers

The signature result counts valid receipts and supplements or identifies invalid
records. Supplement digests must bind to a receipt in the bundle; changed
payloads, wrong keys and inconsistent signer labels are invalid. The signer
result says whether an independently selected or existing matching local key
confirms the signer. The claims answer explains that model names and usage come
from the provider. Provider cost is also reported; Tabslip did not check the
model itself.

| Exit | Meaning |
| --- | --- |
| 0 | Valid signatures and confirmed signer |
| 10 | At least one invalid signature or record |
| 11 | Valid signatures, signer unconfirmed or mismatched |
| 1 | Usage error |
| 2 | Runtime error |
| 5 | File not found |

JSON provides the same three answers and individual envelope results.
A signature shows the record has not changed since your Tabslip signed it.
It does not show the provider told the truth or stop the key owner inventing a record.

## Check a local terminal receipt

`tabslip show <receipt-id> --data-dir ./instance-data` checks the receipt and
all attached supplements against the existing local public key. SIGNED means
all contributors pass signature, type, signer-label and receipt-link checks.
A failed check displays INVALID and returns exit 10 without changing records.

`show --json` returns only the original base envelope, even on exit 10.
Diagnostics on stderr identify the failed contributor. A damaged supplement
can therefore make `show --json` fail while its base envelope still verifies
on its own. Undecodable storage returns exit 2; a missing receipt returns 5.
These reads never create or repair instance state.

## Other export formats

`--format csv` and `--format jsonl` are unsigned exports. JSONL contains one
decoded `{receipt, supplements}` object per line. HTML is a printable statement
with totals, unknown-cost coverage and a compact call table, without individual
receipt SVGs. These formats combine supplement cost once; they cannot be
verified as signed bundles. No export includes local content.
See the [command reference](reference/index.md).
