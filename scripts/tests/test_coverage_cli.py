import json
import pathlib
import re
import subprocess
import sys
import tempfile
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[2]


class CoverageCLITests(unittest.TestCase):
    def test_selected_profile_is_used(self):
        module=re.search(r'^module\s+(\S+)',(ROOT/'go.mod').read_text(encoding='utf-8'),re.M)[1]
        paths=['client.go','kv_client.go','pki_client.go','transit_client.go']+[p+'/a.go' for p in ['internal/engine','internal/authn','auth','kv','pki','baoerr','sensitive','internal/jsondoc','internal/pkiutil','internal/transitutil','internal/pemutil']]
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);profile=root/'new.out';out=root/'report.json'
            profile.write_text('mode: atomic\n'+''.join(module+'/'+p+':1.1,2.1 1 1\n' for p in paths),encoding='utf-8')
            r=subprocess.run([sys.executable,'-B',str(ROOT/'scripts/coverage-check.py'),'--profile',str(profile),'--output',str(out)],capture_output=True,timeout=20)
            self.assertEqual(r.returncode,0,r.stdout.decode('utf-8',errors='replace'))
            report=json.loads(out.read_text(encoding='utf-8'))
            self.assertTrue(report['passed'])
            self.assertEqual(report.get('source_hash_version'),2)
            self.assertEqual(report['source_before'],report['source_after'])
            self.assertEqual(report['status'],'PASS')
            self.assertTrue(report['profile_sha256'])

    def test_missing_and_malformed_profiles_fail_without_fallback(self):
        with tempfile.TemporaryDirectory() as d:
            root=pathlib.Path(d);profile=root/'missing.out';out=root/'report.json'
            for malformed in (False,True):
                if malformed:profile.write_text('not a profile',encoding='utf-8')
                r=subprocess.run([sys.executable,'-B',str(ROOT/'scripts/coverage-check.py'),'--profile',str(profile),'--output',str(out)],capture_output=True,timeout=20)
                self.assertNotEqual(r.returncode,0)
                self.assertTrue(out.exists(),'failure should preserve a report for the selected profile')
                report=json.loads(out.read_text(encoding='utf-8'))
                self.assertFalse(report['passed'])
                self.assertIn('reason',report)


if __name__=='__main__':unittest.main()
