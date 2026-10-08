"""Synthetic fixtures exercise validation rules, never deployment readiness."""
import copy
import hashlib
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import tooling
from evidence import report_results
from evidence_fixture import EvidenceFixture


class EvidenceFixtureTests(unittest.TestCase):
    def fixture(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        return EvidenceFixture(temporary.name)

    def test_instances_and_global_root_are_isolated(self):
        original_root = tooling.ROOT
        first, second = self.fixture(), self.fixture()
        self.assertNotIsInstance(first, unittest.TestCase)
        first.acceptance['results'][0]['status'] = 'FAIL'
        self.assertTrue(first.problems())
        self.assertEqual(second.problems(), [])
        self.assertEqual(tooling.ROOT, original_root)
        self.assertNotEqual(first.root, second.root)

    def reports(self, fixture):
        reports = copy.deepcopy(fixture.reports)
        tooling_report = copy.deepcopy(reports['normal'])
        tooling_report.update(evidence_class='COMMAND_TOOLING', commands=[fixture.receipt(
            'tooling', 'Ran 3 tests in 0.01s\n\nOK\n',
            ['python', '-B', '-m', 'unittest', 'discover', '-s', 'scripts/tests', '-v'])])
        tooling_report.pop('environment_commands')
        reports['tooling'] = tooling_report
        fuzz = copy.deepcopy(reports['normal'])
        fuzz.update(evidence_class='FUZZ_RUNTIME', commands=[])
        for name, package in [('FuzzPath', './internal/engine'), ('FuzzDocument', './kv'),
                              ('FuzzPKICSR', './pki'), ('FuzzEncoding', './internal/transitutil'),
                              ('FuzzDecode', './internal/pemutil')]:
            fuzz['commands'].append(fixture.receipt(name, 'fuzz: elapsed: 30s, execs: 7 (1/sec)\nPASS\n',
                ['go', 'test', '-mod=readonly', '-run=^$', '-fuzz='+name,
                 '-fuzztime=30s', '-parallel=1', '-timeout=90s', package]))
        reports['fuzz'] = fuzz
        document = fixture.write('docs/design.md', 'Synthetic design fixture.\n')
        static = copy.deepcopy(reports['normal'])
        static.update(evidence_class='STATIC_REVIEW', commands=[], reviewed_at='2026-09-29T00:00:00Z',
                      rationale='Synthetic fixture only', documents=[{
                          'path': 'docs/design.md', 'sha256': hashlib.sha256(document.read_bytes()).hexdigest()}])
        static.pop('environment_commands')
        static.pop('working_directory')
        reports['static'] = static
        return reports

    def test_main_and_minimum_categories_keep_their_selectors(self):
        fixture = self.fixture()
        reports = self.reports(fixture)
        parsed = {name: report_results(fixture.root, value, 'a'*64) for name, value in reports.items()}
        self.assertEqual(len(parsed), 11)
        for platform in ('linux','windows'):
            self.assertEqual(parsed['minimum-normal-'+platform]['tests'], parsed['normal']['tests'])
            self.assertEqual(parsed['minimum-consumer-'+platform]['tests'], parsed['consumers']['tests'])
        self.assertEqual(parsed['normal']['tests'], {('example.org/sdk/tests/dependency', 'TestDependencyBaseline')})
        self.assertEqual(len(parsed['integration']['tests']), 6)
        self.assertTrue({'reader', 'signer'}.issubset(parsed['consumers']['commands']))
        self.assertEqual(parsed['scans']['commands'], {'govulncheck', 'gitleaks'})
        self.assertEqual(parsed['tooling']['commands'], {'tooling'})
        self.assertEqual(parsed['fuzz']['fuzz_targets'], {'FuzzPath', 'FuzzDocument', 'FuzzPKICSR', 'FuzzEncoding', 'FuzzDecode'})
        self.assertEqual(parsed['static']['documents'], {'docs/design.md'})

    def test_every_category_rejects_stale_or_unsuccessful_envelope(self):
        fixture = self.fixture()
        for name, value in self.reports(fixture).items():
            for change in [{'status': 'FAIL'}, {'run_id': ''}, {'source_sha256': 'old'},
                           {'source_after': {}}, {'problems': ['baseline_changed']}]:
                with self.subTest(category=name, change=change), self.assertRaises(ValueError):
                    report_results(fixture.root, {**value, **change}, 'a'*64)

    def test_each_command_category_requires_successful_original_receipts(self):
        fixture = self.fixture()
        for name, report in self.reports(fixture).items():
            if not report['commands']:
                continue
            for change in [{'exit_code': 1}, {'completion': 'timeout'}, {'cleanup_incomplete': True},
                           {'log_sha256': 'wrong'}, {'log': '../outside.log'}]:
                modified = copy.deepcopy(report)
                modified['commands'][0].update(change)
                with self.subTest(category=name, change=change), self.assertRaises(ValueError):
                    report_results(fixture.root, modified, 'a'*64)

    def test_static_changed_bytes_and_unknown_class_are_rejected(self):
        fixture = self.fixture()
        reports = self.reports(fixture)
        fixture.write('docs/design.md', 'Different bytes.\n')
        with self.assertRaisesRegex(ValueError, 'static document changed'):
            report_results(fixture.root, reports['static'], 'a'*64)
        with self.assertRaisesRegex(ValueError, 'unsupported evidence class'):
            report_results(fixture.root, {**reports['normal'], 'evidence_class': 'UNKNOWN'}, 'a'*64)


if __name__ == '__main__':
    unittest.main()
