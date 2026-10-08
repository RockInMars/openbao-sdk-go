#!/usr/bin/env python3
"""Opt-in supplemental checks of an explicitly configured external test service.

No fixture lock or release acceptance rule consumes these COMMAND_RECEIPTs.
Credentials are supplied only to the compiled, single-target test process.
"""
import argparse
import hashlib
import json
import os
import pathlib
import re
import sys

from command_runner import succeeded
from reporting import Run, run_cli
from remote_manifest import ManifestError, load_manifest


def build_environment(source):
    allowed = {'PATH', 'SYSTEMROOT', 'WINDIR', 'COMSPEC', 'PATHEXT', 'TEMP', 'TMP',
               'HOME', 'USERPROFILE', 'LOCALAPPDATA', 'APPDATA', 'GOCACHE', 'GOPATH',
               'GOMODCACHE', 'GOROOT', 'CC', 'CXX', 'CGO_ENABLED'}
    env = {k: v for k, v in source.items() if k.upper() in allowed}
    env.update(GOWORK='off', GOFLAGS='', GOTOOLCHAIN='local', GOENV='off', GOPROXY='off')
    return env


REQUIRED = ('ADDRESS', 'CONFIRMED', 'ENVIRONMENT', 'NAMESPACE_MODE', 'TOKEN',
            'KV_MOUNT', 'KV_PATH', 'KV_VERSION', 'KV_MARKER')
OPTIONAL = ('NAMESPACE', 'CA_FILE', 'CLIENT_CERT_FILE', 'CLIENT_KEY_FILE',
             'WRITE_PREFIX', 'ALLOW_WRITE', 'ALLOW_CLEANUP', 'ALLOW_SOFT_DELETE',
             'TRANSIT_MOUNT', 'TRANSIT_KEY', 'TRANSIT_TYPE', 'PKI_MOUNT', 'PKI_ROLE',
             'PKI_DNS', 'PKI_ROOT_FILE', 'ALLOW_SIGN_CSR', 'ALLOW_REVOKE',
             'NEGATIVE_TOKEN', 'NEGATIVE_MOUNT', 'NEGATIVE_PATH', 'FULL', 'FULL_PART',
             'TRANSIT_SIGN_KEY', 'TRANSIT_HMAC_KEY', 'APPROLE_MOUNT',
             'APPROLE_ROLE_ID', 'APPROLE_SECRET_ID', 'MANIFEST', 'MANIFEST_SHA256')
FULL_REQUIRED = ('ALLOW_SOFT_DELETE', 'TRANSIT_MOUNT', 'TRANSIT_KEY', 'TRANSIT_TYPE',
                 'TRANSIT_SIGN_KEY', 'TRANSIT_HMAC_KEY', 'PKI_MOUNT', 'PKI_ROLE',
                 'PKI_DNS', 'PKI_ROOT_FILE', 'ALLOW_SIGN_CSR', 'ALLOW_REVOKE',
                 'NEGATIVE_TOKEN', 'NEGATIVE_MOUNT', 'NEGATIVE_PATH',
                 'APPROLE_MOUNT', 'APPROLE_ROLE_ID', 'APPROLE_SECRET_ID', 'FULL_PART',
                 'MANIFEST', 'MANIFEST_SHA256')


def missing_inputs(env, mode):
    names = list(REQUIRED)
    if env.get('BAO_REMOTE_NAMESPACE_MODE') == 'named': names.append('NAMESPACE')
    if mode == 'isolated': names += ['WRITE_PREFIX', 'ALLOW_WRITE', 'ALLOW_CLEANUP']
    if env.get('BAO_REMOTE_FULL') == 'yes': names += FULL_REQUIRED
    return ['BAO_REMOTE_' + key for key in names if not env.get('BAO_REMOTE_' + key)]


def full_scope_matches(env, run_id):
    try:
        plan = load_manifest(env.get('BAO_REMOTE_MANIFEST'), env.get('BAO_REMOTE_MANIFEST_SHA256', ''))
        part = env.get('BAO_REMOTE_FULL_PART')
        expected = plan.child_environment(part)
    except ManifestError:
        return False
    return (run_id == plan.run_id + '-' + part
            and all(env.get(key) == value for key, value in expected.items()))


