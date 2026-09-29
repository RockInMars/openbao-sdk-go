import importlib.util, pathlib, re, shlex, sys, tempfile, unittest, zipfile
ROOT=pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'scripts'))
import tooling

class ToolingTests(unittest.TestCase):
 def test_missing_integration_fails(self):
  for env in ({},{'BAO_TEST_VERSION':'latest'},{'BAO_TEST_VERSION':'2.7.0','BAO_TEST_IMAGE':'openbao/openbao:latest'},{'BAO_TEST_ADDRESS':'https://external.invalid'}):
   with self.assertRaises(ValueError):tooling.integration_lock(env)
 def test_release_requires_real_non_skipped_evidence(self):
  self.assertFalse(tooling.go_test_passes([{'Action':'skip','Test':'TestIntegrationRuntime'}],['TestIntegrationRuntime']))
  self.assertFalse(tooling.go_test_passes([{'Action':'pass','Test':'SomeOtherTest'}],['TestIntegrationRuntime']))
  self.assertTrue(tooling.go_test_passes([{'Action':'pass','Test':'TestIntegrationRuntime'}],['TestIntegrationRuntime']))
 def test_module_zip_excludes_nested_module_and_private_material(self):
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d);(p/'go.mod').write_text('module git.example.com/infra/openbao-sdk-go\ngo 1.23.2\n');(p/'client.go').write_text('package bao\n')
   (p/'nested').mkdir();(p/'nested/go.mod').write_text('module nested');(p/'nested/secret.go').write_text('package nested')
   (p/'.env').write_text('must-not-export');(p/'session.key').write_text('must-not-export')
   out=p/'out.zip';tooling.module_zip(p,out,'git.example.com/infra/openbao-sdk-go','v0.1.0-dev.0')
   with zipfile.ZipFile(out) as z:
    self.assertEqual(sorted(z.namelist()),['git.example.com/infra/openbao-sdk-go@v0.1.0-dev.0/client.go','git.example.com/infra/openbao-sdk-go@v0.1.0-dev.0/go.mod'])
 def test_artifact_scan_catches_private_pem_not_a_label(self):
  self.assertTrue(tooling.forbidden_material(b'-----BEGIN PRIVATE KEY-----\nMIIABCDabcdef123456789\n-----END PRIVATE KEY-----'))
  self.assertFalse(tooling.forbidden_material(b'PEM type = PRIVATE KEY; actual fixtures generated in memory'))
 def test_release_gate_rejects_unverified(self):
  self.assertTrue(tooling.release_problems({'implementation_status':'IN_PROGRESS'},{'results':[]},{},'current'))
 def test_coverage_merges_duplicate_blocks_without_inflating_denominator(self):
  entries=tooling.parse_coverage("mode: atomic\nm/p/file.go:1.1,2.2 3 0\nm/p/file.go:1.1,2.2 3 2\nm/p/file.go:3.1,4.2 1 0\n")
  self.assertEqual(entries,{"m/p/file.go":(3,4)})
  with self.assertRaises(ValueError):tooling.parse_coverage("mode: atomic\nnot a coverage block\n")
 def test_missing_critical_coverage_is_not_success(self):
  report=tooling.coverage_result({},"m")
  self.assertFalse(report["passed"])
  self.assertTrue(all(x["status"]=="MISSING" for x in report["groups"]))
 def test_release_gate_rejects_partial_or_stale_report(self):
  ledger={"tasks":[{"id":f"OB-{i:03}","status":"VERIFIED"} for i in range(1,19)]}
  ac={"results":[{"id":f"AC-{i:03}","status":"PASS"} for i in range(1,72)]}
  kinds={"integration":"REAL_OPENBAO","consumers":"INDEPENDENT_CONSUMERS","normal":"NORMAL_OFFICIAL_CLIENT","scans":"PINNED_SECURITY_SCANNERS"}
  reports={k:{"status":"PASS","evidence_class":v,"source_sha256":"current"} for k,v in kinds.items()}
  self.assertEqual(tooling.release_problems(ledger,ac,reports,"current"),[])
  reports["normal"]["evidence_class"]="CONTRACT_WITH_TEST_SENDER"
  self.assertTrue(tooling.release_problems(ledger,ac,reports,"current"))
  reports["normal"]["evidence_class"]="NORMAL_OFFICIAL_CLIENT";reports["normal"]["source_sha256"]="old"
  self.assertTrue(tooling.release_problems(ledger,ac,reports,"current"))
 def test_fuzz_commands_select_existing_package_targets(self):
  lines=[x.strip() for x in (ROOT/'Makefile').read_text().splitlines() if '-fuzz=' in x]
  self.assertEqual(len(lines),5)
  targets={next(x.split('=',1)[1] for x in shlex.split(line) if x.startswith('-fuzz=')) for line in lines}
  self.assertEqual(targets,{'FuzzPath','FuzzDocument','FuzzPKICSR','FuzzEncoding','FuzzDecode'})
  for line in lines:
   words=shlex.split(line);name=next(x.split('=',1)[1] for x in words if x.startswith('-fuzz='));directory=ROOT/words[-1]
   declarations='\n'.join(x.read_text() for x in directory.glob('*_test.go'))
   self.assertRegex(declarations,r'func\s+'+re.escape(name)+r'\s*\(',msg=f'{name} is not a runnable target in {words[-1]}')
if __name__=='__main__':unittest.main()
