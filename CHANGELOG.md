## Unreleased

First release of Tollglass.

- Forward OpenAI, Anthropic, OpenRouter and Ollama API calls through a local gateway.
- Keep signed receipts for chat, Responses, embeddings and Messages, including streams.
- Record reported usage, exact costs, timing, body hashes and call findings.
- Add signed OpenRouter generation details without changing the original receipt.
- Browse Calls, receipts and Spend on a local page with live updates.
- Store metadata by default, with optional local content storage and retention settings.
- Export signed bundles or unsigned CSV, JSONL and printable HTML statements.
- Verify signatures offline and confirm the signer with a trusted key.
- Try eight simulated calls with the offline demo, without a provider key.
- Install checksum-checked binaries for Linux, macOS and Windows on amd64 and arm64.
- Include agent setup guidance, runnable examples and command documentation.
- Check builds and tests on three operating systems before publishing attested binaries.
