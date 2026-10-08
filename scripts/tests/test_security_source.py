import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from evidence import report_results
from evidence_fixture import EvidenceFixture
from ci_export import build_export, structured_log


class SecuritySourceTests(unittest.TestCase):
    def fixture(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        return EvidenceFixture(temporary.name)

    def test_scanner_environment_requires_checksums_and_clears_overrides(self):
        spec = importlib.util.spec_from_file_location('security_source_target', Path(__file__).resolve().parents[1]/'security-test.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with mock.patch.dict(os.environ, {'GOPROXY': 'file:///untrusted', 'GOSUMDB': 'off',
                 'GOVULNDB': 'file:///empty', 'GITLEAKS_CONFIG': 'untrusted', 'GONOSUMDB': '*',
                 'GOPRIVATE': '*', 'GOINSECURE': '*'}):
            env = module.scanner_environment()
        self.assertEqual(env['GOPROXY'], 'https://proxy.golang.org,direct')
        self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
        for key in ('GONOSUMDB','GOPRIVATE','GOINSECURE'): self.assertEqual(env[key], '')
        for key in ('GOVULNDB','GITLEAKS_CONFIG'): self.assertNotIn(key, env)

    def test_ci_export_keeps_only_fixed_scanner_source_fields(self):
        source = 'Scanner: govulncheck@v1.1.4\nDB: https://vuln.go.dev\nDB updated: 2026-09-29 00:00:00 +0000 UTC\n'
        kind, output = structured_log({'name': 'govulncheck'}, source+'synthetic-sensitive-diagnostic\n', None)
        self.assertEqual(kind, 'scanner-source-v1')
        self.assertEqual(output, source)

    def test_evidence_rejects_database_flag_override_or_missing_source_receipt(self):
        fixture = self.fixture()
        source = fixture.reports['scans']
        self.assertEqual(report_results(fixture.root, source, 'a'*64)['commands'], {'govulncheck','gitleaks'})
        changed = copy.deepcopy(source)
        changed['commands'][0]['command'][3] = '-db=file:///empty'
        with self.assertRaises(ValueError): report_results(fixture.root, changed, 'a'*64)
        changed = copy.deepcopy(source)
        receipt = changed['commands'][0]
        path = fixture.root/receipt['log']
        original = path.read_text(encoding='utf-8')
        for line in ['Scanner: govulncheck@v1.1.4\n', 'DB: https://vuln.go.dev\n',
                     'DB updated: 2026-09-29 00:00:00 +0000 UTC\n']:
            raw = original.replace(line, '').encode()
            path.write_bytes(raw)
            receipt['log_sha256'] = hashlib.sha256(raw).hexdigest()
            with self.assertRaisesRegex(ValueError, 'database receipt missing'):
                report_results(fixture.root, changed, 'a'*64)

    def test_raw_findings_digest_is_required_and_detects_changed_bytes(self):
        fixture = self.fixture()
        source = fixture.reports['scans']
        path = fixture.root/source['findings']
        source['findings_sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
        report_results(fixture.root, source, 'a'*64)
        changed = copy.deepcopy(source)
        changed.pop('findings_sha256')
        with self.assertRaisesRegex(ValueError, 'findings digest'):
            report_results(fixture.root, changed, 'a'*64)
        path.write_bytes(path.read_bytes()+b'\n')
        with self.assertRaisesRegex(ValueError, 'findings digest'):
            report_results(fixture.root, source, 'a'*64)

    def test_evidence_rejects_partial_directory_scan(self):
        fixture = self.fixture()
        source = fixture.reports['scans']
        report_results(fixture.root, source, 'a'*64)
        for target in ['src', str(fixture.root/'src'), str(fixture.root)]:
            changed = copy.deepcopy(source)
            changed['commands'][1]['command'][-1] = target
            with self.assertRaises(ValueError):
                report_results(fixture.root, changed, 'a'*64)

    def test_export_rejects_changed_raw_findings(self):
        fixture = self.fixture()
        source = fixture.reports['scans']
        path = fixture.root/source['findings']
        source['findings_sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
        path.write_bytes(path.read_bytes()+b'\n')
        with self.assertRaisesRegex(ValueError, 'findings digest'):
            build_export(fixture.root, '.artifacts/security-report.json', source)

    def test_exported_findings_digest_and_metadata_are_verified(self):
        fixture = self.fixture()
        source = fixture.reports['scans']
        source['findings_sha256'] = hashlib.sha256((fixture.root/source['findings']).read_bytes()).hexdigest()
        report_name = '.artifacts/security-report.json'
        files = build_export(fixture.root, report_name, source)
        exported = json.loads(files[report_name])
        directory = fixture.root/'ci-export'
        for name, raw in files.items():
            path = directory/name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(raw)
        report_results(directory, exported, 'a'*64, allow_export=True)
        for key, value in [('kind', 'untrusted'), ('raw_sha256', '0'*64), ('sha256', '0'*64)]:
            changed = copy.deepcopy(exported)
            changed['findings_export'][key] = value
            with self.subTest(key=key), self.assertRaisesRegex(ValueError, 'findings digest'):
                report_results(directory, changed, 'a'*64, allow_export=True)
        (directory/source['findings']).write_bytes(b'[]')
        with self.assertRaisesRegex(ValueError, 'findings digest'):
            report_results(directory, exported, 'a'*64, allow_export=True)


if __name__ == '__main__': unittest.main()
