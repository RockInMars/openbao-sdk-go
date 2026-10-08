import datetime as dt
import importlib.util
import json
import pathlib
import sys
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT/'scripts'))
from remote_manifest import Manifest, ManifestError, load_manifest


def plan_data():
    plan = json.loads((ROOT/'deploy/test/remote-manifest.example.json').read_text(encoding='utf-8'))
    now = dt.datetime.now(dt.timezone.utc).replace(microsecond=0)
    plan.update(issued_at=(now-dt.timedelta(seconds=1)).isoformat().replace('+00:00', 'Z'),
                expires_at=(now+dt.timedelta(minutes=19)).isoformat().replace('+00:00', 'Z'))
    return plan


def manifest(plan=None):
    return Manifest(json.dumps(plan or plan_data()).encode())


class RemoteManifestTests(unittest.TestCase):
    def setUp(self):
        spec = importlib.util.spec_from_file_location('remote_fixture_test_target', ROOT/'scripts/remote-fixture.py')
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)

    def fixture(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        return self.module.Fixture(manifest(), directory.name)

    def connection(self, status=204, data=None):
        conn = mock.Mock()
        response = conn.getresponse.return_value
        response.status = status
        response.read.return_value = json.dumps(data).encode() if data is not None else b''
        return conn

    def test_invalid_plans_fail_before_network(self):
        changes = [('target', 'https://other.invalid'), ('target_allowlist', ['https://other.invalid']),
                   ('namespace', 'root'), ('namespace_mode', 'root'), ('resource_prefix', '../business'),
                   ('bootstrap_mount', 'auth/other'), ('resource_budget', 10), ('schema_version', True),
                   ('credential', 'synthetic'), ('allowed_operations', ['all']),
                   ('request_budget', {'total': 121, 'core': 43, 'transit': 32, 'cleanup': 15})]
        for key, value in changes:
            with self.subTest(key=key), tempfile.TemporaryDirectory() as directory:
                data = plan_data()
                data[key] = value
                path = pathlib.Path(directory)/'plan.json'
                path.write_text(json.dumps(data), encoding='utf-8')
                with mock.patch.object(self.module.http.client, 'HTTPSConnection') as connection:
                    self.assertEqual(self.module.main(['--manifest', str(path), '--execute']), 2)
                    connection.assert_not_called()

    def test_time_window_rejects_expired_future_and_overlong(self):
        for issued, expires in [('2000-01-01T00:00:00Z', '2000-01-01T00:20:00Z'),
                                ('2999-01-01T00:00:00Z', '2999-01-01T00:20:00Z'),
                                ('2000-01-01T00:00:00Z', '2000-01-01T01:00:00Z')]:
            data = plan_data()
            data.update(issued_at=issued, expires_at=expires)
            with self.assertRaises(ManifestError): manifest(data)

    def test_digest_duplicates_and_size_are_rejected(self):
        plan = manifest()
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory)/'plan.json'
            path.write_bytes(plan.raw)
            self.assertEqual(load_manifest(path, plan.sha256).sha256, plan.sha256)
            with self.assertRaises(ManifestError): load_manifest(path, '0'*64)
        with self.assertRaises(ManifestError): Manifest(b'{"schema_version":1,'+plan.raw[1:])
        with self.assertRaises(ManifestError): Manifest(b' '*16385)

    def test_run_directory_and_child_identifier_are_exclusive(self):
        plan = manifest()
        with tempfile.TemporaryDirectory() as root:
            directory = self.module.reserve_directory(root, plan)
            self.assertEqual((directory/'manifest.json').read_bytes(), plan.raw)
            with self.assertRaisesRegex(self.module.StopRun, 'already_exists'):
                self.module.reserve_directory(root, plan)
        with tempfile.TemporaryDirectory() as root:
            (pathlib.Path(root)/'.artifacts/runs'/(plan.run_id+'-core')).mkdir(parents=True)
            with self.assertRaisesRegex(self.module.StopRun, 'already_exists'):
                self.module.reserve_directory(root, plan)

    def test_budget_expiry_prefix_and_unowned_subtree_stop_before_network(self):
        f = self.fixture()
        with mock.patch.object(self.module.http.client, 'HTTPSConnection') as connection:
            with self.assertRaisesRegex(self.module.StopRun, 'outside_manifest'):
                f.call('POST', '/v1/business/data/item', {})
            with self.assertRaisesRegex(self.module.StopRun, 'outside_manifest'):
                f.call('POST', '/v1/'+f.mounts['transit']+'/keys/cipher', {})
            f.report['admin_requests'] = 120
            with self.assertRaisesRegex(self.module.StopRun, 'budget_exhausted'):
                f.call('GET', '/v1/sys/health')
            f.report['admin_requests'] = 0
            with mock.patch.object(f.plan, 'assert_active', side_effect=ManifestError('expired')):
                with self.assertRaisesRegex(self.module.StopRun, 'expired'):
                    f.call('GET', '/v1/sys/health')
                f.cleanup()
            self.assertEqual(f.report['cleanup'], 'BLOCKED_EXPIRED')
            connection.assert_not_called()

    def test_mount_collision_never_becomes_owned_or_deleted(self):
        f = self.fixture()
        conn = self.connection(409)
        with mock.patch.object(self.module.http.client, 'HTTPSConnection', return_value=conn):
            with self.assertRaisesRegex(self.module.StopRun, 'definite_rejection'):
                f.own('mount', f.mounts['kv'], 'POST', '/v1/sys/mounts/'+f.mounts['kv'], {'type': 'kv'})
            f.cleanup()
        self.assertEqual(f.report['resources'][0]['state'], 'not_created')
        self.assertEqual(f.report['cleanup'], 'PASS')
        self.assertEqual(conn.request.call_count, 1)

    def test_policy_collision_requires_exact_cas_response(self):
        for data, expected in [({'errors': ['check-and-set parameter set to -1 on existing entry']}, 'not_created'),
                               ({'errors': ['different failure']}, 'creation_pending')]:
            f = self.fixture()
            conn = self.connection(400, data)
            name = f.policies['runtime']
            with mock.patch.object(self.module.http.client, 'HTTPSConnection', return_value=conn):
                with self.assertRaises(self.module.StopRun):
                    f.own('policy', name, 'POST', '/v1/sys/policies/acl/'+name,
                          {'policy': 'synthetic', 'cas': -1, 'cas_required': True})
                f.cleanup()
            self.assertEqual(f.report['resources'][0]['state'], expected)
            self.assertEqual(conn.request.call_count, 1)

    def test_partial_setup_cleans_only_created_identity_in_reverse(self):
        f = self.fixture()
        for name in (f.mounts['kv'], f.mounts['transit']):
            item = f.register_resource('mount', name)
            item.update(state='owned', identity_sha256=name)
        f.register_resource('policy', f.policies['runtime'])['state'] = 'not_created'
        conn = self.connection()
        with mock.patch.object(f, 'read_identity', side_effect=lambda item: item['name']), \
             mock.patch.object(self.module.http.client, 'HTTPSConnection', return_value=conn):
            f.cleanup()
        paths = [call.args[1] for call in conn.request.call_args_list]
        self.assertEqual(paths, ['/v1/sys/mounts/'+f.mounts['transit'], '/v1/sys/mounts/'+f.mounts['kv']])
        self.assertEqual(f.report['cleanup'], 'PASS')

    def test_changed_identity_cleanup_failure_and_pending_creation_block_cleanup(self):
        f = self.fixture()
        item = f.register_resource('mount', f.mounts['kv'])
        item.update(state='owned', identity_sha256='original')
        with mock.patch.object(f, 'read_identity', return_value='changed'), \
             mock.patch.object(self.module.http.client, 'HTTPSConnection') as connection:
            f.cleanup()
            connection.assert_not_called()
        self.assertEqual(item['state'], 'ownership_changed')
        f = self.fixture()
        item = f.register_resource('mount', f.mounts['kv'])
        item.update(state='owned', identity_sha256='original')
        with mock.patch.object(f, 'read_identity', return_value='original'), \
             mock.patch.object(self.module.http.client, 'HTTPSConnection', return_value=self.connection(500)):
            f.cleanup()
        self.assertEqual(item['state'], 'cleanup_unknown')
        self.assertEqual(f.report['cleanup'], 'INCOMPLETE')
        f = self.fixture()
        f.register_resource('mount', f.mounts['kv'])
        with mock.patch.object(self.module.http.client, 'HTTPSConnection') as connection:
            f.cleanup()
            connection.assert_not_called()
        self.assertEqual(f.report['cleanup'], 'BLOCKED_UNCERTAIN_OWNERSHIP')

    def test_unknown_child_timeout_or_missing_receipt_blocks_cleanup(self):
        for receipt in [{'completion': 'timeout', 'exit_code': None}, {'completion': 'exited', 'exit_code': 0}]:
            f = self.fixture()
            with mock.patch.object(self.module, 'run_command', return_value=receipt), \
                 mock.patch.object(self.module, 'ROOT', pathlib.Path(f.directory)):
                with self.assertRaises(self.module.StopRun): f.run_sdk()
                f.cleanup()
            self.assertTrue(f.unsafe_cleanup)
            self.assertEqual(f.report['request_count_status'], 'UNKNOWN')
            self.assertEqual(f.report['cleanup'], 'BLOCKED_UNCERTAIN_OUTCOME')

    def test_server_contract_is_checked_before_bootstrap(self):
        for version, accepted in [('2.6.3', True), ('2.6.2', True), ('2.5.0', False), ('unknown', False)]:
            f = self.fixture()
            conn = self.connection(200, {'version': version, 'initialized': True, 'sealed': False})
            with mock.patch.object(self.module.http.client, 'HTTPSConnection', return_value=conn):
                if accepted: f.verify_server()
                else:
                    with self.assertRaisesRegex(self.module.StopRun, 'server_contract_unverified'): f.verify_server()
            self.assertEqual(f.report['resources'], [])
            self.assertEqual(conn.request.call_args.args[1], '/v1/sys/health')


if __name__ == '__main__':
    unittest.main()
