#!/usr/bin/env python3
"""Exercise the built receipt commands against a running and stopped offline demo."""
import argparse
import json
import pathlib
import signal
import subprocess
import tempfile
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument('--binary', required=True)
parser.add_argument('--html-out', type=pathlib.Path)
args = parser.parse_args()
binary = str(pathlib.Path(args.binary).resolve())
with tempfile.TemporaryDirectory(prefix='tabslip-receipts-') as temp:
    root = pathlib.Path(temp)
    data = root / 'data'
    demo = subprocess.Popen([binary, 'demo', '--data-dir', str(data), '--json'],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        startup_text = ''
        while True:
            line = demo.stdout.readline()
            assert line, 'Demo stopped before printing its startup object.'
            startup_text += line
            try:
                startup = json.loads(startup_text)
                break
            except json.JSONDecodeError:
                continue
        def run(*parts, expected=0, selected=data):
            result = subprocess.run([binary, *map(str, parts), '--data-dir', str(selected),
                                     '--config', str(root / 'absent.yaml')],
                                    capture_output=True, text=True, timeout=20)
            assert result.returncode == expected, (parts, result.returncode, result.stdout, result.stderr)
            return result
        rows = json.loads(run('receipts', '--json').stdout)
        assert len(rows) == 8
        fallback = json.loads(run('receipts', '--model', 'demo/large', '--json').stdout)
        served = json.loads(run('receipts', '--model', 'demo/small', '--json').stdout)
        assert len(fallback) == 1 and any(r['id'] == fallback[0]['id'] for r in served)
        long_model = 'vendor/a-long-model-name/with-many-segments-and-words/<script>window.receiptInjected=true</script>'
        request = urllib.request.Request(startup['base_urls']['openai'] + '/chat/completions',
                                         data=json.dumps({'model': long_model, 'messages': []}).encode(),
                                         headers={'Content-Type': 'application/json'})
        with urllib.request.urlopen(request, timeout=10) as response:
            response.read()
        # The demo stores each receipt after its response; poll the read view briefly.
        import time
        for _ in range(100):
            if len(json.loads(run('receipts', '--json').stdout)) == 9:
                break
            time.sleep(0.02)
        key = root / 'tabslip.pub'
        run('key', 'export', '--out', key)
        for state in ('running', 'stopped'):
            if state == 'stopped':
                demo.send_signal(signal.SIGTERM)
                assert demo.wait(timeout=10) == 0
            bundle = root / f'{state}.tabslip.json'
            run('export', '--out', bundle)
            run('verify', bundle, '--key', key)
            run('verify', bundle)
            run('verify', bundle, '--key-id', startup['key_id'])
            run('verify', bundle, '--key-id', 'ts_other', expected=11)
            run('verify', bundle, selected=root / 'foreign', expected=11)
            assert len(json.loads(run('receipts', '--json').stdout)) == 9
            run('show', rows[0]['id'])
            assert json.loads(run('show', rows[0]['id'], '--json').stdout)['payloadType'].endswith('receipt+json')
            for fmt in ('csv', 'jsonl', 'html'):
                out = root / f'{state}.{fmt}'
                run('export', '--format', fmt, '--out', out, '--dry-run')
                assert not out.exists()
                run('export', '--format', fmt, '--out', out)
                before = out.read_bytes()
                if fmt == 'html':
                    assert b'<script>' not in before and b'&lt;script&gt;' in before
                run('export', '--format', fmt, '--out', out, expected=2)
                assert out.read_bytes() == before
                if args.html_out and state == 'stopped' and fmt == 'html':
                    args.html_out.parent.mkdir(parents=True, exist_ok=True)
                    args.html_out.write_bytes(before)
        print('PASS: eight demo receipts plus a long markup model; running/stopped reads; four exports; signer verdicts; previews; overwrite refusal.')
    finally:
        if demo.poll() is None:
            demo.send_signal(signal.SIGTERM)
            demo.wait(timeout=10)
