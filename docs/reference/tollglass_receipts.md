# tollglass receipts

List recent receipts, newest first

## Usage

```
tollglass receipts
```

## Examples

```
tollglass receipts
tollglass receipts --tag acme-shop --since 2026-10-01 --json
```

## Flags

- `--limit` Maximum number of receipts (0 for all)
- `--model` Select the requested or reported model exactly
- `--since` Include calls from this date (RFC 3339 or YYYY-MM-DD, UTC)
- `--tag` Select a tag exactly
- `--until` Exclude calls from this date onward
- `--upstream` Select an upstream exactly

## Output modes

human, json

## Related commands

- tollglass receipts
- tollglass export
- tollglass verify