def expected_cases(env, mode):
    names = {'health', 'start', 'identity', 'capabilities', 'read_version', 'ready', 'close'}
    full = env.get('BAO_REMOTE_FULL') == 'yes'
    if full:
        names |= {'state_ready', 'state_closed'}
    if full and env.get('BAO_REMOTE_FULL_PART') == 'transit':
        return names | {'transit_rewrap_key', 'transit_rewrap_source', 'transit_rewrap',
                        'transit_rewrap_decrypt', 'transit_sign_key', 'transit_sign', 'transit_verify',
                        'transit_sign_digest', 'transit_verify_digest', 'transit_public_key',
                        'transit_hmac_key', 'transit_hmac', 'transit_hmac_verify'}
    if mode == 'isolated':
        names |= {'empty_path', 'kv_create', 'kv_read_created', 'kv_update', 'kv_stale_cas', 'kv_read_updated', 'kv_cleanup'}
        if env.get('BAO_REMOTE_ALLOW_SOFT_DELETE') == 'yes': names |= {'kv_soft_delete', 'kv_restore', 'kv_read_restored'}
        if env.get('BAO_REMOTE_NEGATIVE_TOKEN'): names |= {'permission_identity', 'permission_negative'}
        if env.get('BAO_REMOTE_TRANSIT_MOUNT'):
            names |= {'transit_existing_key', 'transit_tamper'}
            names |= {'transit_encrypt', 'transit_decrypt'} if env.get('BAO_REMOTE_TRANSIT_TYPE') == 'aes256-gcm96' else {'transit_sign', 'transit_verify'}
        if env.get('BAO_REMOTE_PKI_MOUNT'):
            names |= {'pki_sign_csr', 'pki_verify'}
            if env.get('BAO_REMOTE_ALLOW_REVOKE') == 'yes': names.add('pki_revoke')
        if full:
            names |= {'kv_read_latest', 'kv_read_ref', 'kv_metadata', 'kv_list',
                      'pki_issue', 'pki_read_certificate', 'pki_read_issuer_chain',
                      'pki_issue_revoke', 'approle_start', 'approle_ready', 'approle_close'}
    return names


