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

- `--format` bundle, jsonl, csv or html. Type: `string`. Default: `bundle`.
- `--model` Select the requested or reported model exactly. Type: `string`. Default: unset (empty).
- `--out` New output file (required). Type: `string`. Default: unset (empty).
- `--since` Include calls from this date (RFC 3339 or YYYY-MM-DD, UTC). Type: `string`. Default: unset (empty).
- `--tag` Select a tag exactly. Type: `string`. Default: unset (empty).
- `--until` Exclude calls from this date onward. Type: `string`. Default: unset (empty).
- `--upstream` Select an upstream exactly. Type: `string`. Default: unset (empty).

## Output

Shows text you can read in your terminal.

Use --json to get JSON for scripts and other programs.

## Preview changes

This command makes changes. Use `--dry-run` to see what would change without changing anything.

## Related commands

- tabslip receipts
- tabslip export
- tabslip verify

## Shared flags

See [global flags](index.md#global-flags) for shared options, types and defaults.
