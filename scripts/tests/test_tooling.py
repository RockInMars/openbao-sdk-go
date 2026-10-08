import importlib.util, json, pathlib, re, shlex, subprocess, sys, tempfile, unittest, zipfile
ROOT=pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'scripts'))
import tooling
from evidence import release_problems

class ToolingTests(unittest.TestCase):
 def test_module_zip_preserves_case_and_proxy_path_escapes_it(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'go.mod').write_text('module github.com/RockInMars/openbao-sdk-go\n',encoding='utf-8')
   module='github.com/RockInMars/openbao-sdk-go';out=root/'sdk.zip'
   tooling.module_zip(root,out,module,'v0.1.0-dev.0')
   with zipfile.ZipFile(out) as z:self.assertEqual(z.namelist(),[module+'@v0.1.0-dev.0/go.mod'])
   self.assertEqual(tooling.module_proxy_path(module),'github.com/!rock!in!mars/openbao-sdk-go')
   with self.assertRaises(ValueError):tooling.module_proxy_path('github.com/../outside')
 def test_missing_integration_fails(self):
  for env in ({},{'BAO_TEST_VERSION':'latest'},{'BAO_TEST_VERSION':'2.7.0','BAO_TEST_IMAGE':'openbao/openbao:latest'},{'BAO_TEST_ADDRESS':'https://external.invalid'}):
   with self.assertRaises(ValueError):tooling.integration_lock(env)
 def test_release_requires_real_non_skipped_evidence(self):
  self.assertFalse(tooling.go_test_passes([{'Action':'skip','Test':'TestIntegrationRuntime'}],['TestIntegrationRuntime']))
  self.assertFalse(tooling.go_test_passes([{'Action':'pass','Test':'SomeOtherTest'}],['TestIntegrationRuntime']))
  self.assertTrue(tooling.go_test_passes([{'Action':'pass','Test':'TestIntegrationRuntime'}],['TestIntegrationRuntime']))
 def test_module_zip_excludes_nested_module_and_private_material(self):
  with tempfile.TemporaryDirectory() as d:
   p=pathlib.Path(d);(p/'go.mod').write_text('module github.com/RockInMars/openbao-sdk-go\ngo 1.23.2\n', encoding='utf-8');(p/'client.go').write_text('package bao\n', encoding='utf-8')
   (p/'nested').mkdir();(p/'nested/go.mod').write_text('module nested', encoding='utf-8');(p/'nested/secret.go').write_text('package nested', encoding='utf-8')
   (p/'.env').write_text('must-not-export', encoding='utf-8');(p/'session.key').write_text('must-not-export', encoding='utf-8')
   out=p/'out.zip';tooling.module_zip(p,out,'github.com/RockInMars/openbao-sdk-go','v0.1.0-dev.0')
   with zipfile.ZipFile(out) as z:
    self.assertEqual(sorted(z.namelist()),['github.com/RockInMars/openbao-sdk-go@v0.1.0-dev.0/client.go','github.com/RockInMars/openbao-sdk-go@v0.1.0-dev.0/go.mod'])
 def test_artifact_scan_catches_private_pem_not_a_label(self):
  self.assertTrue(tooling.forbidden_material(b'-----BEGIN PRIVATE KEY-----\nMIIABCDabcdef123456789\n-----END PRIVATE KEY-----'))
  self.assertFalse(tooling.forbidden_material(b'PEM type = PRIVATE KEY; actual fixtures generated in memory'))
 def test_module_zip_does_not_descend_into_excluded_or_nested_trees(self):
  with tempfile.TemporaryDirectory() as d:
   root=pathlib.Path(d);(root/'go.mod').write_text('module example.com/sdk\n',encoding='utf-8')
   (root/'client.go').write_text('package sdk\n',encoding='utf-8')
   (root/'.artifacts/cache').mkdir(parents=True)
   (root/'nested/deep').mkdir(parents=True)
   (root/'nested/go.mod').write_text('module example.com/nested\n',encoding='utf-8')
   forbidden={(root/'.artifacts').resolve(),(root/'nested/deep').resolve()}
   out=root/'sdk.zip'
   # Observe actual directory opens in an isolated process; audit hooks cannot be removed.
   code="""import json,pathlib,sys
sys.path.insert(0,sys.argv[2])
from tooling import module_zip
visited=[]
sys.addaudithook(lambda event,args: visited.append(str(args[0])) if event=='os.scandir' else None)
module_zip(pathlib.Path(sys.argv[1]),pathlib.Path(sys.argv[3]),'example.com/sdk','v0.1.0')
print(json.dumps(visited))
"""
   result=subprocess.run([sys.executable,'-B','-c',code,str(root),str(ROOT/'scripts'),str(out)],capture_output=True,text=True,check=True,timeout=15)
   visited={pathlib.Path(path).resolve() for path in json.loads(result.stdout)}
   self.assertFalse(forbidden & visited,'entered excluded or nested tree')
   with zipfile.ZipFile(out) as archive:
    self.assertEqual(archive.namelist(),['example.com/sdk@v0.1.0/client.go','example.com/sdk@v0.1.0/go.mod'])
 def test_release_gate_rejects_unverified(self):
  self.assertTrue(release_problems(ROOT,{'implementation_status':'IN_PROGRESS'},{'results':[]},{},'current'))
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
  # Bare PASS labels without versioned ledgers and executed targets are invalid.
  self.assertTrue(release_problems(ROOT,ledger,ac,reports,"current"))
  reports["normal"]["evidence_class"]="CONTRACT_WITH_TEST_SENDER"
  self.assertTrue(release_problems(ROOT,ledger,ac,reports,"current"))
  reports["normal"]["evidence_class"]="NORMAL_OFFICIAL_CLIENT";reports["normal"]["source_sha256"]="old"
  self.assertTrue(release_problems(ROOT,ledger,ac,reports,"current"))
 def test_fuzz_commands_select_existing_package_targets(self):
  spec=importlib.util.spec_from_file_location('fuzz_runner',ROOT/'scripts/fuzz-test.py')
  module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
  commands=module.fuzz_commands()
  self.assertEqual(len(commands),5)
  targets=set(commands)
  self.assertEqual(targets,{'FuzzPath','FuzzDocument','FuzzPKICSR','FuzzEncoding','FuzzDecode'})
  for name,words in commands.items():
   directory=ROOT/words[-1]
   declarations='\n'.join(x.read_text(encoding='utf-8') for x in directory.glob('*_test.go'))
   self.assertRegex(declarations,r'func\s+'+re.escape(name)+r'\s*\(',msg=f'{name} is not a runnable target in {words[-1]}')
if __name__=='__main__':unittest.main()
