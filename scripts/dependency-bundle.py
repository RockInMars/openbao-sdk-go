#!/usr/bin/env python3
"""Export/verify public Go dependencies without changing this SDK or trusting seeded sums.

export: download in fresh caches, authenticate with Go's checksum database, then
replay from a local file proxy in another fresh cache before publishing an archive.
verify: validate transfer integrity, bind to local SDK inputs, and repeat Go's
checksum authentication using the archived signed records. No SDK build is claimed.
"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from urllib.parse import urlsplit
import zipfile

ROOT = Path(__file__).resolve().parents[1]
MAX_FILE_BYTES = 512 * 1024 * 1024
MAX_TOTAL_BYTES = 2 * 1024 * 1024 * 1024
MAX_FILES = 20000
FORMAT = 'openbao-public-dependencies-v1'
SUMDB = 'sum.golang.org'

_spec = importlib.util.spec_from_file_location('dependency_preflight', ROOT / 'scripts/dependency-check.py')
preflight = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(preflight)
MODULE, VERSION = preflight.MODULE, preflight.VERSION


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def strict_json(text):
    def object_pairs(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError('duplicate JSON key')
            result[key] = value
        return result
    return json.loads(text, object_pairs_hook=object_pairs)


def public_proxy(value):
    if any(c in value for c in ',|\\%\r\n\t '):
        raise ValueError('use one explicit HTTPS proxy; no fallback, escaping or credentials')
    u = urlsplit(value)
    if (u.scheme != 'https' or not u.hostname or u.username or u.password or
            u.query or u.fragment or '..' in u.path.split('/') or '.' in u.path.split('/')):
        raise ValueError('use one explicit HTTPS proxy; no fallback, escaping or credentials')
    _ = u.port  # reject malformed ports
    return value.rstrip('/')


def clean_environment(original, scratch, proxy):
    # A fresh HOME prevents netrc and personal Go settings from being consulted.
    allowed = ('PATH', 'SYSTEMROOT', 'WINDIR', 'TMPDIR', 'TEMP', 'TMP', 'LANG', 'LC_ALL',
               'SSL_CERT_FILE', 'SSL_CERT_DIR')
    env = {k: original[k] for k in allowed if k in original}
    env.update(HOME=str(scratch/'home'), USERPROFILE=str(scratch/'home'),
               APPDATA=str(scratch/'config'), XDG_CONFIG_HOME=str(scratch/'config'),
               GOENV='off', GOWORK='off', GOFLAGS='', GO111MODULE='on', GOTOOLCHAIN='local',
               GOSUMDB=SUMDB, GONOSUMDB='', GONOPROXY='', GOPRIVATE='', GOINSECURE='',
               GOAUTH='off', GOVCS='*:off', GOPROXY=proxy,
               GOPATH=str(scratch/'gopath'), GOMODCACHE=str(scratch/'modcache'),
               GOCACHE=str(scratch/'buildcache'))
    return env


def input_files(root):
    root = root.resolve()
    result = []
    for current, dirs, names in os.walk(root):
        p = Path(current)
        if p != root and 'go.mod' in names:
            dirs[:] = []  # separate consumer modules are not part of this graph
            continue
        chosen = []
        for name in dirs:
            if name.startswith('.') or name in ('vendor', '__pycache__'):
                continue
            if (p/name).is_symlink():
                raise ValueError('source input directory is a symlink')
            chosen.append(name)
        dirs[:] = chosen
        for name in names:
            if name.endswith('.go') or (p == root and name in ('go.mod', 'go.sum')):
                file = p/name
                if file.is_symlink() or not stat.S_ISREG(file.stat().st_mode):
                    raise ValueError('source input is not a regular file')
                result.append(file)
    if root/'go.mod' not in result:
        raise ValueError('SDK go.mod is missing')
    return sorted(result)


def input_fingerprint(root):
    root = root.resolve()
    h = hashlib.sha256()
    for p in input_files(root):
        h.update(p.relative_to(root).as_posix().encode()); h.update(b'\0')
        h.update(p.read_bytes()); h.update(b'\0')
    return h.hexdigest()


def copy_sdk_inputs(root, destination):
    root = root.resolve()
    paths = input_files(root)  # validate all inputs before creating the destination
    destination.mkdir(parents=True, exist_ok=False)
    for p in paths:
        if p == root/'go.sum':
            continue  # authenticate from the signed log, not caller-provided h1 values
        out = destination/p.relative_to(root)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_bytes(p.read_bytes())


def allowed_payload(name):
    if (not isinstance(name, str) or not name or len(name) > 1024 or
            not re.fullmatch(r'[A-Za-z0-9!@._+~/\-]+', name)):
        return False
    p = PurePosixPath(name)
    if p.is_absolute() or name != p.as_posix() or any(x in ('.', '..') or x.startswith('.') for x in p.parts):
        return False
    if name in ('resolved/go.mod', 'resolved/go.sum'):
        return True
    prefix = f'proxy/sumdb/{SUMDB}/'
    if name.startswith(prefix):
        tail = name[len(prefix):]
        return tail in ('supported', 'latest') or tail.startswith(('lookup/', 'tile/'))
    if not name.startswith('proxy/') or name.startswith('proxy/sumdb/'):
        return False
    parts = p.parts
    if '@v' not in parts or parts.index('@v') != len(parts)-2:
        return False
    return p.name == 'list' or bool(re.fullmatch(r'v[0-9][A-Za-z0-9.+_-]*\.(mod|info|zip)', p.name))


def file_rows(stage):
    rows = {}
    total = 0
    for p in sorted(stage.rglob('*')):
        if p.is_symlink():
            raise ValueError('payload symlinks are not permitted')
        if p.is_dir():
            continue
        name = p.relative_to(stage).as_posix()
        if not stat.S_ISREG(p.stat().st_mode) or not allowed_payload(name):
            raise ValueError('payload contains a disallowed file')
        size = p.stat().st_size
        total += size
        if size > MAX_FILE_BYTES or total > MAX_TOTAL_BYTES or len(rows) >= MAX_FILES:
            raise ValueError('dependency bundle exceeds transfer limits')
        rows[name] = {'bytes': size, 'sha256': preflight.sha256(p)}
    required = ('resolved/go.mod', 'resolved/go.sum', f'proxy/sumdb/{SUMDB}/supported')
    if any(n not in rows for n in required):
        raise ValueError('bundle is missing required module or sumdb files')
    return rows


def write_bundle(stage, target, metadata):
    if os.path.lexists(target) or os.path.lexists(str(target)+'.sha256'):
        raise FileExistsError('output already exists; use a new output name')
    target.parent.mkdir(parents=True, exist_ok=True)
    manifest = {**metadata, 'format': FORMAT, 'files': file_rows(stage)}
    with tempfile.TemporaryDirectory(prefix='.bundle-write-', dir=target.parent) as td:
        temp = Path(td)/'bundle.zip'
        with zipfile.ZipFile(temp, 'w', zipfile.ZIP_DEFLATED) as z:
            z.writestr('manifest.json', json.dumps(manifest, sort_keys=True, indent=2)+'\n')
            for name in manifest['files']:
                z.write(stage/name, name)
        # Link provides exclusive publication: unlike replace(), it never overwrites.
        os.link(temp, target)
    with Path(str(target)+'.sha256').open('x') as f:
        f.write(preflight.sha256(target)+'  '+target.name+'\n')


def unpack_bundle(archive, expected_sha256, destination):
    if not re.fullmatch(r'[a-f0-9]{64}', expected_sha256) or preflight.sha256(archive) != expected_sha256:
        raise ValueError('archive transfer SHA256 mismatch')
    if os.path.lexists(destination):
        raise FileExistsError('extraction destination already exists')
    with zipfile.ZipFile(archive) as z:
        infos = z.infolist()
        names = [i.filename for i in infos]
        if (len(infos) > MAX_FILES+1 or len(set(names)) != len(names) or
                len({n.casefold() for n in names}) != len(names)):
            raise ValueError('duplicate or too many archive entries')
        total = 0
        for i in infos:
            kind = stat.S_IFMT(i.external_attr >> 16)
            if (i.is_dir() or kind not in (0, stat.S_IFREG) or i.flag_bits & 1 or
                    (i.filename != 'manifest.json' and not allowed_payload(i.filename))):
                raise ValueError('unsafe archive entry')
            total += i.file_size
            if i.file_size > MAX_FILE_BYTES or total > MAX_TOTAL_BYTES:
                raise ValueError('archive exceeds transfer limits')
        if 'manifest.json' not in names or z.getinfo('manifest.json').file_size > 8*1024*1024:
            raise ValueError('missing or oversized manifest')
        manifest = strict_json(z.read('manifest.json'))
        if (not isinstance(manifest, dict) or manifest.get('format') != FORMAT or
                not re.fullmatch(r'[a-f0-9]{64}', manifest.get('input_sha256', '')) or
                not isinstance(manifest.get('files'), dict)):
            raise ValueError('invalid bundle manifest')
        rows = manifest['files']
        if set(rows) != set(names)-{'manifest.json'}:
            raise ValueError('manifest does not cover the exact payload')
        for name, row in rows.items():
            if (not isinstance(row, dict) or type(row.get('bytes')) is not int or
                    row['bytes'] != z.getinfo(name).file_size or
                    not re.fullmatch(r'[a-f0-9]{64}', row.get('sha256', ''))):
                raise ValueError('invalid file record')
            h = hashlib.sha256()
            with z.open(name) as f:
                for block in iter(lambda: f.read(1024*1024), b''):
                    h.update(block)
            if h.hexdigest() != row['sha256']:
                raise ValueError('payload SHA256 mismatch')
        # The caller owns a fresh temporary parent; no archive-controlled links exist.
        destination.mkdir(parents=True, exist_ok=False)
        for name in rows:
            out = destination/name
            out.parent.mkdir(parents=True, exist_ok=True)
            with z.open(name) as src, out.open('xb') as dst:
                shutil.copyfileobj(src, dst, 1024*1024)
    file_rows(destination)
    return manifest  # integrity only; the Go replay below establishes authenticity


def run_go(name, args, work, env, report, logs):
    logs.mkdir(parents=True, exist_ok=True)
    start = now()
    try:
        result = subprocess.run(['go', *args], cwd=work, env=env, capture_output=True,
                                text=True, encoding='utf-8', timeout=180)
        code, stdout, stderr = result.returncode, result.stdout, result.stderr
    except subprocess.TimeoutExpired:
        code, stdout, stderr = 124, '', 'Go command exceeded the deadline\n'
    except OSError:
        code, stdout, stderr = 127, '', 'Go command is unavailable\n'
    safe = re.sub(r'(https?://)[^/\s\"]*@', r'\1[REDACTED]@', stdout+stderr)
    safe = re.sub(r'(https?://[^\s\"?]+)\?[^\s\"]+', r'\1?[REDACTED]', safe)
    (logs/(name+'.log')).write_text(safe, encoding='utf-8')
    report['commands'].append({'name': name, 'command': ['go', *args], 'exit_code': code,
                               'started_at': start, 'finished_at': now(), 'log': name+'.log'})
    print(name+': exit '+str(code), flush=True)
    if code:
        raise ValueError(name+' failed; inspect its recorded log')
    return stdout


def module_records(raw):
    records = []
    for r in preflight.decode_stream(raw):
        path, version = r.get('Path', ''), r.get('Version', '')
        if (r.get('Error') or 'Replace' in r or not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._~/-]+', path) or
                '.' not in path.split('/')[0] or
                not re.fullmatch(r'v\d+\.\d+\.\d+(?:-[A-Za-z0-9.-]+)?(?:\+incompatible)?', version)):
            raise ValueError('invalid public module download record')
        records.append({'Path': path, 'Version': version, 'Sum': preflight.sum_value(r.get('Sum')),
                        'GoModSum': preflight.sum_value(r.get('GoModSum'))})
    identities = [(x['Path'], x['Version']) for x in records]
    if len(set(identities)) != len(identities):
        raise ValueError('duplicate module records')
    if (MODULE, VERSION) not in identities:
        raise ValueError('official candidate is missing from resolved downloads')
    return sorted(records, key=lambda x: (x['Path'], x['Version']))


def collect_proxy(scratch, stage):
    cache = scratch/'modcache/cache/download'
    for p in sorted(cache.rglob('*')):
        if p.is_symlink():
            raise ValueError('fresh download cache contains a symlink')
        if not p.is_file():
            continue
        name = 'proxy/'+p.relative_to(cache).as_posix()
        if allowed_payload(name):
            out = stage/name; out.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(p, out)
    db = stage/'proxy/sumdb'/SUMDB
    db.mkdir(parents=True, exist_ok=True)
    (db/'supported').write_bytes(b'')
    latest = scratch/'gopath/pkg/sumdb'/SUMDB/'latest'
    if latest.is_file() and not latest.is_symlink():
        shutil.copyfile(latest, db/'latest')
    if not (db/'lookup').is_dir() or not (db/'tile').is_dir():
        raise ValueError('signed checksum lookup records or Merkle tiles are missing')


def offline_replay(stage, scratch, expected_modules, report, logs):
    work = scratch/'work'; work.mkdir(parents=True)
    env = clean_environment(os.environ, scratch, (stage/'proxy').resolve().as_uri())
    (work/'go.mod').write_bytes((stage/'resolved/go.mod').read_bytes())
    # Disable sidecars before commands that may exit before this scratch is removed.
    run_go('replay-telemetry-off', ['telemetry', 'off'], work, env, report, logs)
    # Never install supplied go.sum or .ziphash into the verification workspace.
    # Go must consult the signed sumdb records and recompute module hashes.
    parsed = strict_json(run_go('replay-root', ['mod', 'edit', '-json'], work, env, report, logs))
    preflight.validate_root(parsed)
    actual = module_records(run_go('replay-downloads', ['mod', 'download', '-json', 'all'], work, env, report, logs))
    if actual != expected_modules:
        raise ValueError('authenticated replay differs from exported module records')
    graph = preflight.decode_stream(run_go('replay-graph', ['list', '-m', '-json', 'all'], work, env, report, logs))
    preflight.validate_graph(graph)
    run_go('replay-verify', ['mod', 'verify'], work, env, report, logs)
    exported = {tuple(x.split()) for x in (stage/'resolved/go.sum').read_text(encoding='utf-8').splitlines()}
    generated = {tuple(x.split()) for x in (work/'go.sum').read_text(encoding='utf-8').splitlines()}
    if not generated or not generated.issubset(exported):
        raise ValueError('exported go.sum differs from authenticated replay')


def new_report(kind):
    return {'evidence_class': kind, 'status': 'NOT_RUN', 'started_at': now(), 'commands': [],
            'sdk_compile_verified': False, 'server_verified': False, 'module': MODULE, 'version': VERSION}


def save_report(report, logs):
    logs.mkdir(parents=True, exist_ok=True)
    report['finished_at'] = now()
    (logs/'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n', encoding='utf-8')
    print(report['evidence_class']+': '+report['status'], flush=True)
    return report


def export_bundle(root, target, proxy, logs):
    report = new_report('PUBLIC_DEPENDENCY_EXPORT')
    try:
        proxy = public_proxy(proxy)
        if os.path.lexists(target) or os.path.lexists(str(target)+'.sha256'):
            raise ValueError('output exists; use a new output name')
        before = input_fingerprint(root); report['input_sha256'] = before
        with tempfile.TemporaryDirectory(prefix='openbao-deps-export-') as td:
            scratch = Path(td); work = scratch/'work'; stage = scratch/'payload'
            copy_sdk_inputs(root, work)
            env = clean_environment(os.environ, scratch, proxy)
            run_go('export-telemetry-off', ['telemetry', 'off'], work, env, report, logs)
            tc = strict_json(run_go('toolchain', ['env', '-json', 'GOVERSION', 'GOOS', 'GOARCH'], work, env, report, logs))
            report['toolchain'] = tc
            parsed = strict_json(run_go('root', ['mod', 'edit', '-json'], work, env, report, logs))
            preflight.validate_root(parsed)
            record = strict_json(run_go('official-download', ['mod', 'download', '-json', MODULE+'@'+VERSION], work, env, report, logs))
            report['official_download'] = preflight.validate_download(record)
            upstream = strict_json(run_go('upstream-mod', ['mod', 'edit', '-json', record['GoMod']], work, env, report, logs))
            minimum = preflight.validate_upstream_mod(upstream); report['minimum_go'] = minimum
            run_go('tidy', ['mod', 'tidy'], work, env, report, logs)
            preflight.validate_root(strict_json(run_go('resolved-root', ['mod', 'edit', '-json'], work, env, report, logs)))
            records = module_records(run_go('downloads', ['mod', 'download', '-json', 'all'], work, env, report, logs))
            preflight.validate_graph(preflight.decode_stream(run_go('graph', ['list', '-m', '-json', 'all'], work, env, report, logs)))
            run_go('verify', ['mod', 'verify'], work, env, report, logs)
            preflight.validate_sums((work/'go.sum').read_text(encoding='utf-8'), record)
            collect_proxy(scratch, stage)
            (stage/'resolved').mkdir()
            for name in ('go.mod', 'go.sum'):
                shutil.copyfile(work/name, stage/'resolved'/name)
            offline_replay(stage, scratch/'replay', records, report, logs)
            if input_fingerprint(root) != before:
                raise ValueError('SDK inputs changed during export; retry from a stable snapshot')
            metadata = {'input_sha256': before, 'go_version': tc['GOVERSION'], 'toolchain': tc,
                        'minimum_go': minimum, 'modules': records, 'created_at': now()}
            write_bundle(stage, target, metadata)
            report.update(status='PASS', modules=len(records), archive_sha256=preflight.sha256(target),
                          signed_sumdb_replay_verified=True)
    except (ValueError, OSError, KeyError, TypeError, zipfile.BadZipFile):
        report['status'] = 'BLOCKED'
        report['reason'] = 'Export incomplete; inspect logs. No dependency success or SDK build is claimed.'
    return save_report(report, logs)


def verify_bundle(root, archive, expected_sha256, destination, logs):
    report = new_report('OFFLINE_DEPENDENCY_REPLAY')
    try:
        if os.path.lexists(destination):
            raise ValueError('destination exists; use a fresh directory')
        before = input_fingerprint(root); report['input_sha256'] = before
        with tempfile.TemporaryDirectory(prefix='openbao-deps-verify-') as td:
            scratch = Path(td); stage = scratch/'payload'
            manifest = unpack_bundle(archive, expected_sha256, stage)
            if manifest['input_sha256'] != before:
                raise ValueError('bundle does not match the local SDK source inputs')
            env = clean_environment(os.environ, scratch/'probe', (stage/'proxy').as_uri())
            run_go('verify-telemetry-off', ['telemetry', 'off'], stage, env, report, logs)
            tc = strict_json(run_go('toolchain', ['env', '-json', 'GOVERSION', 'GOOS', 'GOARCH'], stage, env, report, logs))
            report['toolchain'] = tc
            if tc['GOVERSION'] != manifest.get('go_version'):
                raise ValueError('use the recorded fixed Go version; automatic downloads are disabled')
            records = module_records('\n'.join(json.dumps(x) for x in manifest['modules']))
            offline_replay(stage, scratch/'replay', records, report, logs)
            if input_fingerprint(root) != before:
                raise ValueError('SDK inputs changed during verification')
            destination.mkdir(parents=True, exist_ok=False)
            shutil.copytree(stage, destination, dirs_exist_ok=True)
            report.update(status='PASS', archive_sha256=expected_sha256, modules=len(records),
                          signed_sumdb_replay_verified=True)
    except (ValueError, OSError, KeyError, TypeError, zipfile.BadZipFile):
        report['status'] = 'BLOCKED'
        report['reason'] = 'Verification incomplete; inspect logs, source identity, fixed toolchain and archive integrity.'
    return save_report(report, logs)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='action', required=True)
    ex = sub.add_parser('export', help='run on a network-enabled development machine')
    ex.add_argument('--output', type=Path, required=True)
    ex.add_argument('--proxy', default='https://proxy.golang.org')
    ve = sub.add_parser('verify', help='verify transferred bytes and independently authenticate with Go')
    ve.add_argument('--bundle', type=Path, required=True)
    ve.add_argument('--sha256', required=True)
    ve.add_argument('--destination', type=Path, required=True)
    args = parser.parse_args()
    logs = ROOT/'.artifacts'/('dependency-bundle-'+now().replace(':', '-'))
    if args.action == 'export':
        report = export_bundle(ROOT, args.output.resolve(), args.proxy, logs)
    else:
        report = verify_bundle(ROOT, args.bundle.resolve(), args.sha256, args.destination.resolve(), logs)
    print('Report: '+str(logs/'report.json'))
    return 0 if report['status'] == 'PASS' else 1


if __name__ == '__main__':
    sys.exit(main())
