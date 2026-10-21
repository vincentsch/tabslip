# tabslip export

Export content-free receipts and spending statements

## Usage

```
tabslip export
```

## Examples

```
tabslip export --out calls.tabslip.json
tabslip export --format html --tag acme-shop --out statement.html --dry-run
```

## Flags

- `--format` bundle, jsonl, csv or html
- `--model` Select the requested or reported model exactly
- `--out` New output file (required)
- `--since` Include calls from this date (RFC 3339 or YYYY-MM-DD, UTC)
- `--tag` Select a tag exactly
- `--until` Exclude calls from this date onward
- `--upstream` Select an upstream exactly

## Output modes

human, json

## Changes state

This command changes state and honors `--dry-run`.

## Related commands

- tabslip receipts
- tabslip export
- tabslip verify
