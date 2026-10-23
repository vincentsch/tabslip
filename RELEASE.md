# Releasing Tabslip

This is the release procedure. The public v0.1.0 verification below records
the completed native execution and GitHub provenance checks.

## Check the candidate locally

Start from a clean checkout. Use the Go version in `go.mod`, Python 3, Make
and a native compiler for the race detector.

```sh
make test race lint docs-check conformance
python3 scripts/docs-links.py
git diff --check
```

Run these again in a disposable clone of the committed candidate, without
local files. Inspect `git ls-files` for private planning state, runtime data,
keys and machine-specific paths. Review secret-search matches before publication:

```sh
git grep -n -i -E 'sk-|OPENROUTER_API_KEY=|BEGIN PRIVATE KEY'
```

Example placeholders are expected. Inspect them; a clean search alone is
not a complete secret review.

Rename `## Unreleased` in `CHANGELOG.md` to `## 0.1.0 - YYYY-MM-DD`, using
the release date. Commit that change before tagging. Keep versions and tags
consistent: the tag is `v0.1.0`; the binary, health response and new receipts
use `0.1.0`. Check the selected notes:

```sh
python3 scripts/release-notes.py 0.1.0
```

Extraction ends at the next level-two heading or EOF. A missing dated version
section fails the release. Start a new Unreleased section for later work.

Build all six files from that committed candidate:

```sh
make build-all VERSION=0.1.0 COMMIT="$(git rev-parse HEAD)"
```

`dist/` contains `tabslip-linux-amd64`, `tabslip-linux-arm64`,
`tabslip-darwin-amd64`, `tabslip-darwin-arm64`,
`tabslip-windows-amd64.exe`, `tabslip-windows-arm64.exe` and
`checksums.txt`. These static binaries are the release assets; keep these
names and bytes through attestation and upload. Run the native binary's
`--version`, `__rungrad_manifest` and `demo`. Check `/healthz` and a new
receipt for the release version. Inspect other binaries with `file` when
you cannot run them. Cross-compilation does not prove native execution.

## Check installation before publishing

Use a disposable local directory for the fake release downloads:

```sh
scripts/fake-release.sh dist /tmp/tabslip-release-fixture v0.1.0
python3 -m http.server 8765 --bind 127.0.0.1 --directory /tmp/tabslip-release-fixture
```

In another shell, install into a temporary prefix:

```sh
INSTALL_DIR=/tmp/tabslip-install TABSLIP_VERSION=v0.1.0 \
  TABSLIP_DOWNLOAD_BASE=http://127.0.0.1:8765 sh install.sh
/tmp/tabslip-install/tabslip --version
/tmp/tabslip-install/tabslip __rungrad_manifest
```

Check the reported version and commit against the candidate. Compare its hash
with `checksums.txt`. `go test ./scripts` checks rejection of wrong checksums
and preservation of an existing installation. Stop the local HTTP server
and remove the disposable directories afterward.

## Publish and check the hosted results

When publication is authorized, make the repository public. Immediately enable
GitHub private vulnerability reporting and verify the
[reporting channel](https://github.com/vincentsch/tabslip/security/advisories/new)
before announcing a release. After enabling reporting and verifying the channel,
remove the obsolete availability caveat from `SECURITY.md`. Do not claim the
channel works before this check.

Push main and confirm CI is green on Ubuntu, macOS and Windows. Native jobs
run `go build ./...` and `go test ./...` directly. They cover the runners'
architectures, not native execution on all six targets. Only after these
checks succeed, create and push the `v0.1.0` tag for the committed candidate.

The tag triggers reusable CI. The publishing job waits for those checks,
builds the six assets, attests them and uploads those same bytes with the
checksum manifest. Only that job has release and attestation write permissions.
Confirm the release contains exactly those seven download assets and the
selected changelog section.

Download and verify the release on supported native systems. The installer
compares each binary's SHA-256 with the manifest. GitHub provenance is a
separate check of the binary's recorded build origin. Install GitHub CLI,
then run:

```sh
gh attestation verify tabslip-<os>-<arch> --repo vincentsch/tabslip
```

Replace `<os>` with `linux`, `darwin` or `windows`, and `<arch>` with `amd64`
or `arm64`. Add `.exe` for Windows, for example `tabslip-windows-amd64.exe`.
Inspect the verified source repository, release workflow and source commit.
See the [attestation action](https://github.com/actions/attest-build-provenance)
for provenance details. Check each of the six binaries separately.

Verify the public raw installer URL and release downloads from a fresh shell:

```sh
curl -fsSL https://raw.githubusercontent.com/vincentsch/tabslip/main/install.sh | sh
tabslip --version
tabslip demo
```

Use a temporary `INSTALL_DIR` if replacing an installed binary is unwanted.
Also test `TABSLIP_VERSION=v0.1.0` and native Windows download/checksum steps
from [the quickstart](docs/quickstart.md#install). Native PowerShell download
and checksum commands still need execution on Windows; Linux review cannot
establish that result. Confirm all eight demo receipts, then test the source fallback
in a temporary `GOBIN`:

```sh
go install github.com/vincentsch/tabslip/cmd/tabslip@latest
```

The source fallback reports `dev`. Confirm its module revision and manifest.
Hosted CI, provenance issuance and verification, public installer access,
release downloads and the public source fallback remain publication-only
checks. Local preparation cannot establish their success.

## Public v0.1.0 verification, 2026-10-23

Published [tabslip v0.1.0](https://github.com/vincentsch/tabslip/releases/tag/v0.1.0)
from `b2897ffcab6449de8812f3d95ecf7703b020aca9`. The [release workflow](https://github.com/vincentsch/tabslip/actions/runs/39947441701)
passed Linux, macOS and Windows CI before the publishing job started. The release
contains six platform executables and `checksums.txt`. All six downloaded SHA-256
hashes matched. All six GitHub build attestations verified with `gh attestation verify`, the release workflow identity, tag ref and source digest pinned, and
`--deny-self-hosted-runners`.

On Linux amd64, latest and pinned (`v0.1.0`) public installers succeeded in fresh
temporary folders. The installed CLI's version and bundled demo were checked.
`go install github.com/vincentsch/tabslip/cmd/tabslip@v0.1.0` succeeded with an
initially empty module cache, the public Go proxy and checksum database. A separate
empty-cache `go install` with `@latest` also resolved v0.1.0 successfully. The
README agent prompt and `AGENTS.md` were fetched anonymously from GitHub and
matched the published source. Fetching the prompt does not establish a trial
with every supported coding agent.

The [native public-release smoke run](https://github.com/vincentsch/tabslip/actions/runs/39948036190)
passed on Linux amd64, macOS arm64 and Windows amd64: public native downloads,
checksum checks, latest and pinned installation where supported, version and demo
checks. Windows checked the `.exe` hash with PowerShell `Get-FileHash`. This is
native execution coverage for those three hosts; the other three assets were
cross-built and their checksums and provenance verified.
