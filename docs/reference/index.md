# tabslip command reference

Keep signed receipts for AI API calls

## Commands

- [tabslip demo](tabslip_demo.md) - Try eight simulated AI calls offline
- [tabslip doctor](tabslip_doctor.md) - Check local setup and upstream reachability
- [tabslip export](tabslip_export.md) - Export content-free receipts and spending statements
- [tabslip key](tabslip_key.md) - Show the existing instance public key
- [tabslip key export](tabslip_key_export.md) - Export the existing PEM public key
- [tabslip prices](tabslip_prices.md) - Show the selected local price table
- [tabslip prices update](tabslip_prices_update.md) - Download and install a LiteLLM price snapshot
- [tabslip receipts](tabslip_receipts.md) - List recent receipts, newest first
- [tabslip serve](tabslip_serve.md) - Start the local AI gateway
- [tabslip show](tabslip_show.md) - Show one signed receipt
- [tabslip verify](tabslip_verify.md) - Check signatures and signer identity offline

## Global flags

- `--config` Path to the config file
- `--data-dir` Directory for local receipts and the instance key
- `--dry-run` Preview changes without performing them
- `--include-meta` Wrap machine output as {data, meta} (commands that expose request metadata)
- `--jq` Transform stable JSON output with a jq expression (commands with machine output)
- `--json` Output stable JSON instead of the human view
- `--keep-content` Keep request and answer content locally
- `--listen` Address to listen on
- `--no-ansi` Disable all ANSI/control sequences in human output
- `--no-color` Disable color in human output
- `--no-pager` Never use a pager for long human output
- `--no-prompt` Never block on an interactive prompt
- `--open` Open the local page in your browser
- `--plain` Print unstyled, copy-safe text (commands with human output)
- `--port` Port to listen on
- `--quiet` Suppress non-essential output
- `--template` Render stable JSON output through a Go text/template (commands with machine output)
