# One OpenAI request

This example makes one chat completion request and prints the answer.
It uses `OPENAI_API_KEY` from your existing environment. `OPENAI_BASE_URL`
selects the provider URL; without it, the SDK uses `https://api.openai.com/v1`.
Use a provider that supports the `gpt-4.1` model, or change the model in
`example.py` to one your provider supports. The request may cost money.

From this directory, create a Python environment and install the SDK:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -r requirements.txt
.venv/bin/python example.py
```

On Windows use `.venv\Scripts\python.exe` instead of `.venv/bin/python`.
Supply your key through your usual environment or secret manager. Do not
paste it into the script or commit it.

To connect this project through Tollglass, give your coding agent the prompt
in the [setup guide](../../AGENTS.md). It should save the routing in this
project and document how to run it from a fresh shell. After setup, use that
saved command; keep the same key and provider. Restore the original base URL
or remove the added configuration to undo routing, as the agent documents.
Tollglass must be running while the example routes through it.

For a single receipt check, `example.py --tag setup-check-<random>` sends a
temporary tag on that request. Without `--tag`, no tag is added. The example
disables SDK retries, so running it sends at most one provider request.
