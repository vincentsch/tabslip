# tabslip key export

Export the existing PEM public key

## Usage

```
tabslip key export
```

## Examples

```
tabslip key export --out tabslip.pub
tabslip key export --out tabslip.pub --dry-run
```

## Flags

- `--out` New PEM output file (required). Type: `string`. Default: unset (empty).

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
