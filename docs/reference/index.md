# tollglass command reference

Keep signed receipts for AI API calls

## Commands

- [tollglass demo](tollglass_demo.md) - Try eight simulated AI calls offline
- [tollglass doctor](tollglass_doctor.md) - Check local setup and upstream reachability
- [tollglass export](tollglass_export.md) - Export content-free receipts and spending statements
- [tollglass key](tollglass_key.md) - Show the existing instance public key
- [tollglass key export](tollglass_key_export.md) - Export the existing PEM public key
- [tollglass prices](tollglass_prices.md) - Show the selected local price table
- [tollglass prices update](tollglass_prices_update.md) - Download and install a LiteLLM price snapshot
- [tollglass receipts](tollglass_receipts.md) - List recent receipts, newest first
- [tollglass serve](tollglass_serve.md) - Start the local AI gateway
- [tollglass show](tollglass_show.md) - Show one signed receipt
- [tollglass verify](tollglass_verify.md) - Check signatures and signer identity offline

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
