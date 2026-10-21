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

- `--out` New PEM output file (required)

## Output modes

human, json

## Changes state

This command changes state and honors `--dry-run`.

## Related commands

- tabslip receipts
- tabslip export
- tabslip verify
