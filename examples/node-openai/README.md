# One OpenAI chat request

Run from this directory. Node.js 20+ and the SDK in package.json are required.
See [tested dependencies](../README.md#tested-dependencies).

```sh
npm install
```

## Without a provider key

Start `tollglass demo` in another terminal. Copy its printed OpenAI base URL
when prompted below. The demo chooses a free port; use `/demo/v1`.

```sh
printf 'Printed OpenAI base URL: '
read -r OPENAI_BASE_URL
export OPENAI_BASE_URL
export OPENAI_API_KEY=demo
TOLLGLASS_TAG=example-check npm start
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
TOLLGLASS_TAG=example-check npm start
```

The example base URL is `http://127.0.0.1:4545/openai/v1`. Keep your existing provider and choose a
model it supports with `OPENAI_MODEL`.
For demo runs, the printed URL and synthetic key replace those live settings.


Restore the original provider base URL before stopping Tollglass to keep live
calls working. For agent setup, use [the shared prompt](../../docs/setup-with-an-agent.md).
