#!/usr/bin/env python3
"""Check the public example with the SDK installed in this Python environment."""

import json
import os
from pathlib import Path
import subprocess
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

EXAMPLE = Path(__file__).resolve().parents[1] / "examples/python-openai/example.py"


class ExampleTest(unittest.TestCase):
    def test_provider_routing_and_request_only_tag(self):
        requests = []

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                requests.append((self.path, self.headers, body))
                result = {
                    "id": "chatcmpl-example", "object": "chat.completion",
                    "created": 1, "model": body["model"],
                    "choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello."},
                                 "finish_reason": "stop"}],
                    "usage": {"prompt_tokens": 10, "completion_tokens": 2, "total_tokens": 12},
                }
                encoded = json.dumps(result).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)

        with ThreadingHTTPServer(("127.0.0.1", 0), Handler) as server:
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            try:
                env = dict(os.environ, OPENAI_API_KEY="example-synthetic-key",
                           OPENAI_BASE_URL=f"http://127.0.0.1:{server.server_port}/provider/v1")
                for args in (["--tag", "setup-check-example"], []):
                    run = subprocess.run([sys.executable, str(EXAMPLE), *args], env=env,
                                         text=True, capture_output=True, check=True)
                    self.assertEqual(run.stdout, "Hello.\n")
                    self.assertNotIn(env["OPENAI_API_KEY"], run.stdout + run.stderr)
                self.assertEqual(len(requests), 2)
                for path, headers, body in requests:
                    self.assertEqual(path, "/provider/v1/chat/completions")
                    self.assertEqual(headers["Authorization"], "Bearer example-synthetic-key")
                    self.assertEqual(body["model"], "gpt-4.1")
                    self.assertEqual(body["max_completion_tokens"], 64)
                    self.assertEqual(len(body["messages"]), 1)
                self.assertEqual(requests[0][1]["X-Tabslip-Tag"], "setup-check-example")
                self.assertNotIn("X-Tabslip-Tag", requests[1][1])
            finally:
                server.shutdown()
                thread.join()


if __name__ == "__main__":
    unittest.main()
