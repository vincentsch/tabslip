#!/usr/bin/env python3
"""Run the five public examples against a built demo and match new receipts.

Use the Python interpreter with the example SDKs installed. Install the Node
example's dependency first. This check makes no provider requests.
"""
import argparse
import json
import os
from pathlib import Path
import signal
import subprocess
import tempfile
import time
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', required=True)
    parser.add_argument('--source', default=str(Path(__file__).resolve().parents[1]))
    args = parser.parse_args()
    binary = str(Path(args.binary).resolve())
    root = Path(args.source).resolve()
    with tempfile.TemporaryDirectory(prefix='tabslip-examples-') as data:
        process = subprocess.Popen([binary, 'demo', '--data-dir', data, '--json'],
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            lines = []
            while True:
                line = process.stdout.readline()
                if not line:
                    raise RuntimeError('Demo stopped before startup: ' + process.stderr.read())
                lines.append(line)
                try:
                    startup = json.loads(''.join(lines))
                    break
                except json.JSONDecodeError:
                    pass
            base = dict(os.environ, OPENAI_API_KEY='demo', ANTHROPIC_API_KEY='demo',
                        OPENROUTER_API_KEY='demo', OPENAI_BASE_URL=startup['base_urls']['openai'],
                        ANTHROPIC_BASE_URL=startup['base_urls']['anthropic'],
                        OPENROUTER_BASE_URL=startup['base_urls']['openai'])
            cases = [
                ('python-openai', [os.sys.executable, 'example.py']),
                ('python-anthropic', [os.sys.executable, 'example.py']),
                ('node-openai', ['node', 'example.mjs']),
                ('curl', ['sh', 'example.sh']),
                ('openrouter-tags', [os.sys.executable, 'example.py']),
            ]
            results = []
            for name, command in cases:
                tag = 'example-' + name + '-' + uuid.uuid4().hex[:8]
                if name.startswith('python-'):
                    command += ['--tag', tag]
                run = subprocess.run(command, cwd=root / 'examples' / name,
                                     env=dict(base, TABSLIP_TAG=tag), text=True,
                                     capture_output=True, timeout=30, check=True)
                assert run.stdout.strip(), name
                rows = []
                for _ in range(30):
                    rows = json.loads(subprocess.check_output(
                        [binary, 'receipts', '--data-dir', data, '--tag', tag, '--json']))
                    if rows:
                        break
                    time.sleep(1)
                assert len(rows) == 1 and rows[0]['status'] == 200, (name, rows)
                envelope = json.loads(subprocess.check_output(
                    [binary, 'show', rows[0]['id'], '--data-dir', data, '--json']))
                import base64
                receipt = json.loads(base64.b64decode(envelope['payload']))
                assert receipt['response']['complete'], name
                assert receipt['request']['upstream'] == 'demo', name
                assert receipt['usage']['source'] == 'reported', name
                results.append({'example': name, 'tag': tag, 'receipt_id': rows[0]['id'],
                                'status': rows[0]['status']})
            print(json.dumps(results, indent=2))
        finally:
            process.send_signal(signal.SIGTERM)
            try:
                process.communicate(timeout=35)
            except subprocess.TimeoutExpired:
                process.kill()
                process.communicate()
                raise


if __name__ == '__main__':
    main()
