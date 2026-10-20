# tollglass verify

Check signatures and signer identity offline

## Usage

```
tollglass verify <file>
```

## Examples

```
tollglass verify calls.tollglass.json --key tollglass.pub
tollglass verify calls.tollglass.json --key-id tg_6f1c2a9b04d3e871 --json
```

## Flags

- `--key` PEM public key confirmed through another channel
- `--key-id` Expected signer key id

## Output modes

human, json

## Related commands

- tollglass receipts
- tollglass export
- tollglass verify
