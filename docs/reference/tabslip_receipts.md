# tabslip receipts

List recent receipts, newest first

## Usage

```
tabslip receipts
```

## Examples

```
tabslip receipts
tabslip receipts --tag acme-shop --since 2026-10-01 --json
```

## Flags

- `--limit` Maximum number of receipts (0 for all). Type: `int`. Default: `20`.
- `--model` Select the requested or reported model exactly. Type: `string`. Default: unset (empty).
- `--since` Include calls from this date (RFC 3339 or YYYY-MM-DD, UTC). Type: `string`. Default: unset (empty).
- `--tag` Select a tag exactly. Type: `string`. Default: unset (empty).
- `--until` Exclude calls from this date onward. Type: `string`. Default: unset (empty).
- `--upstream` Select an upstream exactly. Type: `string`. Default: unset (empty).

## Output modes

human, json

## Related commands

- tabslip receipts
- tabslip export
- tabslip verify

## Shared flags

See [global flags](index.md#global-flags) for shared options, types and defaults.
