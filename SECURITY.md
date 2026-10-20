# Reporting a security issue

Use [GitHub private vulnerability reporting](https://github.com/vincentsch/tollglass/security/advisories/new)
for security reports. This channel becomes available when the repository is
public and Vincent has enabled private vulnerability reporting. It is not
yet available while the repository is private. Do not put exploit details,
keys or private call data in public issues.

Include the Tollglass version, operating system and steps to reproduce.
Use synthetic keys and prompts in examples. Remove credentials and private
content from logs, screenshots and files before sharing them.

## Boundaries worth checking

The proxy listens on loopback by default. A proxy made reachable on another
interface forwards credentials without adding authentication. Restrict access
with host networking rules. The local page remains loopback-only, checks its
Host header and requires a token for settings changes.

Provider credentials pass through and must never enter receipts or logs.
Receipts normally keep metadata and hashes. Optional prompt and answer
storage stays local and is excluded from exports. Metadata can still be
private; inspect it before sharing. Keep the database and signing key together
and protect backups.

Ed25519 signatures detect changes since your Tollglass signed a record.
They do not verify model identity or provider claims. The owner of a signing
key can create records. Confirm the signer through a separate trusted channel.
