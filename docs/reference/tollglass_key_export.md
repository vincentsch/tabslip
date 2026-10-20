# tollglass key export

Export the existing PEM public key

## Usage

```
tollglass key export
```

## Examples

```
tollglass key export --out tollglass.pub
tollglass key export --out tollglass.pub --dry-run
```

## Flags

- `--out` New PEM output file (required)

## Output modes

human, json

## Changes state

This command changes state and honors `--dry-run`.

## Related commands

- tollglass receipts
- tollglass export
- tollglass verify
