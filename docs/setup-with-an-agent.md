# Set up Tabslip with your coding agent

Start your agent inside the project that already calls an AI API and paste:

```text
Set up Tabslip for this project.
Use https://github.com/vincentsch/tabslip: read its AGENTS.md and follow
the steps. Route this project's AI API calls through it, make one real call,
and show me the receipt.
```

The agent follows [AGENTS.md](../AGENTS.md). It finds where your project creates
its AI client and reports the settings it will change before editing. If the
project has no AI calls, it stops. The guide lists client patterns to help find
calls. See [examples](../examples/README.md) for runnable clients.

## What changes

The agent installs the binary only when missing and records its resolved path.
It runs `tabslip doctor` to find your existing setup. Before reusing a running
server, it checks that the server and database use the same key. If they differ,
it asks for that server's data directory. A port owned by another program stays
running; the selected free port is saved in the resolved YAML config.

Your project's normal environment file or client option gets the local base
URL for its existing provider. A custom URL needs a matching upstream entry.
Keys stay in their current source and pass through to the same provider.
No key belongs in Tabslip YAML. A fresh shell must load the saved project
configuration; a temporary export alone is insufficient.

The prompt authorizes one call that may cost money. The agent adds a unique tag
to that request to find its receipt. It checks that the call succeeded, then
reports the receipt id and `/r/<id>` page. A failed-call receipt is not
successful setup. Without a usable project key, use the simulated `tabslip demo`
instead.

## Reuse, stopping and undo

The agent reports which files it changed and how to restart Tabslip. It also
records where Tabslip is installed and which config and data paths it uses. The
report includes the content settings and says who started the process. Existing
content settings stay as they were. Restarting someone else's server requires
the owner's permission; custom upstream entries load only at startup.

To undo, restore each saved base URL or remove the SDK override to recover
its previous default. Remove only custom upstream entries this setup added,
preserving other YAML settings. Then stop only a process the agent started,
using its recorded PID after checking it still belongs to that instance.
SIGTERM on Unix drains queued receipts. Keep the database and signing key together.

While the app points at Tabslip, calls fail when it stops, including after
reboot. The setup does not install a service manager. Use the recorded restart
command with the same config environment, explicit flags and data directory.
See [upstreams](upstreams.md) for Docker and remote-page access, and the
[command reference](reference/index.md) for flags.
