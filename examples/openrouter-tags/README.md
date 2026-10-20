# One tagged OpenRouter chat request

Run from this directory. Python 3.10+ and the SDK in requirements.txt are required.
See [tested dependencies](../README.md#tested-dependencies).

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
```

## Without a provider key

Start `tollglass demo` in another terminal. Copy its printed OpenAI base URL
when prompted below. The demo chooses a free port; use `/demo/v1`.

```sh
printf 'Printed OpenAI base URL: '
read -r OPENROUTER_BASE_URL
export OPENROUTER_BASE_URL
export OPENROUTER_API_KEY=demo
TOLLGLASS_TAG=example-check .venv/bin/python example.py
```

Copy the printed demo data directory to query `tollglass receipts --tag
example-check --data-dir <directory> --json`, replacing `<directory>` with
that path. A new tagged receipt confirms the call; the eight seeded calls do not.
Use a fresh tag when repeating this check.

## With your provider

Start `tollglass serve`. Use your project's existing key from its environment
or secret manager. These calls may cost money. Save the routing in your normal
project configuration, for example by copying `.env.example` to `.env`, replacing
the key placeholder and keeping it out of Git. The scripts do not load dotenv
automatically; from a fresh POSIX shell load your local settings with:

```sh
set -a
. ./.env
set +a
TOLLGLASS_TAG=example-check .venv/bin/python example.py
```

The example base URL is `http://127.0.0.1:4545/openrouter/v1`. Keep your existing provider and choose a
model it supports with `OPENROUTER_MODEL`.
For demo runs, the printed URL and synthetic key replace those live settings.

This uses a lasting client tag through `default_headers`. Against the demo,
only chat and tag behaviour is exercised; live OpenRouter routing and generation
details remain simulated. Against the OpenRouter preset, generation details
can arrive as a signed supplement.

On Windows use `.venv\Scripts\python.exe` in place of `.venv/bin/python`.
Restore the original provider base URL before stopping Tollglass to keep live
calls working. For agent setup, use [the shared prompt](../../docs/setup-with-an-agent.md).
