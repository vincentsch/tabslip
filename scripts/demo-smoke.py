#!/usr/bin/env python3
"""Check a built offline demo, SQLite records, signatures and data cleanup."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import select
import signal
import sqlite3
import subprocess
import tempfile
import time
import urllib.request
from decimal import Decimal

from cryptography.hazmat.primitives.serialization import load_pem_public_key


def verify(envelope, public_key, payload_type):
    envelope = json.loads(envelope)
    assert envelope['payloadType'] == payload_type
    payload = base64.b64decode(envelope['payload'])
    kind = payload_type.encode()
    pae = b'DSSEv1 %d %s %d ' % (len(kind), kind, len(payload)) + payload
    public_key.verify(base64.b64decode(envelope['signatures'][0]['sig']), pae)
    return json.loads(payload), payload


def check_records(data, expected):
    public_key = load_pem_public_key((data / 'key.pub').read_bytes())
    with sqlite3.connect(data / 'tabslip.db') as db:
        rows = db.execute('SELECT id,envelope,cost_amount,cost_source,provider,has_findings,input_tokens FROM receipts ORDER BY started_at,id').fetchall()
        assert len(rows) == expected, len(rows)
        receipts = {}
        for row in rows:
            receipt, payload = verify(row[1], public_key, 'application/vnd.tabslip.receipt+json')
            receipts[row[0]] = (receipt, payload)
            assert receipt['cost']['price_table'] is None
            assert 'ignored-synthetic-key' not in payload.decode()
            if receipt['request']['api'] == 'anthropic.messages':
                assert row[6] == 175
                assert receipt['usage']['input_tokens'] == 100
        supplements = db.execute('SELECT receipt_id,envelope FROM supplements').fetchall()
        assert len(supplements) == 1
        supplement, _ = verify(supplements[0][1], public_key, 'application/vnd.tabslip.supplement+json')
        receipt, payload = receipts[supplement['receipt_id']]
        assert supplement['receipt_sha256'] == hashlib.sha256(payload).hexdigest()
        assert receipt['request']['model'] == 'demo/large'
        assert 'models' not in receipt['request']['params']
        assert receipt['response']['model'] == 'demo/small'
        assert receipt['cost']['amount'] == '0.000240'
        assert [f['code'] for f in receipt['findings']] == ['model_differs']
        assert supplement['source'] == 'demo.generation'
        assert supplement['cost'] == '0.000123'
        assert len(supplement['attempts']) == 2
        assert supplement['provider'] == 'Demo provider (simulated)'
        assert supplement['findings'][0]['text'] == 'The demo router tried 2 providers. The answer came from Demo provider (simulated).'
        summary = next(r for r in rows if r[0] == supplement['receipt_id'])
        assert summary[2:6] == ('0.000123', 'reported', 'Demo provider (simulated)', 1)
        total = sum((Decimal(r[2]) for r in rows if r[2] is not None), Decimal(0))
        assert total == Decimal('0.000603' if expected == 8 else '0.000843'), total
        types = [r['request']['api'] for r, _ in receipts.values()]
        assert types.count('openai.chat') == (5 if expected == 8 else 6)
        assert types.count('anthropic.messages') == 2
        assert types.count('openai.embeddings') == 1
        tags = [r['request'].get('tag') for r, _ in receipts.values()]
        assert tags.count('acme-shop') == 4 and tags.count('blog-tool') == 4
        values = [r for r, _ in receipts.values()]
        assert sum(r['request']['stream'] is True for r in values) == 2
        assert sum(r['response']['status'] == 429 for r in values) == 1
        missing = next(r for r in values if r['request']['model'] == 'demo/no-usage')
        assert missing['usage']['source'] == 'unknown' and missing['cost']['amount'] is None
        assert all(f['code'] != 'price_unknown' for r in values for f in r['findings'])
        if expected == 8:
            times = [(r['request']['model'], r['request']['stream'], r['time']['ttfb_ms'], r['time']['total_ms']) for r in values]
            assert all(first >= 150 for _, _, first, _ in times), times
            assert all(total >= 1000 for _, stream, _, total in times if stream), times
            print('Initial measured timings (model, stream, first/total ms):', times)
        return {row[0]: row[1] for row in rows}


def run(binary, explicit, root):
    config = root / 'normal-config.yaml'
    config.write_text('port: [invalid normal config\n')
    original_config = config.read_bytes()
    args = [str(binary), 'demo', '--json', '--config', str(config)]
    if explicit:
        args += ['--data-dir', str(root / 'kept-data')]
    proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                            env={**os.environ, 'TABSLIP_CONFIG': str(config)})
    started = time.monotonic()
    data = None
    try:
        assert select.select([proc.stdout], [], [], 15)[0], 'startup timed out'
        line = proc.stdout.readline()
        assert line, proc.stderr.read()
        while True:
            try:
                info = json.loads(line)
                break
            except json.JSONDecodeError:
                extra = proc.stdout.readline()
                assert extra, 'incomplete startup JSON'
                line += extra
        print(f'Completed seed in {time.monotonic() - started:.3f}s (explicit={explicit}).')
        assert set(info) == {'page_url', 'base_urls', 'data_dir', 'key_id', 'commands'}
        assert info['base_urls'] == {'openai': info['page_url'] + '/demo/v1', 'anthropic': info['page_url'] + '/demo'}
        assert set(info['commands']) == {'export', 'key_export', 'verify'}
        data = Path(info['data_dir'])
        assert info['key_id'].startswith('ts_')
        before = check_records(data, 8)
        req = urllib.request.Request(info['base_urls']['openai'] + '/chat/completions',
                                     data=b'{"model":"gpt-4.1","messages":[{"role":"user","content":"hi"}]}',
                                     headers={'Content-Type': 'application/json', 'Authorization': 'Bearer ignored-synthetic-key'})
        with urllib.request.urlopen(req, timeout=5) as response:
            assert json.load(response)['model'] == 'gpt-4.1'
        # Shutdown drains capture and storage before inspection.
        proc.send_signal(signal.SIGINT)
        stdout, stderr = proc.communicate(timeout=15)
        assert proc.returncode == 0 and stdout == '' and stderr == '', (proc.returncode, stdout, stderr)
        assert config.read_bytes() == original_config
        if explicit:
            assert data.exists()
            after = check_records(data, 9)
            assert all(after[key] == value for key, value in before.items())
        else:
            assert not data.exists(), 'temporary data left behind'
    finally:
        if proc.poll() is None:
            proc.terminate()
            proc.communicate(timeout=15)


def interrupt_seed(binary, explicit, root):
    # Own TMPDIR so the default directory can be located before startup output.
    temporary = root / ('interrupt-explicit' if explicit else 'interrupt-default')
    temporary.mkdir()
    data = temporary / 'kept-data'
    args = [str(binary), 'demo', '--json']
    if explicit:
        args += ['--data-dir', str(data)]
    proc = subprocess.Popen(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
                            env={**os.environ, 'TMPDIR': str(temporary)})
    try:
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            candidates = [data] if explicit else list(temporary.glob('tabslip-demo-*'))
            if candidates and (candidates[0] / 'tabslip.db').exists():
                data = candidates[0]
                break
            assert proc.poll() is None, proc.communicate()
            time.sleep(0.01)
        else:
            raise AssertionError('demo data directory not created')
        # Reach the delayed stream, before startup output is allowed.
        time.sleep(0.6)
        started = time.monotonic()
        proc.send_signal(signal.SIGINT)
        stdout, stderr = proc.communicate(timeout=2)
        elapsed = time.monotonic() - started
        assert proc.returncode == 0 and stdout == '' and stderr == '', (proc.returncode, stdout, stderr)
        assert data.exists() == explicit, 'wrong interruption cleanup'
        print(f'Interrupted seed (explicit={explicit}): exit 0 in {elapsed:.3f}s; stdout/stderr empty; data preserved={explicit}.')
    finally:
        if proc.poll() is None:
            proc.terminate()
            proc.communicate(timeout=5)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix='tabslip-demo-smoke-') as directory:
        root = Path(directory)
        for explicit in (False, True):
            run(args.binary.resolve(), explicit, root)
            interrupt_seed(args.binary.resolve(), explicit, root)
    print('Demo smoke passed: eight calls, signed supplement, exact totals, later call, isolated config and shutdown cleanup.')


if __name__ == '__main__':
    main()
