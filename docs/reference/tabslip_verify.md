# tabslip verify

Check signatures and signer identity offline

## Usage

```
tabslip verify <file>
```

## Examples

```
tabslip verify calls.tabslip.json --key tabslip.pub
tabslip verify calls.tabslip.json --key-id ts_6f1c2a9b04d3e871 --json
```

## Flags

- `--key` PEM public key confirmed through another channel
- `--key-id` Expected signer key id

## Output modes

human, json

## Related commands

- tabslip receipts
- tabslip export
- tabslip verify
