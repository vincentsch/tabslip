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

- `--key` PEM public key confirmed through another channel. Type: `string`. Default: unset (empty).
- `--key-id` Expected signer key id. Type: `string`. Default: unset (empty).

## Output

Shows text you can read in your terminal.

Use --json to get JSON for scripts and other programs.

## Related commands

- tabslip receipts
- tabslip export
- tabslip verify

## Shared flags

See [global flags](index.md#global-flags) for shared options, types and defaults.
