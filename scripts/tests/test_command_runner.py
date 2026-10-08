import importlib
import importlib.util
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1]))


class CommandRunnerTests(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(importlib.util.find_spec('command_runner'), 'bounded command runner is required')
        self.runner = importlib.import_module('command_runner')
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)

    def run_python(self, source, timeout=3):
        return self.runner.run_command([sys.executable, '-c', source], cwd=self.root,
                                       env=os.environ.copy(), log=self.root/'command.log', timeout_s=timeout)

    def test_natural_exit_and_utf8_log(self):
        r = self.run_python("print('hello'); raise SystemExit(7)")
        self.assertEqual((r['completion'], r['exit_code']), ('exited', 7))
        self.assertIn('hello', (self.root/'command.log').read_text(encoding='utf-8'))
        self.assertEqual(r['log_sha256'], self.runner.file_sha256(self.root/'command.log'))

    def test_success_requires_natural_exit(self):
        r = self.run_python("print('done')")
        self.assertTrue(self.runner.succeeded(r))
        self.assertFalse(self.runner.succeeded({'completion': 'timeout', 'exit_code': 0}))

    def test_missing_executable_is_not_success(self):
        r = self.runner.run_command([str(self.root/'missing-program')], cwd=self.root,
                                    env=os.environ.copy(), log=self.root/'missing.log', timeout_s=1)
        self.assertEqual(r['completion'], 'unavailable')
        self.assertIsNone(r['exit_code'])

    def test_timeout_keeps_partial_output_and_kills_owned_child(self):
        marker = self.root/'escaped.txt'
        child = 'import time,pathlib; time.sleep(2); pathlib.Path('+repr(str(marker))+').write_text("escaped")'
        parent = 'import subprocess,sys,time; subprocess.Popen([sys.executable,"-c",'+repr(child)+']); print("started",flush=True); time.sleep(30)'
        r = self.run_python(parent, .8)
        self.assertEqual(r['completion'], 'timeout')
        self.assertIsNone(r['exit_code'])
        self.assertIn('started', (self.root/'command.log').read_text(encoding='utf-8'))
        time.sleep(1.5)
        self.assertFalse(marker.exists(), 'owned child survived the deadline')

    def test_interruption_is_recorded_and_reaped(self):
        original = subprocess.Popen.wait
        called = False
        def interrupted(process, *args, **kwargs):
            nonlocal called
            if not called:
                called = True
                raise KeyboardInterrupt
            return original(process, *args, **kwargs)
        with mock.patch.object(subprocess.Popen, 'wait', interrupted):
            r = self.run_python('import time; time.sleep(30)')
        self.assertEqual(r['completion'], 'interrupted')
        self.assertIsNone(r['exit_code'])

    def test_logs_cannot_be_overwritten(self):
        self.run_python("print('original')")
        with self.assertRaises(FileExistsError):
            self.run_python("print('replacement')")
        self.assertIn('original', (self.root/'command.log').read_text(encoding='utf-8'))

    def test_cleanup_error_is_explicitly_incomplete(self):
        # Reap the real owned child, then simulate failure to confirm the receipt
        # blocks further work without leaving a process behind in the test host.
        stop = self.runner._stop_owned
        def fail_after_reap(process):
            stop(process)
            raise OSError('fixture cleanup confirmation failure')
        with mock.patch.object(self.runner, '_stop_owned', fail_after_reap):
            receipt = self.run_python('import time; time.sleep(30)', .2)
        self.assertTrue(receipt.get('cleanup_incomplete'))
        self.assertFalse(self.runner.succeeded(receipt))

    def test_taskkill_does_not_inherit_remote_credentials(self):
        process = mock.Mock(pid=12345)
        process.wait.return_value = 1
        with mock.patch.dict(os.environ, {'BAO_REMOTE_TOKEN': 'synthetic-admin',
                                          'BAO_REMOTE_APPROLE_SECRET_ID': 'synthetic-secret',
                                          'VAULT_TOKEN': 'synthetic-unrelated'}), \
             mock.patch.object(self.runner.os, 'name', 'nt'), \
             mock.patch.object(self.runner.subprocess, 'CREATE_NO_WINDOW', 0x08000000, create=True), \
             mock.patch.object(self.runner.subprocess, 'run') as called:
            self.runner._stop_owned(process)
        environment = called.call_args.kwargs.get('env')
        self.assertIsInstance(environment, dict)
        self.assertFalse(any(k.startswith('BAO_REMOTE_') or k == 'VAULT_TOKEN' for k in environment))


if __name__ == '__main__':
    unittest.main()
