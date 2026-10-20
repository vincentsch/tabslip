# Connect an upstream

Tollglass keeps your provider and forwards the client's credentials. Change
the client's base URL to the selected local route. The [setup guide](../AGENTS.md)
explains discovery, instance reuse and undo.

| Provider | Client base URL at the default port |
| --- | --- |
| OpenAI | `http://127.0.0.1:4545/openai/v1` |
| OpenRouter | `http://127.0.0.1:4545/openrouter/v1` |
| Ollama | `http://127.0.0.1:4545/ollama/v1` |
| Anthropic | `http://127.0.0.1:4545/anthropic` |

For another provider, preserve the original root in a custom YAML upstream.
Remove its API `/v1` suffix, keeping any path before that suffix:

```yaml
upstreams:
  - name: my-provider
    base_url: https://provider.example/api
    api: openai
```

Point the OpenAI-compatible client at `http://127.0.0.1:4545/my-provider/v1`.
For Anthropic Messages, use `api: anthropic` and a client URL ending in
`/my-provider`. Preserve other settings and entries. Custom upstreams load
at startup, so a running server needs a restart to see changes.

## An app in Docker on Linux

With `docker run --network host ...`, the app can reach the host's loopback
address and use the URLs above.

With bridge networking, add `--add-host=host.docker.internal:host-gateway` to
`docker run`, then use `http://host.docker.internal:4545/openai/v1`, or the
route for your upstream. The hostname alone does not make loopback reachable.
Tollglass must listen on the host bridge address or another reachable address,
for example `tollglass serve --listen 0.0.0.0`. Check access from the container.
Use the selected port if it differs from 4545.

The proxy forwards credentials without added authentication. Restrict access
to the app with host networking rules. The page stays loopback-only; exposing
the proxy does not expose the page. Ask the owner before restarting an
existing instance or changing its listen address.

## A server beside your app

Run Tollglass beside the app and use its loopback address. Reach the page with
`ssh -N -L 4545:127.0.0.1:4545 user@server`, then open
`http://127.0.0.1:4545`. The local port must equal the remote Tollglass port
because the page checks the port in the browser's Host header. Use the selected
port on both sides and in the local page URL. That local port must be free.
The tunnel ends with Ctrl+C. Tollglass must keep running while the app routes
through it. See the setup guide for detached start, owned-process stop and undo.
