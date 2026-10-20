# Set up Tollglass with your coding agent

Start your agent inside the project that already calls an AI API and paste:

```text
Set up Tollglass for this project.
Use https://github.com/vincentsch/tollglass: read its AGENTS.md and follow
the steps. Route this project's AI API calls through it, make one real call,
and show me the receipt.
```

The agent follows [AGENTS.md](../AGENTS.md). It finds the client creation
sites and reports the settings it will change before editing. If the project
has no AI calls, it stops. The guide lists client patterns to help find calls.
See [examples](../examples/README.md) for runnable clients.

## What changes

The agent installs the binary only when missing and records its resolved path.
It locates the normal OS instance, runs doctor, and compares the running
server's key id with the selected database's identity before reuse. A different
identity needs the running instance's data directory, rather than a new key.
A port owned by another program stays running; the selected free port is
saved in the resolved YAML config.

Your project's normal environment file or client option gets the local base
URL for its existing provider. A custom URL needs a matching upstream entry.
Keys stay in their current source and pass through to the same provider.
No key belongs in Tollglass YAML. A fresh shell must load the saved project
configuration; a temporary export alone is insufficient.

The prompt authorizes one call that may cost money. The agent uses a unique
request-only tag, checks the successful response and its new receipt, and
reports its id and `/r/<id>` page. A failed-call receipt is not successful
setup. Without a usable project key, use the simulated `tollglass demo` instead.

## Reuse, stopping and undo

The completion report gives the executable, config and data paths, changed
files, content settings, process ownership and restart command. Existing
content settings stay as they were. Restarting someone else's server requires
the owner's permission; custom upstream entries load only at startup.

To undo, restore each saved base URL or remove the SDK override to recover
its previous default. Remove only custom upstream entries this setup added,
preserving other YAML settings. Then stop only a process the agent started,
using its recorded PID after checking it still belongs to that instance.
SIGTERM on Unix drains queued receipts. Keep the database and signing key together.

While the app points at Tollglass, calls fail when it stops, including after
reboot. The setup does not install a service manager. Use the recorded restart
command with the same config environment, explicit flags and data directory.
See [upstreams](upstreams.md) for Docker and remote-page access, and the
[command reference](reference/index.md) for flags.

See the [quickstart](quickstart.md) and [FAQ](faq.md) for getting started and common questions.
