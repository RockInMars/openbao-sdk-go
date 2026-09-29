"""Tests for the transport tool. Synthetic fixtures NEVER certify OpenBao."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/dependency-bundle.py'


class DependencyBundleTests(unittest.TestCase):
    def load(self):
        self.assertTrue(SCRIPT.is_file(), 'OB-001 is missing the isolated public-dependency handoff tool')
        spec = importlib.util.spec_from_file_location('dependency_bundle', SCRIPT)
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod

    def fixture(self, folder):
        root = folder / 'sdk'
        root.mkdir()
        (root / 'go.mod').write_text('module example.com/synthetic-sdk\n\ngo 1.23.2\n\nrequire github.com/openbao/openbao/api/v2 v2.7.0\n')
        (root / 'go.sum').write_text('untrusted-seeded-content\n')
        (root / 'main.go').write_text('package sdk\n')
        (root / '.env').write_text('test-secret-must-not-be-copied\n')
        (root / 'private.key').write_text('test-secret-must-not-be-copied\n')
        return root

    def payload(self, folder):
        stage = folder / 'payload'
        for name, data in {
            'proxy/sumdb/sum.golang.org/supported': b'',
            'proxy/sumdb/sum.golang.org/lookup/example.com/fixture@v1.0.0': b'synthetic-invalid-signed-response',
            'proxy/sumdb/sum.golang.org/tile/8/0/000': b'synthetic-invalid-tile',
            'proxy/example.com/fixture/@v/v1.0.0.mod': b'module example.com/fixture\n\ngo 1.23.2\n',
            'proxy/example.com/fixture/@v/v1.0.0.info': b'{"Version":"v1.0.0","Time":"2020-01-01T00:00:00Z"}',
            'resolved/go.mod': b'module example.com/replay\n\ngo 1.23.2\n\nrequire example.com/fixture v1.0.0\n',
            'resolved/go.sum': b'not trusted as authentication input\n',
        }.items():
            p = stage / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_bytes(data)
        zpath = stage / 'proxy/example.com/fixture/@v/v1.0.0.zip'
        with zipfile.ZipFile(zpath, 'w') as z:
            z.writestr('example.com/fixture@v1.0.0/fixture.go', 'package fixture\n')
        return stage

    def test_environment_uses_fresh_caches_and_no_identity_or_bypass(self):
        m = self.load()
        env = m.clean_environment({'PATH': os.environ['PATH'], 'HOME': '/personal',
                                   'GOSUMDB': 'off', 'GONOSUMDB': '*', 'GONOPROXY': '*',
                                   'GOWORK': '/other/go.work', 'GOFLAGS': '-overlay=fake',
                                   'BAO_TOKEN': 'fixture-token', 'VAULT_TOKEN': 'fixture-token',
                                   'HTTP_PROXY': 'http://user:password@proxy.invalid'}, Path('/isolated'), 'https://proxy.golang.org')
        self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
        self.assertEqual(env['GOTOOLCHAIN'], 'local')
        self.assertEqual(env['GOMODCACHE'], '/isolated/modcache')
        self.assertEqual(env['GOPATH'], '/isolated/gopath')
        self.assertEqual(env['HOME'], '/isolated/home')
        self.assertEqual(env['GOWORK'], 'off')
        for key in ('GOFLAGS', 'GONOPROXY', 'GONOSUMDB', 'GOPRIVATE', 'GOINSECURE'):
            self.assertEqual(env[key], '')
        for key in ('BAO_TOKEN', 'VAULT_TOKEN', 'HTTP_PROXY'):
            self.assertNotIn(key, env)

    def test_proxy_must_be_one_https_origin_without_credentials_or_fallback(self):
        m = self.load()
        self.assertEqual(m.public_proxy('https://proxy.golang.org'), 'https://proxy.golang.org')
        for bad in ('http://proxy.invalid', 'https://u:p@proxy.invalid', 'https://proxy.invalid/?token=x',
                    'https://proxy.invalid#x', 'https://proxy.invalid,direct', 'https://proxy.invalid|direct',
                    'file:///personal/cache', 'off', 'direct', 'https://proxy.invalid/../x'):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                m.public_proxy(bad)

    def test_source_snapshot_does_not_copy_secrets_nested_modules_or_seed_sums(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); root = self.fixture(base)
            nested = root / 'consumer'; nested.mkdir()
            (nested / 'go.mod').write_text('module example.com/nested\n')
            (nested / 'main.go').write_text('package nested\n')
            work = base / 'work'
            m.copy_sdk_inputs(root, work)
            self.assertEqual({x.relative_to(work).as_posix() for x in work.rglob('*') if x.is_file()}, {'main.go', 'go.mod'})
            self.assertFalse((work / 'go.sum').exists())

    def test_source_symlink_is_rejected_not_followed(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); root = self.fixture(base)
            (root / 'injected.go').symlink_to(root / 'private.key')
            with self.assertRaises(ValueError):
                m.copy_sdk_inputs(root, base / 'work')

    def test_input_fingerprint_detects_code_and_module_changes(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            root = self.fixture(Path(td))
            before = m.input_fingerprint(root)
            (root / '.env').write_text('other secret')
            self.assertEqual(m.input_fingerprint(root), before)
            (root / 'main.go').write_text('package changed\n')
            self.assertNotEqual(m.input_fingerprint(root), before)

    def test_bundle_paths_cannot_escape_or_carry_code_credentials_or_ziphash(self):
        m = self.load()
        for good in ('resolved/go.mod', 'resolved/go.sum', 'proxy/github.com/!upper/lib/@v/v1.2.3.zip',
                     'proxy/sumdb/sum.golang.org/lookup/github.com/!upper/lib@v1.2.3',
                     'proxy/sumdb/sum.golang.org/tile/8/0/x001/051.p/123'):
            self.assertTrue(m.allowed_payload(good), good)
        for bad in ('../escape', '/abs', 'C:/bad', 'proxy/../../escape', 'proxy/a//b',
                    'proxy/a/./b', 'proxy/a\\b', 'proxy/a/%2e%2e/b', '.env', 'scripts/run.py',
                    'proxy/github.com/x/@v/v1.0.0.ziphash', 'proxy/github.com/x/@v/v1.0.0.lock',
                    'proxy/sumdb/untrusted.invalid/latest', 'proxy/.git/config'):
            self.assertFalse(m.allowed_payload(bad), bad)

    def test_duplicate_json_keys_are_rejected(self):
        m = self.load()
        with self.assertRaises(ValueError):
            m.strict_json('{"schema":1,"schema":2}')

    def test_archive_roundtrip_is_integrity_only_not_official_verification(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); stage = self.payload(base); target = base / 'bundle.zip'
            meta = {'input_sha256': '0'*64, 'modules': [], 'go_version': 'go1.23.2'}
            m.write_bundle(stage, target, meta)
            self.assertTrue(target.is_file())
            digest = hashlib.sha256(target.read_bytes()).hexdigest()
            parsed = m.unpack_bundle(target, digest, base / 'unpacked')
            self.assertEqual(parsed['input_sha256'], '0'*64)
            self.assertNotIn('official_verified', parsed)
            self.assertEqual((base/'unpacked/resolved/go.mod').read_bytes(), (stage/'resolved/go.mod').read_bytes())

    def test_archive_corruption_and_wrong_expected_sha_are_rejected(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); stage = self.payload(base); target = base / 'bundle.zip'
            m.write_bundle(stage, target, {'input_sha256': '0'*64, 'modules': [], 'go_version': 'go1.23.2'})
            with self.assertRaises(ValueError):
                m.unpack_bundle(target, '1'*64, base/'bad-sha')
            with zipfile.ZipFile(target, 'a') as z:
                z.writestr('injected.txt', 'must reject')
            with self.assertRaises(ValueError):
                m.unpack_bundle(target, hashlib.sha256(target.read_bytes()).hexdigest(), base/'bad-file')

    def test_archive_symlink_traversal_and_duplicate_are_rejected(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td)
            for index, name in enumerate(('../outside', 'proxy/link', 'resolved/go.mod')):
                target = base/f'bad-{index}.zip'
                with zipfile.ZipFile(target, 'w') as z:
                    z.writestr('manifest.json', '{}')
                    info = zipfile.ZipInfo(name)
                    if index == 1:
                        info.create_system=3; info.external_attr=(stat.S_IFLNK|0o777)<<16
                    z.writestr(info, 'not-valid')
                with self.assertRaises(ValueError):
                    m.unpack_bundle(target, hashlib.sha256(target.read_bytes()).hexdigest(), base/f'extract-{index}')
            self.assertFalse((base/'outside').exists())

    def test_existing_output_is_not_overwritten(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base=Path(td); stage=self.payload(base); target=base/'existing.zip'; target.write_bytes(b'keep')
            with self.assertRaises((ValueError, FileExistsError)):
                m.write_bundle(stage, target, {})
            self.assertEqual(target.read_bytes(), b'keep')

    def test_payload_limits_are_enforced_before_extraction(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base=Path(td); stage=self.payload(base); target=base/'bundle.zip'
            m.write_bundle(stage,target,{'input_sha256':'0'*64,'modules':[],'go_version':'go1.23.2'})
            with patch.object(m, 'MAX_TOTAL_BYTES', 12):
                with self.assertRaises(ValueError):
                    m.unpack_bundle(target,hashlib.sha256(target.read_bytes()).hexdigest(),base/'unpack')

    def test_failed_export_leaves_no_bundle_or_modified_sdk(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base=Path(td); root=self.fixture(base); before=m.input_fingerprint(root)
            with patch.object(m, 'run_go', side_effect=ValueError('simulated download failure')):
                report=m.export_bundle(root,base/'bundle.zip','https://proxy.golang.org',base/'report')
            self.assertEqual(report['status'],'BLOCKED')
            self.assertFalse(report['sdk_compile_verified'])
            self.assertFalse((base/'bundle.zip').exists())
            self.assertEqual(m.input_fingerprint(root),before)

    def test_real_go_rejects_unsigned_synthetic_sumdb_without_network_fallback(self):
        # Genuine Go command, synthetic invalid material; NOT a positive upstream test.
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base=Path(td); stage=self.payload(base); scratch=base/'scratch'; scratch.mkdir()
            env=m.clean_environment(os.environ,scratch,(stage/'proxy').as_uri())
            work=scratch/'work';work.mkdir()
            (work/'go.mod').write_bytes((stage/'resolved/go.mod').read_bytes())
            result=subprocess.run(['go','mod','download','-json','example.com/fixture@v1.0.0'],cwd=work,env=env,
                                  capture_output=True,text=True,timeout=20)
            self.assertNotEqual(result.returncode,0)
            self.assertIn('verifying go.mod:',result.stdout+result.stderr)
            self.assertIn('malformed record data',result.stdout+result.stderr)
            self.assertNotIn('https://sum.golang.org',result.stdout+result.stderr)
            self.assertFalse((work/'go.sum').exists())


    def test_proxy_collection_does_not_pack_cache_bypass_or_personal_files(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); cache = base/'scratch/modcache/cache/download'
            for name in ('example.com/fixture/@v/v1.0.0.mod',
                         'example.com/fixture/@v/v1.0.0.ziphash',
                         'example.com/fixture/@v/v1.0.0.lock',
                         'sumdb/sum.golang.org/lookup/example.com/fixture@v1.0.0',
                         'sumdb/sum.golang.org/tile/8/0/000', '.netrc'):
                p = cache/name; p.parent.mkdir(parents=True, exist_ok=True); p.write_bytes(b'synthetic')
            m.collect_proxy(base/'scratch', base/'stage')
            names = {x.relative_to(base/'stage').as_posix() for x in (base/'stage').rglob('*') if x.is_file()}
            self.assertEqual(len(names), 4)
            self.assertFalse(any(x.endswith(('.ziphash', '.lock', '.netrc')) for x in names))
            self.assertIn('proxy/sumdb/sum.golang.org/supported', names)

    def test_verify_wrong_source_does_not_run_go_or_publish_destination(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); root = self.fixture(base); stage = self.payload(base); target = base/'bundle.zip'
            m.write_bundle(stage, target, {'input_sha256': '0'*64, 'modules': [], 'go_version': 'go1.23.2'})
            with patch.object(m, 'run_go') as command:
                report = m.verify_bundle(root, target, hashlib.sha256(target.read_bytes()).hexdigest(),
                                         base/'published', base/'logs')
            command.assert_not_called()
            self.assertEqual(report['status'], 'BLOCKED')
            self.assertFalse((base/'published').exists())
            self.assertFalse(report['sdk_compile_verified'])

    def test_replay_does_not_seed_supplied_go_sum_or_cache(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); stage = self.payload(base)
            def inspect(name, args, work, env, report, logs):
                self.assertEqual(name, 'replay-root')
                self.assertFalse((work/'go.sum').exists())
                self.assertFalse(Path(env['GOMODCACHE']).exists())
                self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
                self.assertEqual(env['GOPROXY'], (stage/'proxy').as_uri())
                self.assertEqual(env['GOTOOLCHAIN'], 'local')
                raise ValueError('stop after inspecting authentication inputs')
            with patch.object(m, 'run_go', side_effect=inspect):
                with self.assertRaisesRegex(ValueError, 'authentication inputs'):
                    m.offline_replay(stage, base/'replay', [], {'commands': []}, base/'logs')

    def test_payload_hash_rejects_tampering_even_with_new_archive_sha(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as td:
            base = Path(td); stage = self.payload(base); target = base/'original.zip'
            m.write_bundle(stage, target, {'input_sha256': '0'*64, 'modules': [], 'go_version': 'go1.23.2'})
            changed = base/'changed.zip'
            with zipfile.ZipFile(target) as src, zipfile.ZipFile(changed, 'w') as dst:
                for info in src.infolist():
                    data = src.read(info)
                    if info.filename == 'resolved/go.sum':
                        data = b'x'*len(data)
                    dst.writestr(info, data)
            with self.assertRaisesRegex(ValueError, 'payload SHA256 mismatch'):
                m.unpack_bundle(changed, hashlib.sha256(changed.read_bytes()).hexdigest(), base/'extract')


if __name__ == '__main__':
    unittest.main()
