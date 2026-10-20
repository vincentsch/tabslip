# Commands

Tollglass starts the gateway with `tollglass` or `tollglass serve`.
Use `tollglass demo` to try simulated calls without a key. Commands print
human text by default; `--json` provides machine output and `--help` lists flags.

The [generated command index](reference/index.md) describes the installed
interface. `receipts` lists calls, `show` opens a terminal receipt, `export`
writes records, `verify` checks signatures offline, `key` shows the signer,
`prices` shows the selected table, and `doctor` checks the setup.
Read-only commands never create keys, databases or price snapshots.

Choose a guide for the task:

- [Install and connect manually](quickstart.md)
- [Frequently asked questions](faq.md)
- [Set up with an agent](setup-with-an-agent.md)
- [Read receipt fields and findings](receipts.md)
- [Verify shared records](verify.md)
- [Understand storage and privacy](privacy.md)
- [Connect upstreams, containers and servers](upstreams.md)
- [Understand costs and update prices](costs.md)
- [Troubleshoot setup and recording](troubleshooting.md)
- [Run an example](../examples/README.md)

For source changes, `make docs` regenerates only `docs/reference/`.
`make docs-check` checks that generated copy without rewriting it.
