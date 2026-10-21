# Runnable examples

These examples send one call through Tabslip using environment settings.
Each directory has install/run commands, a placeholder `.env.example`, a
key-free demo route and a separate provider configuration. Start
`tabslip demo`, then use its printed base URLs and data directory. The
OpenAI-style demo route ends in `/demo/v1`; Anthropic ends in `/demo`.
The synthetic key `demo` meets SDK requirements. No paid calls are needed.

| Example | Client | What it shows |
| --- | --- | --- |
| [Python OpenAI](python-openai/README.md) | Python openai | Chat, per-request tag |
| [Python Anthropic](python-anthropic/README.md) | Python anthropic | Messages, no `/v1` in SDK base URL |
| [Node OpenAI](node-openai/README.md) | Node openai | Chat, per-request header |
| [curl](curl/README.md) | POSIX shell and curl | Raw HTTP with a tag |
| [OpenRouter tags](openrouter-tags/README.md) | Python openai | A lasting client tag and response id |

Demo OpenRouter examples exercise chat and tags; provider routing and generation
details remain simulated. Live examples keep the project's provider keys and
may cost money. Never borrow credentials from another project.

## Tested dependencies

The versions below are recorded after running all examples and
`scripts/sdk-smoke.py` against the built binary with local fake providers.
They are tested versions, rather than a claim about all compatible SDK releases.

| Client | Exercised version |
| --- | --- |
| Python openai | 3.35.1 |
| Python anthropic | 1.16.0 |
| Node openai | 7.39.0 |
| Anthropic TypeScript (`@anthropic-ai/sdk`) | 0.136.0 |

Python cryptography 50.0.2 independently verifies SDK smoke-test
signatures; it is not required by the examples.

Python examples need Python 3.10+, Node needs Node.js 22+, and curl needs a POSIX
shell. `.env` files are optional local configuration, loaded explicitly with
`set -a; . ./.env; set +a` in the per-example instructions. Keep credentials
in the project's existing secret source in an application.

See [upstreams](../docs/upstreams.md) for endpoint coverage and
[commands](../docs/reference/index.md) for receipt queries.
