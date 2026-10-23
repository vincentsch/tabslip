# Costs

Cost is reported, estimated or unknown. A provider's reported zero is zero;
missing cost is unknown. Reported cost wins over an estimate, which stays
in the signed receipt for comparison.

## Estimates

Direct OpenAI and Anthropic calls can use the embedded LiteLLM table. Tabslip
looks up the reported model first, then the requested model. If neither matches,
it tries names without a date suffix. It only uses prices listed for that
provider. The receipt records the table version even when lookup misses.
OpenRouter, Ollama and custom upstreams have no price-table estimates; they need
reported cost.

OpenAI cached input is subtracted from ordinary input and charged once.
Reasoning tokens are already part of output. Anthropic signed input is the
uncached count. Cache reads and writes use their own rates; one-hour writes
are part of total writes, never an extra category to add twice. Missing cache
rates fall back to the applicable ordinary or cache-write rate. Embeddings
use input alone. Missing required usage or prices leave the estimate unknown.
Calculations use exact decimals, rounded half-up to twelve fractional digits.

OpenRouter generation details arrive as a separate signed supplement.
Its cost and non-null provider can replace the displayed summary, while the
signed base receipt stays unchanged. Spend and unsigned exports count that
cost once. Totals show reported and estimated subtotals and the number of
calls with unknown cost. A group with only unknown costs displays unknown.
Spend date windows use UTC midnight.

## Price tables

Replace `./instance-data` with your instance's data directory:

```sh
tabslip prices --data-dir ./instance-data
tabslip prices update --data-dir ./instance-data --dry-run
```

The first command shows the selected table's date and SHA-256. The second
previews an update without a download or file change. To download explicitly,
remove `--dry-run`. The source is LiteLLM's
`model_prices_and_context_window.json` on its main branch. Tabslip checks the
downloaded prices before replacing the saved table. A failed update keeps the
old table. Restart the gateway to load it. A running gateway retains its startup
table, and already signed costs never change. Demo mode always uses embedded
prices.

The embedded snapshot comes from LiteLLM revision
`4818b767d8b4c4e5fa9885ab953d6f5cfe7c7ca5`. Developers can reproduce it with
`go generate ./internal/prices`, which downloads that pinned revision.
See [prices commands](reference/tabslip_prices.md) and
[receipt fields](receipts.md).