def verify_result(report, mode, env=None):
    """Check independent required targets; producer-supplied lists are insufficient."""
    limit, duration = (10, 90000) if mode == 'readonly' else (60, 600000)
    if env and env.get('BAO_REMOTE_FULL') == 'yes':
        limit = {'core': 43, 'transit': 32}[env['BAO_REMOTE_FULL_PART']]
    cases = report.get('cases', [])
    names = [item.get('name') for item in cases]
    requests = report.get('requests')
    required = expected_cases(env or {}, mode)
    if (report.get('status') != 'PASS' or type(requests) is not int or not 1 <= requests <= limit
            or not isinstance(report.get('elapsed_ms'), int) or not 0 < report['elapsed_ms'] <= duration
            or len(names) != len(set(names)) or required != set(names)
            or any(item.get('status') != 'PASS' for item in cases)
            or any(item.get('unknown') for item in cases)
            or not isinstance(report.get('resources'), list)):
        raise ValueError('remote evidence incomplete or unsuccessful')
    categories = report.get('request_categories', {})
    allowed_categories = {'kv', 'pki', 'transit', 'health', 'sdk_other', 'raw_kv', 'identity', 'capabilities'}
    if env and env.get('BAO_REMOTE_FULL') == 'yes' and env.get('BAO_REMOTE_FULL_PART') == 'core':
        allowed_categories.add('authn')
    if (not isinstance(categories, dict) or not categories or not set(categories).issubset(allowed_categories)
            or any(type(value) is not int or value < 0 for value in categories.values())
            or sum(categories.values()) != requests):
        raise ValueError('request categories inconsistent')
    context = report.get('target_context', {})
    if context.get('mode') != mode or context.get('environment') != 'nonproduction':
        raise ValueError('target context missing')
    if env:
        for key in ['address', 'environment', 'namespace_mode', 'namespace', 'kv_mount', 'kv_path',
                    'write_prefix', 'transit_mount', 'transit_key', 'transit_sign_key', 'transit_hmac_key',
                    'pki_mount', 'pki_role', 'pki_dns', 'negative_mount', 'negative_path', 'approle_mount', 'full_part']:
            if context.get(key) != env.get('BAO_REMOTE_' + key.upper(), ''):
                raise ValueError('target context mismatch')
        if context.get('kv_version') != int(env['BAO_REMOTE_KV_VERSION']):
            raise ValueError('KV version mismatch')
    if mode == 'readonly' and report['resources']:
        raise ValueError('readonly run has resources')
    if env and context.get('full') is not (env.get('BAO_REMOTE_FULL') == 'yes'):
        raise ValueError('full mode mismatch')
    if env and env.get('BAO_REMOTE_FULL') == 'yes' and (
            not re.fullmatch(r'[a-f0-9]{64}', env.get('BAO_REMOTE_MANIFEST_SHA256', ''))
            or report.get('manifest_sha256') != env['BAO_REMOTE_MANIFEST_SHA256']):
        raise ValueError('manifest evidence mismatch')
    for resource in report['resources']:
        allowed = {'removed'} if resource.get('kind') == 'kv' else {'retained_until_expiry', 'revoked_record_retained'}
        if resource.get('state') not in allowed:
            raise ValueError('resource cleanup unresolved')
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--execute', action='store_true', help='Explicitly permit the configured remote requests')
    parser.add_argument('--mode', choices=['readonly', 'isolated'], default='readonly')
    parser.add_argument('--run-id', help='New evidence identifier; never reused')
    args = parser.parse_args(argv)
    if args.run_id and not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]{0,99}', args.run_id):
        parser.error('invalid run identifier')
    run = Run('remote', 'COMMAND_RECEIPT', run_id=args.run_id)
    run.report.update(scope='REMOTE_SUPPLEMENTAL', mode=args.mode,
                      credential_source='process_environment', target='https://kms.jiup9.com:443')
    if not args.execute:
        return run.finish('BLOCKED', 'explicit --execute required; no remote requests made')
    if os.environ.get('BAO_REMOTE_FULL') == 'yes' and args.mode != 'isolated':
        return run.finish('BLOCKED', 'full checks require isolated mode; no remote requests made')
    if os.environ.get('BAO_REMOTE_FULL') == 'yes' and not full_scope_matches(os.environ, run.report['run_id']):
        return run.finish('BLOCKED', 'full checks require scoped sdk-test configuration; no remote requests made')
    if os.environ.get('BAO_REMOTE_FULL') == 'yes':
        run.report['manifest_sha256'] = os.environ['BAO_REMOTE_MANIFEST_SHA256']
    missing = missing_inputs(os.environ, args.mode)
    if missing:
        run.report['missing_inputs'] = missing
        return run.finish('BLOCKED', 'remote input configuration incomplete; no remote requests made')
    clean = build_environment(os.environ)
    version = run.record.command('go-version', ['go', 'version'], 30, env=clean,
                                 log=run.directory/'go-version.log', environment=True)
    run.save()
    raw = (run.directory/'go-version.log').read_text(encoding='utf-8')
    if not succeeded(version) or not re.fullmatch(r'go version go[\w.+-]+ [\w]+/[\w]+\s*', raw):
        return run.finish('BLOCKED' if version['completion'] == 'unavailable' else 'FAIL', 'Go probe failed')
    run.report['tool_versions']['go'] = raw.strip()
    binary = run.directory/('remote.test.exe' if os.name == 'nt' else 'remote.test')
    built = run.command('build', ['go', 'test', '-mod=readonly', '-c', '-tags=remote', '-o', str(binary), './tests/remote'], 180, env=clean)
    if not succeeded(built): return run.finish('FAIL', 'remote binary build did not succeed')
    runtime = clean.copy()
    for key in REQUIRED + OPTIONAL:
        name = 'BAO_REMOTE_' + key
        if name in os.environ: runtime[name] = os.environ[name]
    runtime.update(BAO_REMOTE_EXECUTE='yes', BAO_REMOTE_MODE=args.mode,
                   BAO_REMOTE_RUN_ID=run.report['run_id'], BAO_REMOTE_OUTPUT_DIR=str(run.directory))
    # Never persist stdout/stderr of the credential-bearing process, including panics.
    # The harness writes only a fixed structured projection of each result.
    duration = 95 if args.mode == 'readonly' else 605
    if os.environ.get('BAO_REMOTE_FULL') == 'yes':
        try:
            plan = load_manifest(runtime['BAO_REMOTE_MANIFEST'], runtime['BAO_REMOTE_MANIFEST_SHA256'])
            duration = min(duration, max(1, int(plan.remaining_seconds())))
        except ManifestError:
            runtime.clear()
            return run.finish('BLOCKED', 'manifest expired before execution; no remote requests made')
    try:
        receipt = run.record.command('remote', [str(binary), '-test.run=^TestRemoteService$', '-test.count=1',
                                     '-test.timeout='+str(duration)+'s'], duration+5,
                                    log=run.directory/'remote.log', env=runtime, discard_output=True)
    finally:
        runtime.clear()
    run.save()
    if not succeeded(receipt): return run.finish('FAIL', 'remote process did not naturally succeed; inspect checkpoints for resource ownership')
    try:
        path = run.directory/'result.json'
        raw = path.read_bytes()
        report = verify_result(json.loads(raw), args.mode, os.environ)
        run.report.update(result_path=path.relative_to(run.root).as_posix(), result_sha256=hashlib.sha256(raw).hexdigest(),
                          requests=report['requests'], executed_targets=[x['name'] for x in report['cases']],
                          server_version=report.get('server_version', ''), resources=report['resources'])
        run.report.update(target_context=report['target_context'], request_categories=report['request_categories'])
    except (OSError, ValueError, TypeError, KeyError):
        return run.finish('FAIL', 'structured result missing or rejected; inspect checkpoints')
    return run.finish('PASS')


if __name__ == '__main__':
    sys.exit(run_cli(main))
