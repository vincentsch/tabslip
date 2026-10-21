# One Anthropic Messages request

Run from this directory. Python 3.10+ and the SDK in requirements.txt are required.
See [tested dependencies](../README.md#tested-dependencies).

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
```

## Without a provider key

Start `tabslip demo` in another terminal. Copy its printed Anthropic base URL
when prompted below. The demo chooses a free port; use `/demo`, without `/v1`.

```sh
printf 'Printed Anthropic base URL: '
read -r ANTHROPIC_BASE_URL
export ANTHROPIC_BASE_URL
export ANTHROPIC_API_KEY=demo
.venv/bin/python example.py --tag example-check
```

Copy the printed demo data directory to query `tabslip receipts --tag
example-check --data-dir <directory> --json`, replacing `<directory>` with
that path. A new tagged receipt confirms the call; the eight seeded calls do not.
Use a fresh tag when repeating this check.

## With your provider

Start `tabslip serve`. Use your project's existing key from its environment
or secret manager. These calls may cost money. Save the routing in your normal
project configuration, for example by copying `.env.example` to `.env`, replacing
the key placeholder and keeping it out of Git. The scripts do not load dotenv
automatically; from a fresh POSIX shell load your local settings with:

```sh
set -a
. ./.env
set +a
.venv/bin/python example.py --tag example-check
```

The example base URL is `http://127.0.0.1:4545/anthropic`. Keep your existing provider and choose a
model it supports with `ANTHROPIC_MODEL`.
For demo runs, the printed URL and synthetic key replace those live settings.

On Windows use `.venv\Scripts\python.exe` in place of `.venv/bin/python`.
Restore the original provider base URL before stopping Tabslip to keep live
calls working. For agent setup, use [the shared prompt](../../docs/setup-with-an-agent.md).
