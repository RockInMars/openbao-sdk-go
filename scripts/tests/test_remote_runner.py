import importlib.util
import datetime
import hashlib
import json
import os
import pathlib
import sys
import unittest
import tempfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]


class RemoteRunnerTests(unittest.TestCase):
    def setUp(self):
        self.path_patch = mock.patch.object(sys, 'path', [str(ROOT/'scripts'), *sys.path])
        self.path_patch.start()
        self.addCleanup(self.path_patch.stop)
        spec = importlib.util.spec_from_file_location('remote_runner_test_target', ROOT/'scripts/remote-test.py')
        self.runner = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.runner)

    def test_build_environment_never_inherits_credentials(self):
        env = self.runner.build_environment({'PATH': 'compiler-path', 'BAO_REMOTE_TOKEN': 'synthetic-secret',
                                             'VAULT_TOKEN': 'synthetic-secret', 'UNRELATED_TOKEN': 'synthetic-secret'})
        self.assertEqual(env['PATH'], 'compiler-path')
        self.assertEqual(env['GOPROXY'], 'off')
        self.assertFalse(any('TOKEN' in key or key.startswith('BAO_REMOTE') for key in env))

    def test_full_scope_accepts_fresh_manifest_instead_of_historical_run(self):
        now = datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0)
        run_id = 'remote-sdk-20261001-0123456789abcdef'
        prefix = run_id.removeprefix('remote-')
        plan = {'schema_version': 1, 'target_allowlist': ['https://kms.jiup9.com:443'],
                'target': 'https://kms.jiup9.com:443', 'environment': 'nonproduction',
                'namespace_mode': 'named', 'namespace': 'sdk-test', 'run_id': run_id,
                'resource_prefix': prefix, 'bootstrap_mount': 'auth/approle',
                'issued_at': (now-datetime.timedelta(seconds=1)).isoformat().replace('+00:00', 'Z'),
                'expires_at': (now+datetime.timedelta(minutes=19)).isoformat().replace('+00:00', 'Z'),
                'request_budget': {'total': 120, 'core': 43, 'transit': 32, 'cleanup': 15},
                'resource_budget': 9, 'ownership': 'create_only_with_identity',
                'cleanup': 'reverse_created_resources_before_expiry',
                'allowed_operations': ['bootstrap_approle', 'create_isolated_mounts',
                    'create_acl_policies_cas', 'create_runtime_tokens', 'seed_kv_cas',
                    'configure_owned_transit_pki_approle', 'run_sdk_core', 'run_sdk_transit',
                    'cleanup_owned_resources']}
        expected = {'ADDRESS': plan['target'], 'ENVIRONMENT': 'nonproduction',
                    'NAMESPACE_MODE': 'named', 'NAMESPACE': 'sdk-test', 'WRITE_PREFIX': 'sdk-validation',
                    'KV_MOUNT': prefix+'-kv', 'KV_PATH': f'sdk-validation/{run_id}-core/seed',
                    'KV_MARKER': 'owned-'+run_id, 'NEGATIVE_MOUNT': prefix+'-kv',
                    'NEGATIVE_PATH': f'sdk-validation/{run_id}-core/seed',
                    'TRANSIT_MOUNT': prefix+'-transit', 'TRANSIT_KEY': 'cipher',
                    'TRANSIT_SIGN_KEY': 'sign', 'TRANSIT_HMAC_KEY': 'mac', 'PKI_MOUNT': prefix+'-pki',
                    'PKI_ROLE': 'device', 'PKI_DNS': 'node.sdk-test.invalid',
                    'APPROLE_MOUNT': prefix+'-approle', 'FULL_PART': 'core'}
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory)/'manifest.json'
            raw = json.dumps(plan).encode()
            path.write_bytes(raw)
            env = {'BAO_REMOTE_'+k: v for k, v in expected.items()}
            env.update(BAO_REMOTE_MANIFEST=str(path), BAO_REMOTE_MANIFEST_SHA256=hashlib.sha256(raw).hexdigest())
            self.assertTrue(self.runner.full_scope_matches(env, run_id+'-core'))
            for key, value in [('BAO_REMOTE_NAMESPACE', 'root'), ('BAO_REMOTE_KV_MOUNT', 'business'),
                               ('BAO_REMOTE_MANIFEST_SHA256', '0'*64), ('BAO_REMOTE_NEGATIVE_PATH', 'other')]:
                self.assertFalse(self.runner.full_scope_matches({**env, key: value}, run_id+'-core'))

    def test_missing_remote_inputs_fail_closed(self):
        missing = self.runner.missing_inputs({}, 'readonly')
        self.assertIn('BAO_REMOTE_TOKEN', missing)
        self.assertIn('BAO_REMOTE_NAMESPACE_MODE', missing)
        self.assertIn('BAO_REMOTE_KV_VERSION', missing)

    def test_result_requires_real_targets_and_budget(self):
        for result in [{}, {'status': 'PASS', 'cases': [], 'requests': 0},
                       {'status': 'PASS', 'cases': [{'name': 'health', 'status': 'PASS'}], 'requests': 11}]:
            with self.assertRaises(ValueError):
                self.runner.verify_result(result, 'readonly')

    def test_valid_receipt_targets_are_checked_independently(self):
        report = {'status': 'PASS', 'requests': 5, 'elapsed_ms': 120, 'resources': [],
                  'request_categories': {'health': 1, 'identity': 1, 'capabilities': 1, 'kv': 2},
                  'target_context': {'mode': 'readonly', 'environment': 'nonproduction'},
                  'cases': [{'name': name, 'status': 'PASS'} for name in
                            ['health', 'start', 'identity', 'capabilities', 'read_version', 'ready', 'close']]}
        self.runner.verify_result(report, 'readonly')
        report['cases'][0]['status'] = 'BLOCKED'
        with self.assertRaises(ValueError): self.runner.verify_result(report, 'readonly')

    def test_full_core_authn_attempts_are_counted_without_relaxing_other_modes(self):
        env = {'BAO_REMOTE_FULL': 'yes', 'BAO_REMOTE_FULL_PART': 'core',
               'BAO_REMOTE_ENVIRONMENT': 'nonproduction',
               'BAO_REMOTE_MANIFEST_SHA256': 'a'*64,
               'BAO_REMOTE_KV_VERSION': '1'}
        context = {'mode': 'isolated', 'environment': 'nonproduction', 'full': True,
                   'kv_version': 1}
        for key in ['address', 'environment', 'namespace_mode', 'namespace', 'kv_mount', 'kv_path',
                    'write_prefix', 'transit_mount', 'transit_key', 'transit_sign_key', 'transit_hmac_key',
                    'pki_mount', 'pki_role', 'pki_dns', 'negative_mount', 'negative_path', 'approle_mount', 'full_part']:
            context[key] = env.get('BAO_REMOTE_' + key.upper(), '')
        report = {'status': 'PASS', 'requests': 3, 'elapsed_ms': 120, 'resources': [],
                  'manifest_sha256': 'a'*64,
                  'request_categories': {'health': 1, 'kv': 1, 'authn': 1},
                  'target_context': context,
                  'cases': [{'name': name, 'status': 'PASS'} for name in
                            self.runner.expected_cases(env, 'isolated')]}
        self.runner.verify_result(report, 'isolated', env)
        report['requests'] = 43
        report['request_categories'] = {'health': 1, 'kv': 41, 'authn': 1}
        self.runner.verify_result(report, 'isolated', env)
        report['requests'] = 44
        report['request_categories']['kv'] = 42
        with self.assertRaisesRegex(ValueError, 'remote evidence incomplete'):
            self.runner.verify_result(report, 'isolated', env)
        report['requests'] = 3
        report['request_categories'] = {'health': 1, 'kv': 1, 'authn': 1}
        report['request_categories']['authn'] = 2
        with self.assertRaisesRegex(ValueError, 'request categories inconsistent'):
            self.runner.verify_result(report, 'isolated', env)
        transit_env = {**env, 'BAO_REMOTE_FULL_PART': 'transit'}
        transit_report = {**report, 'request_categories': {'health': 1, 'kv': 1, 'authn': 1},
                          'target_context': {**context, 'full_part': 'transit'},
                          'cases': [{'name': name, 'status': 'PASS'} for name in
                                    self.runner.expected_cases(transit_env, 'isolated')]}
        with self.assertRaisesRegex(ValueError, 'request categories inconsistent'):
            self.runner.verify_result(transit_report, 'isolated', transit_env)

    def test_optional_targets_cannot_disappear(self):
        env = {'BAO_REMOTE_TRANSIT_MOUNT': 'transit', 'BAO_REMOTE_TRANSIT_TYPE': 'aes256-gcm96',
               'BAO_REMOTE_ALLOW_SOFT_DELETE': 'yes', 'BAO_REMOTE_PKI_MOUNT': 'pki', 'BAO_REMOTE_ALLOW_REVOKE': 'yes'}
        required = self.runner.expected_cases(env, 'isolated')
        self.assertTrue({'transit_tamper', 'kv_restore', 'pki_revoke'}.issubset(required))

    def test_full_mode_requires_inputs_and_independent_targets(self):
        env = {'BAO_REMOTE_FULL': 'yes', 'BAO_REMOTE_FULL_PART': 'core'}
        missing = self.runner.missing_inputs(env, 'isolated')
        self.assertIn('BAO_REMOTE_TRANSIT_SIGN_KEY', missing)
        self.assertIn('BAO_REMOTE_APPROLE_SECRET_ID', missing)
        required = self.runner.expected_cases(env, 'isolated')
        self.assertTrue({'kv_read_latest', 'kv_read_ref', 'kv_metadata', 'kv_list',
                         'pki_issue', 'pki_read_certificate', 'pki_read_issuer_chain',
                         'approle_start', 'approle_ready', 'approle_close'}.issubset(required))
        self.assertNotIn('transit_hmac_verify', required)
        transit = self.runner.expected_cases({**env, 'BAO_REMOTE_FULL_PART': 'transit'}, 'isolated')
        self.assertTrue({'transit_rewrap', 'transit_sign_digest', 'transit_hmac_verify'}.issubset(transit))
        self.assertNotIn('kv_create', transit)
        self.assertNotIn('pki_issue', transit)

    def test_full_scope_rejects_other_namespace_or_resource(self):
        run_id = 'remote-sdk-full-20260930-07-core'
        env = {'BAO_REMOTE_FULL_PART': 'core'}
        expected = {'NAMESPACE_MODE': 'named', 'NAMESPACE': 'sdk-test',
                    'WRITE_PREFIX': 'sdk-validation', 'KV_MOUNT': 'sdk-codex-20260930-68edb3eb-kv',
                    'KV_PATH': f'sdk-validation/{run_id}/seed',
            'KV_MARKER': 'owned-remote-sdk-full-20260930-07',
                    'NEGATIVE_MOUNT': 'sdk-codex-20260930-68edb3eb-kv',
                    'NEGATIVE_PATH': f'sdk-validation/{run_id}/seed',
                    'TRANSIT_MOUNT': 'sdk-codex-20260930-68edb3eb-transit',
                    'TRANSIT_KEY': 'cipher', 'TRANSIT_SIGN_KEY': 'sign', 'TRANSIT_HMAC_KEY': 'mac',
                    'PKI_MOUNT': 'sdk-codex-20260930-68edb3eb-pki', 'PKI_ROLE': 'device',
                    'PKI_DNS': 'node.sdk-test.invalid',
                    'APPROLE_MOUNT': 'sdk-codex-20260930-68edb3eb-approle'}
        env.update({'BAO_REMOTE_' + key: value for key, value in expected.items()})
        self.assertFalse(self.runner.full_scope_matches(env, run_id))
        for key, value in [('BAO_REMOTE_NAMESPACE', 'root'), ('BAO_REMOTE_KV_MOUNT', 'business'),
                           ('BAO_REMOTE_NEGATIVE_PATH', 'other')]:
            changed = dict(env)
            changed[key] = value
            self.assertFalse(self.runner.full_scope_matches(changed, run_id))
        self.assertFalse(self.runner.full_scope_matches(env, run_id + '-other'))
        old = dict(env)
        old_id = 'remote-sdk-full-20260930-06-core'
        old['BAO_REMOTE_KV_PATH'] = f'sdk-validation/{old_id}/seed'
        old['BAO_REMOTE_NEGATIVE_PATH'] = old['BAO_REMOTE_KV_PATH']
        old['BAO_REMOTE_KV_MARKER'] = 'owned-remote-sdk-full-20260930-06'
        self.assertFalse(self.runner.full_scope_matches(old, old_id))

    def test_cli_missing_inputs_does_not_probe_or_build(self):
        with tempfile.TemporaryDirectory() as directory:
            real_run = self.runner.Run
            create = lambda *args, **kwargs: real_run(*args, root=directory, **kwargs)
            with mock.patch.dict(os.environ, {}, clear=True), mock.patch.object(self.runner, 'Run', side_effect=create), \
                 mock.patch('reporting.RunRecord.command', side_effect=AssertionError('must not start a process')):
                self.assertEqual(self.runner.main(['--execute', '--run-id', 'missing-inputs']), 2)
                with self.assertRaises(FileExistsError): self.runner.main(['--execute', '--run-id', 'missing-inputs'])

    def test_credential_process_output_is_discarded_before_persistence(self):
        from command_runner import run_command
        with tempfile.TemporaryDirectory() as directory:
            log = pathlib.Path(directory)/'command.log'
            receipt = run_command([sys.executable, '-c', "import os; print(os.environ['TEST_SECRET'])"],
                                  cwd=directory, env={**self.runner.build_environment(os.environ), 'TEST_SECRET': 'synthetic-secret'},
                                  log=log, timeout_s=5, discard_output=True)
            self.assertEqual(receipt['exit_code'], 0)
            self.assertTrue(receipt['output_suppressed'])
            self.assertNotIn('synthetic-secret', log.read_text(encoding='utf-8'))

    def test_build_environment_ignores_proxy_and_go_override_injection(self):
        env = self.runner.build_environment({'HTTPS_PROXY': 'untrusted', 'GOFLAGS': '-overlay=untrusted',
                                            'GOPROXY': 'https://untrusted', 'VAULT_SKIP_VERIFY': 'true'})
        self.assertNotIn('HTTPS_PROXY', env)
        self.assertNotIn('VAULT_SKIP_VERIFY', env)
        self.assertEqual(env['GOFLAGS'], '')
        self.assertEqual(env['GOPROXY'], 'off')


if __name__ == '__main__':
    unittest.main()
