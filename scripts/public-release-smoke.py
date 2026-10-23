#!/usr/bin/env python3
"""Exercise a published, checksum-verified native release in a clean folder."""
from pathlib import Path
import hashlib
import json
import os
import platform
import re
import subprocess
import ssl
import tempfile
import time
import urllib.request

name = re.search(r"^module github.com/vincentsch/(\w+)$", Path("go.mod").read_text(), re.M)[1]
assert name in {"tabslip", "saidran", "sealjar", "fendkit"}
version = os.environ.get("RELEASE_VERSION", "v0.1.0")
assert re.fullmatch(r"v\d+\.\d+\.\d+", version)
system = {"Linux": "linux", "Darwin": "darwin", "Windows": "windows"}[platform.system()]
arch = "arm64" if platform.machine().lower() in {"arm64", "aarch64"} else "amd64"
asset = f"{name}-{system}-{arch}" + (".exe" if system == "windows" else "")
work = Path(tempfile.mkdtemp(prefix=f"{name}-public-smoke-"))
results = {"product": name, "version": version, "system": system, "arch": arch, "checks": []}

def download(url, destination):
    with urllib.request.urlopen(url, timeout=90) as response:
        destination.write_bytes(response.read())

def command(args, expected=(0,), env=None):
    result = subprocess.run(list(map(str, args)), cwd=work, env=env, text=True, capture_output=True, timeout=300)
    print("$", " ".join(map(str, args)), "exit", result.returncode)
    print(result.stdout)
    print(result.stderr)
    assert result.returncode in expected, (args, result.returncode)
    return result

for mode, route in [("latest", "latest/download"), ("pinned", f"download/{version}")]:
    folder = work / mode
    folder.mkdir()
    base = f"https://github.com/vincentsch/{name}/releases/{route}"
    download(f"{base}/checksums.txt", folder / "checksums.txt")
    expected = {line.split()[1].lstrip("*"): line.split()[0] for line in (folder / "checksums.txt").read_text().splitlines()}[asset]
    download(f"{base}/{asset}", folder / asset)
    binary = folder / asset
    assert hashlib.sha256(binary.read_bytes()).hexdigest() == expected
    binary.chmod(0o755)
    if system == "windows":
        env = dict(os.environ, RELEASE_SMOKE_BINARY=str(binary), RELEASE_SMOKE_SHA256=expected)
        command(["pwsh", "-NoProfile", "-Command", 'if ((Get-FileHash -Algorithm SHA256 -LiteralPath $env:RELEASE_SMOKE_BINARY).Hash.ToLower() -ne $env:RELEASE_SMOKE_SHA256) { throw "Checksum mismatch" }'], env=env)
    else:
        installer = folder / "install.sh"
        download(f"https://raw.githubusercontent.com/vincentsch/{name}/main/install.sh", installer)
        env = dict(os.environ, INSTALL_DIR=str(folder / "installed"))
        env.pop(name.upper() + "_VERSION", None)
        if mode == "pinned":
            env[name.upper() + "_VERSION"] = version
        command(["sh" if name == "tabslip" else "bash", installer], env=env)
        binary = folder / "installed" / name
        assert hashlib.sha256(binary.read_bytes()).hexdigest() == expected
    assert version.lstrip("v") in command([binary, "--version"]).stdout
    results["checks"].append(mode + " native binary, checksum and version")

if name == "tabslip":
    output = work / "demo.json"
    with output.open("w") as stream:
        process = subprocess.Popen([str(binary), "demo", "--data-dir", str(work / "data"), "--json"], cwd=work, stdout=stream)
        try:
            for attempt in range(150):
                try:
                    demo = json.loads(output.read_text())
                    break
                except json.JSONDecodeError:
                    time.sleep(.2)
            else:
                raise RuntimeError("Demo did not become ready")
            with urllib.request.urlopen(demo["page_url"] + "/healthz", timeout=10) as response:
                assert json.load(response)["version"] == version.lstrip("v")
            rows = json.loads(command([binary, "receipts", "--data-dir", work / "data", "--json"]).stdout)
            assert len(rows) == 8
            command([binary, "export", "--data-dir", work / "data", "--out", work / "demo.tabslip.json"])
            command([binary, "key", "export", "--data-dir", work / "data", "--out", work / "tabslip.pub"])
            command([binary, "verify", work / "demo.tabslip.json", "--key", work / "tabslip.pub"])
        finally:
            process.terminate()
            process.wait(timeout=40)
    results["checks"].append("eight demo receipts, health version, export and signed verification")
