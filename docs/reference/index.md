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

- `--config` Path to the config file. Type: `string`. Default: `TABSLIP_CONFIG`, then the OS config directory: Linux `$XDG_CONFIG_HOME/tabslip/config.yaml` or `~/.config/tabslip/config.yaml`; macOS `~/Library/Application Support/tabslip/config.yaml`; Windows `%AppData%\tabslip\config.yaml`.
- `--data-dir` Directory for local receipts and the instance key. Type: `string`. Default: Linux `$XDG_DATA_HOME/tabslip` or `~/.local/share/tabslip`; macOS `~/Library/Application Support/tabslip`; Windows `%LocalAppData%\tabslip`.
- `--dry-run` Preview changes without performing them. Type: `bool`. Default: `false`.
- `--include-meta` Wrap machine output as {data, meta} (commands that expose request metadata). Type: `bool`. Default: `false`.
- `--jq` Transform stable JSON output with a jq expression (commands with machine output). Type: `string`. Default: unset (empty).
- `--json` Output stable JSON instead of the human view. Type: `bool`. Default: `false`.
- `--keep-content` Keep request and answer content locally. Type: `bool`. Default: `false`.
- `--listen` Address to listen on. Type: `string`. Default: `127.0.0.1`.
- `--no-ansi` Disable all ANSI/control sequences in human output. Type: `bool`. Default: `false`.
- `--no-color` Disable color in human output. Type: `bool`. Default: `false`.
- `--no-pager` Never use a pager for long human output. Type: `bool`. Default: `false`.
- `--no-prompt` Never block on an interactive prompt. Type: `bool`. Default: `false`.
- `--open` Open the local page in your browser. Type: `bool`. Default: `false`.
- `--plain` Print unstyled, copy-safe text (commands with human output). Type: `bool`. Default: `false`.
- `--port` Port to listen on. Type: `int`. Default: `4545`.
- `--quiet` Suppress non-essential output. Type: `bool`. Default: `false`.
- `--template` Render stable JSON output through a Go text/template (commands with machine output). Type: `string`. Default: unset (empty).
