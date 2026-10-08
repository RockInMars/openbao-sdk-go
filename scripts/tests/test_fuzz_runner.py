import importlib.util
import pathlib
import sys
import unittest

ROOT=pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'scripts'))
from evidence import fuzz_executions


class FuzzRunnerTests(unittest.TestCase):
    def test_actual_go126_windows_output(self):
        raw=(ROOT/'scripts/tests/fixtures/fuzz-go1.26.3-windows.txt').read_text(encoding='utf-8')
        self.assertEqual(fuzz_executions(raw),11743)

    def test_each_declared_target_has_its_own_bounded_command(self):
        file=ROOT/'scripts/fuzz-test.py'
        self.assertTrue(file.exists(),'a per-target fuzz runner is required')
        spec=importlib.util.spec_from_file_location('fuzz_runner',file)
        module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
        commands=module.fuzz_commands()
        self.assertEqual(set(commands),{'FuzzPath','FuzzDocument','FuzzPKICSR','FuzzEncoding','FuzzDecode'})
        for name,cmd in commands.items():
            self.assertIn('-fuzz='+name,cmd)
            self.assertIn('-fuzztime=30s',cmd)
            self.assertIn('-timeout=90s',cmd)
            self.assertIn('-parallel=1',cmd)
            declarations='\n'.join(p.read_text(encoding='utf-8') for p in (ROOT/cmd[-1]).glob('*_test.go'))
            self.assertIn('func '+name+'(',declarations)

    def test_seeds_zero_work_or_unrecognized_output_is_not_timed_fuzz(self):
        for raw in ['PASS\nok\n','fuzz: elapsed: 1s, execs: 0 (0/sec)\nPASS\n',
                    'fuzz: elapsed: 1s, execs: 27 (27/sec)\nFAIL\n','no tests to run\nPASS\n']:
            with self.subTest(raw=raw),self.assertRaises(ValueError):fuzz_executions(raw)
        self.assertEqual(fuzz_executions('fuzz: elapsed: 1s, execs: 27 (27/sec), new interesting: 0\nPASS\n'),27)


if __name__=='__main__':unittest.main()
