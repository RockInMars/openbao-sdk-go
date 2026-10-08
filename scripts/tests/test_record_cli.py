import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[2]


class RecordCLITests(unittest.TestCase):
    def test_record_has_natural_exit_and_refuses_overwrite(self):
        with tempfile.TemporaryDirectory() as d:
            cmd=[sys.executable,'-B',str(ROOT/'scripts/record.py'),'--output-dir',d,'--run-id','test-run',
                 'OB-018','tooling','--',sys.executable,'-c','print("done")']
            env=dict(os.environ,PATH='')
            r=subprocess.run(cmd,cwd=ROOT,env=env,capture_output=True,timeout=20)
            self.assertEqual(r.returncode,0,r.stderr.decode('utf-8',errors='replace'))
            record=json.loads((pathlib.Path(d)/'tooling-test-run.json').read_text(encoding='utf-8'))
            self.assertNotIn('go',record['tool_versions'])
            self.assertNotIn('environment_commands',record)
            self.assertEqual(record['commands'][0]['completion'],'exited')
            self.assertEqual(record['source_before'],record['source_after'])
            original=(pathlib.Path(d)/'tooling-test-run.log').read_bytes()
            again=subprocess.run(cmd,cwd=ROOT,capture_output=True,timeout=20)
            self.assertNotEqual(again.returncode,0)
            self.assertEqual((pathlib.Path(d)/'tooling-test-run.log').read_bytes(),original)


if __name__=='__main__':unittest.main()
