import pathlib
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
import tooling


class SourceIdentityTests(unittest.TestCase):
    def fixture(self, root):
        for name, raw in {
            'client.go': b'package bao\n',
            'go.mod': b'module example.org/bao\ngo 1.25.0\n',
            'scripts/tests/fixtures/中文.txt': '中文\n'.encode('utf-8'),
        }.items():
            p = root / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_bytes(raw)

    def test_identity_is_independent_of_root_and_path_separator(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            a, b = pathlib.Path(a), pathlib.Path(b)
            self.fixture(a)
            self.fixture(b)
            expected = '52b0a1927d842d084b1836b75ab3c908a472a29011bfed975ea680c657e42416'
            self.assertEqual(tooling.source_hash(a), expected)
            self.assertEqual(tooling.source_hash(b), expected)
            self.assertEqual(tooling.source_identity(a), {'source_hash_version': 2, 'source_sha256': expected})

    def test_local_agent_config_is_excluded(self):
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            self.fixture(root)
            before = tooling.source_hash(root)
            for name in ['.codex/config.toml', '.serena/project.yml', '.artifacts/generated.go', 'docs/evidence/old.py', 'task-status.json', 'README.md']:
                p = root / name
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_text('local state', encoding='utf-8')
                self.assertEqual(tooling.source_hash(root), before, name)

    def test_code_tests_workflows_and_server_lock_change_identity(self):
        names = ['internal/engine/a.go', 'examples/demo/go.mod', 'tests/testdata/cert.pem',
                 'scripts/tests/test_extra.py', 'scripts/acceptance-rules.json',
                 'scripts/tests/fixtures/result.jsonl', '.github/workflows/ci.yml',
                 'deploy/test/server-lock.json', 'deploy/test/config.hcl']
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            self.fixture(root)
            for name in names:
                before = tooling.source_hash(root)
                p = root / name
                p.parent.mkdir(parents=True, exist_ok=True)
                p.write_bytes(b'first\n')
                self.assertNotEqual(tooling.source_hash(root), before, name)
                before = tooling.source_hash(root)
                p.write_bytes(b'first\r\n')
                self.assertNotEqual(tooling.source_hash(root), before, 'raw byte changes: ' + name)

    def test_source_symlink_is_rejected(self):
        # Model the filesystem link flag without needing Windows symlink privilege.
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            self.fixture(root)
            original = pathlib.Path.is_symlink
            with mock.patch.object(pathlib.Path, 'is_symlink', lambda p: p == root / 'client.go' or original(p)):
                with self.assertRaisesRegex(ValueError, 'symlink'):
                    tooling.source_hash(root)

    def test_changed_baseline_cannot_finish_as_pass(self):
        self.assertTrue(hasattr(tooling, 'start_report'), 'report producers need a shared identity guard')
        with tempfile.TemporaryDirectory() as d:
            root = pathlib.Path(d)
            self.fixture(root)
            report = tooling.start_report('TEST', root)
            report['status'] = 'PASS'
            (root / 'client.go').write_bytes(b'package changed\n')
            tooling.finish_report(report, root)
            self.assertEqual(report['status'], 'FAIL')
            self.assertIn('baseline_changed', report['problems'])
            self.assertNotEqual(report['source_before'], report['source_after'])
            (root / 'client.go').write_bytes(b'package bao\n')
            report['status'] = 'PASS'
            tooling.finish_report(report, root)
            self.assertEqual(report['source_before'], report['source_after'])
            self.assertEqual(report['status'], 'FAIL', 'observed A-B-A change must remain latched')


if __name__ == '__main__':
    unittest.main()
