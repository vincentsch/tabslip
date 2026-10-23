# Commands

Tabslip starts the gateway with `tabslip` or `tabslip serve`.
Use `tabslip demo` to try simulated calls without a key. Commands print
human text by default; `--json` provides machine output and `--help` lists flags.

The [generated command index](reference/index.md) describes the installed
interface.

| Command | What it does |
| --- | --- |
| `receipts` | Lists recorded calls |
| `show` | Shows a receipt in the terminal |
| `export` | Writes records to a file |
| `verify` | Checks signatures offline |
| `key` | Shows the signer |
| `prices` | Shows the selected price table |
| `doctor` | Checks the setup |

Read-only commands never create a database or signing key. They do not save
price snapshots.

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

Each command page links to the [shared flags](reference/index.md#global-flags).
Flag entries show their types and defaults.
