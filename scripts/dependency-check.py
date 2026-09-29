#!/usr/bin/env python3
"""OB-001 real-module preflight. Never substitutes sources or manufactures sums.

--prepare additionally runs go mod tidy in THIS SDK. Run with a reviewed, fixed
Go toolchain; automatic toolchain downloading is disabled. PASS here does not
mean normal tests, server compatibility, consumers or release checks passed.
"""
from __future__ import annotations

import argparse
import base64
import datetime
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parents[1]
MODULE = 'github.com/openbao/openbao/api/v2'
VERSION = 'v2.7.0'
SOURCE_NAMES = ('client.go', 'request.go', 'response.go')


def sha256(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def real_file(value):
    p = pathlib.Path(value)
    if not p.is_absolute() or p.is_symlink() or not p.is_file():
        raise ValueError('required module file is absent or not a regular absolute file')
    return p


def sum_value(value):
    if not isinstance(value, str) or not value.startswith('h1:'):
        raise ValueError('missing module checksum')
    try:
        raw = base64.b64decode(value[3:], validate=True)
    except ValueError as e:
        raise ValueError('invalid module checksum encoding') from e
    if len(raw) != 32:
        raise ValueError('invalid module checksum length')
    return value


def validate_download(record):
    if record.get('Error') or record.get('Path') != MODULE or record.get('Version') != VERSION:
        raise ValueError('download failed or returned a different module/version')
    mod = real_file(record.get('GoMod', ''))
    archive = real_file(record.get('Zip', ''))
    directory = pathlib.Path(record.get('Dir', ''))
    if not directory.is_absolute() or directory.is_symlink() or not directory.is_dir():
        raise ValueError('downloaded source directory is unavailable')
    return {
        'module': MODULE, 'version': VERSION,
        'sum': sum_value(record.get('Sum')), 'go_mod_sum': sum_value(record.get('GoModSum')),
        'go_mod_sha256': sha256(mod), 'zip_sha256': sha256(archive),
        'source_sha256': {name: sha256(real_file(str(directory / name))) for name in SOURCE_NAMES},
    }


def validate_upstream_mod(parsed):
    version = parsed.get('Go', '')
    if parsed.get('Module', {}).get('Path') != MODULE or not re.fullmatch(r'1\.\d+(?:\.\d+)?', version):
        raise ValueError('upstream go.mod lacks the exact module identity or minimum Go directive')
    return version


def validate_root(parsed):
    if parsed.get('Replace'):
        raise ValueError('replace directives are not permitted for the official baseline')
    pins = [item for item in parsed.get('Require', []) if item.get('Path') == MODULE]
    if len(pins) != 1 or pins[0].get('Version') != VERSION:
        raise ValueError('SDK go.mod does not pin the approved candidate')


def validate_sums(raw, record):
    rows = set(tuple(line.split()) for line in raw.splitlines())
    if (MODULE, VERSION, record['Sum']) not in rows or (MODULE, VERSION + '/go.mod', record['GoModSum']) not in rows:
        raise ValueError('go.sum is empty, incomplete or inconsistent with downloaded module records')


def validate_graph(graph):
    if any('Replace' in item or item.get('Error') for item in graph):
        raise ValueError('module graph contains replacements or unresolved errors')
    pins = [item for item in graph if item.get('Path') == MODULE]
    if len(pins) != 1 or pins[0].get('Version') != VERSION:
        raise ValueError('resolved module graph changed or omitted the approved candidate')


def decode_stream(raw):
    decoder = json.JSONDecoder()
    records = []
    rest = raw.strip()
    while rest:
        item, end = decoder.raw_decode(rest)
        if not isinstance(item, dict):
            raise ValueError('expected a Go JSON object')
        records.append(item)
        rest = rest[end:].lstrip()
    if not records:
        raise ValueError('empty Go JSON output')
    return records


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def preflight_environment(original):
    env = original.copy()
    env.update(GOENV='off', GOWORK='off', GOFLAGS='', GOTOOLCHAIN='local',
               GOSUMDB='sum.golang.org', GONOSUMDB='', GOINSECURE='',
               GOPRIVATE='', GOAUTH='off')
    return env


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prepare', action='store_true', help='allow go mod tidy in the SDK after the exact download is validated')
    args = parser.parse_args()
    # Imported here so local parser tests remain independent of the SDK imports.
    from tooling import source_hash
    out = ROOT / '.artifacts'
    out.mkdir(exist_ok=True)
    report = {
        'evidence_class': 'OFFICIAL_DEPENDENCY_PREFLIGHT', 'status': 'NOT_RUN',
        'started_at': utc_now(), 'source_before_sha256': source_hash(),
        'module': MODULE, 'version': VERSION, 'commands': [], 'minimum_go': None,
        'sdk_compile_verified': False, 'server_verified': False,
        'scope': 'actual download/go.mod/go.sum/module graph/cache verification only',
    }
    # All dependencies are public. Do not let private-module patterns bypass the
    # checksum database, workspace substitutions or ambient overlay flags.
    env = preflight_environment(os.environ)

    def run(name, command):
        log = out / ('dependency-' + name + '.log')
        started = utc_now()
        try:
            result = subprocess.run(command, cwd=ROOT, env=env, text=True,
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=180)
            code, stdout, stderr = result.returncode, result.stdout, result.stderr
        except subprocess.TimeoutExpired:
            code, stdout, stderr = 124, '', 'command exceeded the preflight deadline\n'
        except OSError:
            code, stdout, stderr = 127, '', 'required command is unavailable\n'
        # Public dependencies only; redact credentials/query portions if a custom
        # module proxy emits them in an error. Do not print any environment values.
        combined = stdout + stderr
        combined = re.sub(r'(https?://)[^/\s\"]*@', r'\1[REDACTED]@', combined)
        combined = re.sub(r'(https?://[^\s\"?]+)\?[^\s\"]+', r'\1?[REDACTED]', combined)
        log.write_text(combined)
        report['commands'].append({'name': name, 'command': command, 'exit_code': code,
                                   'started_at': started, 'finished_at': utc_now(),
                                   'log': str(log.relative_to(ROOT))})
        print(f'{name}: exit {code}', flush=True)
        if code:
            raise ValueError(f'{name} did not complete; inspect the recorded log')
        return stdout

    try:
        report['toolchain'] = json.loads(run('toolchain', ['go', 'env', '-json', 'GOVERSION', 'GOOS', 'GOARCH', 'GOTOOLCHAIN']))
        root_mod = json.loads(run('root-mod', ['go', 'mod', 'edit', '-json']))
        validate_root(root_mod)
        record = json.loads(run('download', ['go', 'mod', 'download', '-json', MODULE + '@' + VERSION]))
        report['download'] = validate_download(record)
        upstream = json.loads(run('upstream-mod', ['go', 'mod', 'edit', '-json', record['GoMod']]))
        report['minimum_go'] = validate_upstream_mod(upstream)
        # Save exact reference bytes, never use them as a replacement dependency.
        (out / 'dependency-upstream-go.mod').write_bytes(pathlib.Path(record['GoMod']).read_bytes())
        if args.prepare:
            run('tidy', ['go', 'mod', 'tidy'])
        validate_root(json.loads(run('final-root-mod', ['go', 'mod', 'edit', '-json'])))
        run('all-downloads', ['go', 'mod', 'download', 'all'])
        graph = decode_stream(run('graph', ['go', 'list', '-m', '-json', 'all']))
        validate_graph(graph)
        run('verify', ['go', 'mod', 'verify'])
        validate_sums((ROOT / 'go.sum').read_text(), record)
        report['go_sum_sha256'] = sha256(ROOT / 'go.sum')
        report['resolved_modules'] = len(graph)
        report['status'] = 'PASS'
    except (ValueError, KeyError, OSError, TypeError) as e:
        report['status'] = 'BLOCKED'
        report['reason'] = str(e) if isinstance(e, ValueError) else 'required metadata or artifact is unavailable'
    finally:
        report['source_sha256'] = source_hash()
        report['finished_at'] = utc_now()
        (out / 'dependency-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print('OFFICIAL_DEPENDENCY_PREFLIGHT: ' + report['status'])
    return 0 if report['status'] == 'PASS' else 1


if __name__ == '__main__':
    sys.exit(main())
