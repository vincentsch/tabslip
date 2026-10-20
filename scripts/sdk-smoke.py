#!/usr/bin/env python3
"""Check installed OpenAI SDKs against a local fake through the built gateway.

Run with a temporary Python environment containing openai. Pass the path to
an openai Node module and the built Tollglass binary. No provider calls occur.
"""
import argparse
import base64
import hashlib
import http.server
import importlib.metadata
import json
import pathlib
import signal
import socket
import sqlite3
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request

import openai


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--node-module", required=True)
    args = parser.parse_args()
    root = pathlib.Path(__file__).resolve().parents[1]
    binary = str(pathlib.Path(args.binary).resolve())
    node_module = str(pathlib.Path(args.node_module).resolve())
    originals = []
    forwarded = []
    responses = []

    class Fake(http.server.BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def do_POST(self):
            body = self.rfile.read(int(self.headers["Content-Length"]))
            forwarded.append(body)
            payload = json.loads(body)
            stream = payload.get("stream", False)
            raw = (root / "testdata" / ("chat.sse" if stream else "chat.json")).read_bytes()
            responses.append(raw)
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream" if stream else "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            for offset in range(0, len(raw), 7):
                self.wfile.write(raw[offset:offset + 7])
                self.wfile.flush()

    fake = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Fake)
    thread = threading.Thread(target=fake.serve_forever, daemon=True)
    thread.start()
    try:
        with tempfile.TemporaryDirectory(prefix="tollglass-smoke-") as tmp:
            tmp = pathlib.Path(tmp)
            port = free_port()
            closed_port = free_port()
            cfg = tmp / "config.yaml"
            cfg.write_text(f"port: {port}\nupstreams:\n"
                           f"  - name: local-test\n    base_url: http://127.0.0.1:{fake.server_port}\n"
                           "    api: openai\n    inject_usage: false\n"
                           f"  - name: offline\n    base_url: http://127.0.0.1:{closed_port}\n    api: openai\n")
            process = subprocess.Popen([binary, "serve", "--config", str(cfg), "--data-dir", str(tmp / "data"), "--json"],
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            base = f"http://127.0.0.1:{port}"
            try:
                startup_start = time.monotonic()
                for _ in range(200):
                    try:
                        with urllib.request.urlopen(base + "/healthz", timeout=0.2) as response:
                            health = json.load(response)
                        break
                    except (urllib.error.URLError, TimeoutError):
                        if process.poll() is not None:
                            raise AssertionError("gateway exited before health became ready")
                        time.sleep(0.01)
                else:
                    raise AssertionError("gateway did not start")
                startup_ms = round((time.monotonic() - startup_start) * 1000)
                assert health["tool"] == "tollglass" and health["unrecorded_calls"] == 0
                import httpx2 as httpx
                def capture(request):
                    originals.append(request.content)
                with openai.OpenAI(api_key="synthetic-sdk-credential", base_url=base + "/local-test/v1",
                                   http_client=httpx.Client(event_hooks={"request": [capture]})) as client:
                    response = client.chat.completions.create(model="gpt-4.1", messages=[{"role": "user", "content": "hi"}])
                    assert response.usage.prompt_tokens == 19
                    chunks = list(client.chat.completions.create(model="gpt-4.1", messages=[{"role": "user", "content": "hi"}], stream=True))
                    assert chunks[-1].usage.completion_tokens == 10
                node = r'''
const OpenAI = require(process.argv[1]);
const originals = [];
const client = new OpenAI({apiKey: 'synthetic-sdk-credential', baseURL: process.argv[2],
  fetch: async (url, init) => {originals.push(init.body); return fetch(url, init);}});
(async () => {
  const response = await client.chat.completions.create({model:'gpt-4.1',messages:[{role:'user',content:'hi'}]});
  if (response.usage.prompt_tokens !== 19) throw new Error('JSON usage missing');
  let last;
  for await (const chunk of await client.chat.completions.create({model:'gpt-4.1',messages:[{role:'user',content:'hi'}],stream:true})) last = chunk;
  if (last.usage.completion_tokens !== 10) throw new Error('stream usage missing');
  console.log(JSON.stringify({originals,version:require(process.argv[1]+'/package.json').version}));
})().catch(() => {process.exitCode = 1;});
'''
                result = subprocess.run(["node", "-e", node, node_module, base + "/local-test/v1"],
                                        capture_output=True, text=True, check=True, timeout=30)
                node_result = json.loads(result.stdout)
                originals.extend(x.encode() for x in node_result["originals"])
                assert len(originals) == len(forwarded) == 4
                assert originals == forwarded, "disabled injection changed SDK bytes"
                error_request = urllib.request.Request(base + "/offline/v1/chat/completions", data=b'{"model":"gpt-4.1"}', headers={"Content-Type": "application/json"})
                try:
                    urllib.request.urlopen(error_request, timeout=5)
                    raise AssertionError("closed port answered")
                except urllib.error.HTTPError as error:
                    assert error.code == 502
                    failure = json.load(error)
                    assert failure["error"]["type"] == "transport_error"
                process.send_signal(signal.SIGTERM)
                stdout, stderr = process.communicate(timeout=10)
                assert process.returncode == 0
                startup = json.loads(stdout)
                assert startup["key_id"] == health["key_id"] and startup["base_urls"]["local-test"] == base + "/local-test/v1"
                assert "synthetic-sdk-credential" not in stdout + stderr
                db = sqlite3.connect(tmp / "data" / "tollglass.db")
                envelopes = db.execute("select envelope from receipts order by started_at, rowid").fetchall()
                assert len(envelopes) == 5, "shutdown did not drain all receipts"
                for index, (envelope,) in enumerate(envelopes):
                    assert b"synthetic-sdk-credential" not in envelope
                    receipt = json.loads(base64.b64decode(json.loads(envelope)["payload"]))
                    if index < 4:
                        assert receipt["request"]["body_sha256"] == hashlib.sha256(originals[index]).hexdigest()
                        assert receipt["response"]["body_sha256"] == hashlib.sha256(responses[index]).hexdigest()
                        assert receipt["response"]["complete"]
                        assert receipt["response"]["model"] == "gpt-4.1-2025-04-14"
                        assert receipt["usage"]["input_tokens"] == 19
                    else:
                        assert receipt["response"]["status"] is None
                        assert receipt["time"]["first_byte"] is None and receipt["time"]["ttfb_ms"] is None
                        assert not receipt["response"]["complete"]
                        assert any(f["code"] == "transport_error" for f in receipt["findings"])
                assert db.execute("select count(*) from contents").fetchone()[0] == 0
                db.close()
                assert b"synthetic-sdk-credential" not in (tmp / "data" / "tollglass.db").read_bytes()
                # Exercise the default root action, human startup and Ctrl+C separately.
                second = subprocess.Popen([binary, "--config", str(cfg), "--data-dir", str(tmp / "data")], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    for _ in range(200):
                        try:
                            urllib.request.urlopen(base + "/healthz", timeout=0.2).close()
                            break
                        except urllib.error.URLError:
                            time.sleep(0.01)
                    second.send_signal(signal.SIGINT)
                    human, error = second.communicate(timeout=10)
                    assert second.returncode == 0 and "Tollglass is running." in human
                    assert "Content is not stored" in human
                finally:
                    if second.poll() is None:
                        second.kill()
                        second.communicate()
                print(json.dumps({"python_openai": importlib.metadata.version("openai"), "node_openai": node_result["version"],
                                  "sdk_calls": 4, "transport_failure_calls": 1, "stored_receipts": 5,
                                  "sdk_hashes_match": True, "shutdown_signals": ["SIGTERM", "SIGINT"], "startup_ms": startup_ms}))
            finally:
                if process.poll() is None:
                    process.kill()
                    process.communicate()
    finally:
        fake.shutdown()
        fake.server_close()
        thread.join()


if __name__ == "__main__":
    main()
