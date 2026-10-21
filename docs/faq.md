# Frequently asked questions

For installation and a first call, use the [quickstart](quickstart.md).
The [command guide](commands.md) links the full procedures.

## Does Tabslip cost money?

Tabslip has no usage charge. Your provider still charges for API calls.
The offline demo uses simulated calls and needs no key. Your coding agent
is a separate service with its own terms and prices. See [costs](costs.md)
for reported amounts, estimates and unknown values.

## Can I use it on a server or in a container?

Run it beside your app, then use a local route to the gateway.
The [upstream guide](upstreams.md) explains Docker networking and an SSH
tunnel for the local page. Restrict access to a reachable proxy, which
forwards credentials without added authentication.

## Can I keep prompts and answers?

Enable local content storage in Setup for all calls or selected tags.
The page header shows the current setting. Exports exclude this content.
Read [storage and privacy](privacy.md) before enabling it or sharing metadata.

## How do I share a receipt and check who signed it?

Export a signed bundle and confirm the public key through a separate channel.
[Offline verification](verify.md) gives the commands and exit codes.
A valid signature checks the signed record; provider claims remain unverified.
[Receipt fields](receipts.md) explains findings and signed supplements.

## How do I stop or restart it?

Restore your app's original provider URL before stopping if it must keep working.
Ctrl+C stops a foreground server. A temporary demo directory is removed;
an explicit demo data directory remains. Keep the database and signing key together.

Restart with the same config, data directory and required environment.
Installation does not add a service manager, so routing through Tabslip
also needs a running gateway after reboot. The [agent setup guide](setup-with-an-agent.md#reuse-stopping-and-undo)
explains owned background processes and undo. See [troubleshooting](troubleshooting.md)
if calls fail after stopping.