elif name == "saidran":
    env = dict(os.environ, TMPDIR=str(work), TMP=str(work), TEMP=str(work))
    result = command([binary, "demo"], env=env)
    assert "Stale" in result.stdout
    reports = list(work.glob("saidran-demo-*/repo/.git/saidran/*/report.json"))
    assert len(reports) == 1
    for filename in ["report.json", "report.html", "report.md"]:
        assert (reports[0].parent / filename).stat().st_size > 0
    results["checks"].append("Stale demo and all three report files")
elif name == "sealjar":
    demo = json.loads(command([binary, "demo", "--out", work / "bakery", "--json"]).stdout)
    verified = command([binary, "verify", demo["package"], "--json"], expected=(0, 11))
    command([binary, "verify", demo["tampered_package"], "--json"], expected=(10,))
    assert Path(demo["reader_folder"], "report", "report.en.html").is_file()
    results["checks"].append("demo, reader report and tamper rejection")
    results["demo_verify_exit"] = verified.returncode
else:
    command([binary, "demo", "--offline", "--out", work / "demo"], expected=(11,))
    for filename in ["status.html", "status.md", "sbom.cdx.json"]:
        assert (work / "demo/acme-booking/cra/reports" / filename).stat().st_size > 0
    results["checks"].append("offline demo, expected incomplete exit 11, status and SBOM")
if name == "sealjar" and system == "darwin":
    trust = Path.cwd() / "public-release-trust"
    trust.mkdir()
    system_roots = trust / "macos-system-roots.pem"
    exported = command(["security", "find-certificate", "-a", "-p", "/System/Library/Keychains/SystemRootCertificates.keychain"])
    system_roots.write_text(exported.stdout)
    assert "BEGIN CERTIFICATE" in exported.stdout
    freetsa_root = trust / "freetsa-cacert.pem"
    download("https://freetsa.org/files/cacert.pem", freetsa_root)
    repo = work / "trust-repo"
    repo.mkdir()
    (repo / "example.txt").write_text("Timestamp trust check\n")
    command(["git", "-C", repo, "init"])
    command(["git", "-C", repo, "add", "example.txt"])
    command(["git", "-C", repo, "-c", "user.name=Trust Check", "-c", "user.email=trust@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "Add example"])
    results["macos_timestamp_trust"] = []
    for authority, endpoint, roots, source in [
        ("digicert", "http://timestamp.digicert.com", system_roots, "macOS security export from SystemRootCertificates.keychain"),
        ("freetsa", "https://freetsa.org/tsr", freetsa_root, "https://freetsa.org/files/cacert.pem"),
    ]:
        capture = json.loads(command([binary, "capture", repo, "--confirm", "--tsa", endpoint, "--name", authority, "--out", work / "trust-captures", "--json"]).stdout)
        default = command([binary, "verify", capture["package"], "--json"], expected=(0, 11))
        explicit = command([binary, "verify", capture["package"], "--tsa-root", roots, "--json"], expected=(0, 11))
        parsed = json.loads(default.stdout)
        assert parsed["tokens"] and all(not token["demo"] for token in parsed["tokens"])
        assert any(authority in token["authority"].lower() for token in parsed["tokens"])
        fingerprints = [hashlib.sha256(ssl.PEM_cert_to_DER_cert(pem)).hexdigest() for pem in re.findall(r"-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----", roots.read_text(), re.S)]
        assert fingerprints
        entry = {"authority": authority, "endpoint": endpoint, "default_exit": default.returncode, "explicit_root_exit": explicit.returncode, "roots_source": source, "roots_path": str(roots), "roots_file_sha256": hashlib.sha256(roots.read_bytes()).hexdigest(), "root_certificate_sha256": fingerprints, "default": parsed, "explicit": json.loads(explicit.stdout)}
        results["macos_timestamp_trust"].append(entry)
        (trust / (authority + "-results.json")).write_text(json.dumps(entry, indent=2) + "\n")
        assert explicit.returncode == 0, "Explicit-root trust failed"
    results["checks"].append("native macOS DigiCert and FreeTSA capture, default trust and independent-root verification recorded")

Path("public-release-smoke-result.json").write_text(json.dumps(results, indent=2) + "\n")
print(json.dumps(results, indent=2))
if os.environ.get("GITHUB_STEP_SUMMARY"):
    with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a") as summary:
        summary.write(f"### {name} {version}: {system}/{arch}\n\n" + "\n".join("- " + check for check in results["checks"]) + "\n")
