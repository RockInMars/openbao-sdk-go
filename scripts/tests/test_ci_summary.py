import copy
import importlib.util
import json
import pathlib
import shutil
import sys
import tempfile
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'scripts'))
from evidence import FUZZ_TARGETS
from evidence_fixture import EvidenceFixture


class CISummaryTests(unittest.TestCase):
    def setUp(self):
        file=ROOT/'scripts/ci-summary.py'
        self.assertTrue(file.exists(),'independent CI evidence summary is required')
        spec=importlib.util.spec_from_file_location('ci_summary',file)
        self.module=importlib.util.module_from_spec(spec);spec.loader.exec_module(self.module)
        fixture_tmp=tempfile.TemporaryDirectory();self.addCleanup(fixture_tmp.cleanup)
        self.fixture=EvidenceFixture(fixture_tmp.name)
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.artifacts=pathlib.Path(self.tmp.name)
        self.mapping={'tooling-linux':('tooling','COMMAND_TOOLING'),'tooling-windows':('tooling','COMMAND_TOOLING'),
                      'normal-linux':('normal','NORMAL_OFFICIAL_CLIENT'),'normal-windows':('normal','NORMAL_OFFICIAL_CLIENT'),
                      'minimum-normal-linux':('minimum-normal-linux','MINIMUM_GO_COMPATIBILITY'),
                      'minimum-normal-windows':('minimum-normal-windows','MINIMUM_GO_COMPATIBILITY'),
                      'minimum-consumer-linux':('minimum-consumer-linux','MINIMUM_GO_CONSUMERS'),
                      'minimum-consumer-windows':('minimum-consumer-windows','MINIMUM_GO_CONSUMERS'),
                      'fuzz':('fuzz','FUZZ_RUNTIME'),'consumers':('consumer','INDEPENDENT_CONSUMERS'),
                      'security':('security','PINNED_SECURITY_SCANNERS'),'real-integration':('integration','REAL_OPENBAO')}
        for job,(name,kind) in self.mapping.items():
            source={'consumer':'consumers','security':'scans'}.get(name,name)
            report=copy.deepcopy(self.fixture.reports.get(source,self.fixture.reports['normal']))
            report.update(evidence_class=kind,platform='windows' if job.endswith('windows') else 'linux')
            version='1.25.0' if job.startswith('minimum-') else '1.26.8'
            text=f'go version go{version} {report["platform"]}/amd64'
            receipt=self.fixture.receipt(job+'-go-version',text+'\n',['go','version'])
            receipt['name']='go-version'
            report.update(environment_commands=[receipt],tool_versions={'go':text})
            if name=='tooling':
                report['commands']=[self.fixture.receipt('tooling','Ran 9 tests in 0.1s\n\nOK\n',['python','-B','-m','unittest','discover','-s','scripts/tests','-v'])]
            elif name=='fuzz':
                report['commands']=[self.fixture.receipt(n,'fuzz: elapsed: 30s, execs: 99 (3/sec)\nPASS\n',
                    ['go','test','-mod=readonly','-run=^$','-fuzz='+n,'-fuzztime=30s','-parallel=1','-timeout=90s',p]) for n,p in FUZZ_TARGETS.items()]
            dest=self.artifacts/job
            shutil.copytree(self.fixture.root,dest)
            target=dest/'.artifacts'/(name+'-report.json');target.parent.mkdir()
            target.write_text(json.dumps(report),encoding='utf-8')

    def summarize(self):return self.module.summarize(self.artifacts,'a'*64,reference_root=self.fixture.root)

    def mutate(self,job,change):
        name=self.mapping[job][0];p=self.artifacts/job/'.artifacts'/(name+'-report.json')
        value=json.loads(p.read_text(encoding='utf-8'));change(value);p.write_text(json.dumps(value),encoding='utf-8')

    def test_complete_current_artifacts_pass(self):self.assertEqual(self.summarize()['status'],'PASS')
    def test_stale_or_failed_report_fails(self):
        self.mutate('normal-windows',lambda r:r.update(source_sha256='old'))
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_missing_job_fails(self):
        shutil.rmtree(self.artifacts/'security')
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_minimum_job_cannot_use_main_class(self):
        self.mutate('minimum-normal-linux',lambda r:r.update(evidence_class='NORMAL_OFFICIAL_CLIENT'))
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_missing_minimum_consumer_fails(self):
        shutil.rmtree(self.artifacts/'minimum-consumer-windows')
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_server_digest_and_lock_tampering_fail(self):
        self.mutate('real-integration',lambda r:r['baseline'].update(image='openbao/openbao@sha256:'+'c'*64))
        self.assertEqual(self.summarize()['status'],'FAIL')
        lock=self.artifacts/'real-integration/deploy/test/server-lock.json'
        value=json.loads(lock.read_text(encoding='utf-8'));value['image']='openbao/openbao@sha256:'+'c'*64
        lock.write_text(json.dumps(value),encoding='utf-8')
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_wrong_class_or_platform_fails(self):
        self.mutate('normal-windows',lambda r:r.update(platform='linux'))
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_duplicate_report_fails(self):
        p=self.artifacts/'normal-linux/.artifacts/normal-report.json'
        shutil.copyfile(p,p.with_name('unexpected-report.json'))
        self.assertEqual(self.summarize()['status'],'FAIL')
    def test_zero_tooling_targets_fail(self):
        # Preserve digest correctness so rejection must come from target parsing.
        r=self.fixture.receipt('tooling-empty','Ran 0 tests in 0.0s\n\nOK\n')
        for job in ['tooling-linux']:
            shutil.copyfile(self.fixture.root/r['log'],self.artifacts/job/r['log'])
            r['name']='tooling';self.mutate(job,lambda report:report.update(commands=[r]))
        self.assertEqual(self.summarize()['status'],'FAIL')

    def test_export_drops_uncontrolled_output_and_preserves_provenance(self):
        from evidence import report_results
        report=self.fixture.reports['normal']
        command=report['commands'][0]['command']
        secret='_'.join(('PRIVATE','FIXTURE','DO','NOT','EXPORT'))
        raw=self.fixture.events(['TestDependencyBaseline'])+json.dumps({'Action':'output','Package':'example.org/sdk','Output':secret})+'\n'
        report['commands'][0]=self.fixture.receipt('unit',raw,command)
        self.fixture.save()
        output=self.artifacts/'sanitized'
        self.module.package_report(self.fixture.root,'evidence/normal.json',output)
        exported=json.loads((output/'evidence/normal.json').read_text(encoding='utf-8'))
        self.assertEqual(exported['commands'][0]['log_sha256'],report['commands'][0]['log_sha256'])
        self.assertNotIn(secret,'\n'.join(p.read_text(encoding='utf-8') for p in output.rglob('*') if p.is_file()))
        self.assertIn(secret,(self.fixture.root/report['commands'][0]['log']).read_text(encoding='utf-8'))
        self.assertTrue(report_results(output,exported,'a'*64,allow_export=True)['tests'])
        with self.assertRaises(ValueError):report_results(output,exported,'a'*64)

    def test_dynamic_subtest_names_are_not_exported(self):
        report=self.fixture.reports['normal']
        raw=self.fixture.events(['TestDependencyBaseline','TestDependencyBaseline/PRIVATE_FIXTURE_SUBTEST'])
        report['commands'][0]=self.fixture.receipt('unit',raw,report['commands'][0]['command'])
        self.fixture.save()
        output=self.artifacts/'names-export'
        self.module.package_report(self.fixture.root,'evidence/normal.json',output)
        all_text='\n'.join(p.read_text(encoding='utf-8') for p in output.rglob('*') if p.is_file())
        self.assertNotIn('PRIVATE_FIXTURE_SUBTEST',all_text)

    def test_executable_examples_survive_structured_export(self):
        from evidence import report_results
        self.fixture.write('example_test.go', 'package sdk_test\nfunc ExampleNew() {}\nfunc Example() {}\n')
        report=self.fixture.reports['normal']
        raw=self.fixture.events(['TestDependencyBaseline'])+self.fixture.events(['ExampleNew','Example'], 'example.org/sdk')
        report['commands'][0]=self.fixture.receipt('unit',raw,report['commands'][0]['command'])
        self.fixture.save()
        output=self.artifacts/'examples-export'
        self.module.package_report(self.fixture.root,'evidence/normal.json',output)
        exported=json.loads((output/'evidence/normal.json').read_text(encoding='utf-8'))
        tests=report_results(output,exported,'a'*64,allow_export=True)['tests']
        self.assertTrue({('example.org/sdk','ExampleNew'),('example.org/sdk','Example')}.issubset(tests))

    def test_export_path_traversal_is_rejected_before_creation(self):
        report=self.fixture.reports['normal']
        report['commands'][0]['log']='../outside.log'
        self.fixture.save()
        output=self.artifacts/'outside-export'
        with self.assertRaises(ValueError):self.module.package_report(self.fixture.root,'evidence/normal.json',output)
        self.assertFalse(output.exists())

    def test_export_preserves_required_integration_subtests(self):
        from evidence import report_results
        output=self.artifacts/'integration-export'
        self.module.package_report(self.fixture.root,'evidence/integration.json',output)
        report=json.loads((output/'evidence/integration.json').read_text(encoding='utf-8'))
        self.assertEqual(len(report_results(output,report,'a'*64,allow_export=True)['tests']),6)

    def test_no_test_file_package_survives_structured_export(self):
        from evidence import report_results
        self.fixture.write('diagnostics/types.go','package diagnostics\n')
        report=self.fixture.reports['normal']
        raw=self.fixture.events(['TestDependencyBaseline'])+'\n'.join(json.dumps(e) for e in [
            {'Action':'output','Package':'example.org/sdk/diagnostics','Output':'?   \texample.org/sdk/diagnostics\t[no test files]\n'},
            {'Action':'skip','Package':'example.org/sdk/diagnostics'}])
        report['commands'][0]=self.fixture.receipt('unit',raw,report['commands'][0]['command'])
        self.fixture.save()
        output=self.artifacts/'empty-package-export'
        self.module.package_report(self.fixture.root,'evidence/normal.json',output)
        exported=json.loads((output/'evidence/normal.json').read_text(encoding='utf-8'))
        self.assertTrue(report_results(output,exported,'a'*64,allow_export=True)['tests'])


if __name__=='__main__':unittest.main()
