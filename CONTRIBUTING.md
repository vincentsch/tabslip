# Contributing to Tollglass

Tollglass is a Go program with an embedded local page. There is no frontend
build step. Use the Go version in [go.mod](go.mod); `GOTOOLCHAIN=auto` can
fetch it. Release builds use `CGO_ENABLED=0` and need no installed runtime.

## Build and check a change

On Linux or macOS with Make:

```sh
make build
./bin/tollglass --version
make test race lint docs-check conformance
python3 scripts/docs-links.py
```

Race tests need a native C compiler supported by Go. They do not use the
release build's `CGO_ENABLED=0` setting. On native Windows, start with
`go build ./...`, `go test ./...` and `go vet ./...` in PowerShell.
The hosted checks run builds and tests on Ubuntu, macOS and Windows.
They exercise each runner's architecture, not all six release targets.

For a local release build, run `make build-all VERSION=0.1.0 COMMIT=<commit>`
in a POSIX shell, replacing `<commit>` with the source revision.
[RELEASE.md](RELEASE.md) covers the six assets and installer fixture.

## Keep help and docs together

Run `make docs` after changing commands or flags, then `make docs-check`.
The generator owns only `docs/reference/`. Keep the handwritten guides and
examples in sync with behavior. Python 3 checks local documentation links
and extracts release notes. It is a development tool, not a runtime requirement.

Write plain English, with short sentences and concrete commands. Explain
what a reader can do and what a result means. Avoid claims the code and
checks do not support. Missing provider data stays unknown. A signature
shows that a receipt has not changed since signing; it cannot check the
provider's claims or prevent the signing key's owner from inventing records.

## Test without provider keys

Tests use local fake upstreams and synthetic credentials. Do not add paid
provider calls or subscription agent trials to CI. Add response fixtures
under `testdata/`, with their official reference or SDK source URL and any
changes recorded in [testdata/README.md](testdata/README.md). Test public
behavior, including failure messages, exit codes and forwarding bytes.

The scripts under `scripts/` provide additional SDK, demo and example checks.
Their headers and command help describe required temporary dependencies.
Keep credentials, local databases, signing keys and generated build output
out of commits. Inspect the tracked diff before sending it.

Report security issues through [SECURITY.md](SECURITY.md).
