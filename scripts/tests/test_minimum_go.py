"""Minimum Go receipts cannot replace main-toolchain or other-platform proof."""
import copy
import importlib.util
import pathlib
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
from evidence import report_results, release_problems
from evidence_fixture import EvidenceFixture


class MinimumGoTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.fixture = EvidenceFixture(tmp.name)

    def report(self, consumer=False, version='1.25.0', platform='windows'):
        report = copy.deepcopy(self.fixture.reports['consumers' if consumer else 'normal'])
        report.update(evidence_class='MINIMUM_GO_CONSUMERS' if consumer else 'MINIMUM_GO_COMPATIBILITY', platform=platform)
        raw = f'go version go{version} {platform}/amd64'
        receipt = self.fixture.receipt('minimum-version', raw + '\n', ['go', 'version'])
        receipt['name'] = 'go-version'
        report.update(environment_commands=[receipt], tool_versions={'go': raw})
        return report

    def test_minimum_normal_and_consumers_require_real_targets(self):
        for consumer in (False, True):
            report = self.report(consumer)
            self.assertTrue(report_results(self.fixture.root, report, 'a' * 64)['tests'])
            report['commands'] = []
            with self.assertRaises(ValueError):
                report_results(self.fixture.root, report, 'a' * 64)

    def test_main_and_minimum_versions_are_not_interchangeable(self):
        for consumer in (False, True):
            report = self.report(consumer, version='1.26.8')
            with self.assertRaises(ValueError):
                report_results(self.fixture.root, report, 'a' * 64)
            report = self.report(consumer)
            report['evidence_class'] = 'INDEPENDENT_CONSUMERS' if consumer else 'NORMAL_OFFICIAL_CLIENT'
            with self.assertRaises(ValueError):
                report_results(self.fixture.root, report, 'a' * 64)

    def test_report_platform_must_match_go_version_receipt(self):
        report = copy.deepcopy(self.fixture.reports['normal'])
        report['platform'] = 'linux'
        with self.assertRaises(ValueError):
            report_results(self.fixture.root, report, 'a' * 64)

    def test_previous_main_patch_cannot_satisfy_current_toolchain(self):
        for consumer in (False, True):
            report = self.report(consumer, version='1.26.3')
            report['evidence_class'] = 'INDEPENDENT_CONSUMERS' if consumer else 'NORMAL_OFFICIAL_CLIENT'
            with self.assertRaises(ValueError):
                report_results(self.fixture.root, report, 'a' * 64)

    def test_release_requires_both_minimum_platforms_and_consumers(self):
        reports = {k: v for k, v in self.fixture.reports.items() if not k.startswith('minimum-')}
        failures = release_problems(self.fixture.root, self.fixture.ledger, self.fixture.acceptance, reports, 'a' * 64)
        for name in ('minimum-normal-linux', 'minimum-normal-windows', 'minimum-consumer-linux', 'minimum-consumer-windows'):
            self.assertTrue(any(f.startswith(name + ':') for f in failures), name)

    def test_ci_requires_four_independent_minimum_jobs(self):
        spec = importlib.util.spec_from_file_location('minimum_ci', ROOT / 'scripts/ci-summary.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        for kind in ('normal', 'consumer'):
            for platform in ('linux', 'windows'):
                name = f'minimum-{kind}-{platform}'
                self.assertIn(name, module.JOBS)


if __name__ == '__main__':
    unittest.main()
