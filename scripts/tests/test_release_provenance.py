import copy
import json
import pathlib
import sys
import tempfile
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))
from evidence_fixture import EvidenceFixture


class ReleaseProvenanceTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.fixture = EvidenceFixture(temporary.name)

    def test_complete_current_evidence_passes(self):
        self.assertEqual(self.fixture.problems(),[])

    def test_successful_replacement_commands_are_rejected(self):
        for report_name in ['normal','integration','consumers','scans']:
            original=copy.deepcopy(self.fixture.reports[report_name])
            for index in range(len(original['commands'])):
                self.fixture.reports[report_name]=copy.deepcopy(original)
                self.fixture.reports[report_name]['commands'][index]['command']=['echo',*original['commands'][index]['command']]
                self.assertTrue(self.fixture.problems(), (report_name,index))
            self.fixture.reports[report_name]=original

    def test_wrong_working_directory_and_latched_change_are_rejected(self):
        original=copy.deepcopy(self.fixture.reports['normal'])
        self.fixture.reports['normal']['commands'][0]['cwd']=str(self.fixture.root/'other')
        self.assertTrue(self.fixture.problems())
        self.fixture.reports['normal']=original
        self.fixture.reports['normal']['problems']=['baseline_changed']
        self.assertTrue(self.fixture.problems())

    def test_changed_server_digest_is_rejected(self):
        self.fixture.reports['integration']['baseline']['image']='openbao/openbao@sha256:'+'c'*64
        self.assertTrue(self.fixture.problems())

    def test_failed_or_missing_version_probe_cannot_pass(self):
        original=copy.deepcopy(self.fixture.reports['normal'])
        for value in ['failure','missing','incorrect']:
            self.fixture.reports['normal']=copy.deepcopy(original)
            if value=='failure':self.fixture.reports['normal']['environment_commands'][0]['exit_code']=1
            if value=='missing':self.fixture.reports['normal'].pop('environment_commands')
            if value=='incorrect':self.fixture.reports['normal']['tool_versions']['go']='unavailable'
            self.assertTrue(self.fixture.problems(),value)

    def test_rejects_stale_ledger(self):
        self.fixture.ledger['source_sha256']='old'
        self.assertTrue(self.fixture.problems())

    def test_rejects_stale_acceptance_item(self):
        self.fixture.acceptance['results'][0]['source_sha256']='old'
        self.assertTrue(self.fixture.problems())

    def test_rejects_missing_hash_version(self):
        del self.fixture.reports['normal']['source_hash_version']
        self.assertTrue(self.fixture.problems())

    def test_existing_old_evidence_is_rejected(self):
        old=copy.deepcopy(self.fixture.reports['normal']);old['source_sha256']='old'
        self.fixture.write('evidence/old.json',old)
        self.fixture.acceptance['results'][0]['evidence']=['evidence/old.json']
        self.assertTrue(self.fixture.problems())

    def test_pass_report_without_required_target_is_rejected(self):
        self.fixture.reports['normal']['commands'][0]=self.fixture.receipt('unit',self.fixture.events(['SomeOtherTest']))
        self.assertTrue(self.fixture.problems())

    def test_failure_skip_and_incomplete_events_are_rejected(self):
        for action in ['fail','skip','run']:
            with self.subTest(action=action):
                raw=self.fixture.events(['TestDependencyBaseline'])+json.dumps({'Action':action,'Test':'TestOther','Package':'example.org/sdk'})+'\n'
                self.fixture.reports['normal']['commands'][0]=self.fixture.receipt('unit',raw)
                self.assertTrue(self.fixture.problems())

    def test_runtime_cannot_use_impact_review(self):
        self.fixture.acceptance['results'][0]['revalidation']={'method':'impact_review'}
        self.assertTrue(self.fixture.problems())

    def test_missing_rules_and_outside_evidence_fail_closed(self):
        del self.fixture.rules['rules']['AC-001']
        self.assertTrue(self.fixture.problems())
        self.fixture.acceptance['results'][1]['evidence']=['../outside.json']
        self.assertTrue(self.fixture.problems())

    def test_stale_profile_failed_race_and_changed_source_are_rejected(self):
        original=copy.deepcopy(self.fixture.reports['normal'])
        for field,value in [('race',1),('profile','old'),('source','changed')]:
            self.fixture.reports['normal']=copy.deepcopy(original)
            if field=='race':self.fixture.reports['normal']['commands'][1]['exit_code']=value
            if field=='profile':self.fixture.reports['normal']['coverage_profile']['sha256']=value
            if field=='source':self.fixture.reports['normal']['source_after']['source_sha256']=value
            self.assertTrue(self.fixture.problems(),field)


if __name__=='__main__':unittest.main()
