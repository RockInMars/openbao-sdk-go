import copy
import hashlib
import importlib.util
import pathlib
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
from evidence import FORMAL_REPORT_NAMES
from evidence_fixture import EvidenceFixture

spec = importlib.util.spec_from_file_location('current_status', ROOT / 'scripts/current-status.py')
status = importlib.util.module_from_spec(spec)
spec.loader.exec_module(status)


class CurrentStatusTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.fixture = EvidenceFixture(tmp.name)
        self.fixture.write('task-status.json', self.fixture.ledger)
        self.fixture.write('acceptance-results.json', self.fixture.acceptance)
        for key, name in FORMAL_REPORT_NAMES.items():
            self.fixture.write('.artifacts/' + name + '-report.json', self.fixture.reports[key])

    def summary(self, current='a' * 64):
        return status.summarize(self.fixture.root, current_hash=current)

    def test_valid_current_reports_and_gate_are_derived_without_mutating_ledgers(self):
        before = (self.fixture.root / 'task-status.json').read_bytes()
        summary = self.summary()
        self.assertEqual(summary['gate'], 'PASS')
        self.assertTrue(all(row['state'] == 'PASS' for row in summary['rows']))
        self.assertEqual((self.fixture.root / 'task-status.json').read_bytes(), before)

    def test_historical_passes_are_not_promoted_to_current(self):
        summary = self.summary('b' * 64)
        self.assertEqual(summary['gate'], 'FAIL')
        self.assertFalse(summary['ledger_current'])
        self.assertTrue(all(row['state'] == 'STALE' and row['recorded'] == 'PASS' for row in summary['rows']))

    def test_changed_log_and_missing_report_never_show_pass(self):
        self.fixture.write(self.fixture.reports['normal']['commands'][0]['log'], 'tampered\n')
        (self.fixture.root / '.artifacts/minimum-consumer-linux-report.json').unlink()
        rows = {row['name']: row for row in self.summary()['rows']}
        self.assertEqual(rows['normal']['state'], 'INVALID')
        self.assertEqual(rows['minimum-consumer-linux']['state'], 'NOT_RUN')

    def test_reported_block_and_remote_history_stay_distinct(self):
        report = copy.deepcopy(self.fixture.reports['integration'])
        report.update(status='BLOCKED', commands=[])
        self.fixture.write('.artifacts/integration-report.json', report)
        acceptance = copy.deepcopy(self.fixture.acceptance)
        acceptance['supplemental_remote'] = {**self.fixture.identity, 'status': 'PASS', 'historical_only': True}
        self.fixture.write('acceptance-results.json', acceptance)
        summary = self.summary()
        self.assertEqual(summary['gate'], 'FAIL')
        self.assertEqual(next(row for row in summary['rows'] if row['name'] == 'integration')['state'], 'BLOCKED')
        self.assertEqual(summary['remote_status'], 'PASS')
        self.assertFalse(summary['remote_current'])

    def test_index_checks_bytes_and_never_changes_historical_identity(self):
        raw = (self.fixture.root / 'evidence/normal.json').read_bytes()
        index = {**self.fixture.identity, 'reports': [{'path': 'evidence/normal.json', 'bytes': len(raw), 'sha256': hashlib.sha256(raw).hexdigest()}]}
        self.fixture.write('evidence/index.json', index)
        ledger = {**self.fixture.ledger, 'evidence_index': 'evidence/index.json'}
        self.assertEqual(status.index_state(self.fixture.root, ledger, 'b' * 64), 'STALE；文件摘要匹配 1/1')
        self.fixture.write('evidence/normal.json', {})
        self.assertEqual(status.index_state(self.fixture.root, ledger, 'a' * 64), 'CURRENT；文件摘要匹配 0/1')

    def test_generation_is_deterministic_and_has_no_new_status_store(self):
        rendered = status.render(self.summary())
        self.assertEqual(rendered, status.render(self.summary()))
        self.assertIn('最低版本', rendered)
        self.assertIn('不授予发布批准', rendered)


if __name__ == '__main__':
    unittest.main()
