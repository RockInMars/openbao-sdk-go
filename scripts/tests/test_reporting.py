"""Run lifecycle tests use isolated roots and explicit synthetic receipts."""
import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import reporting


class ReportingTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        (self.root / 'go.mod').write_text('module example.org/fixture\ngo 1.25.0\n', encoding='utf-8')
        patch = mock.patch.object(reporting, 'ROOT', self.root)
        patch.start()
        self.addCleanup(patch.stop)

    def receipt(self, completion='exited', exit_code=0, text='go version go1.26.3 windows/amd64\n', **fields):
        def execute(command, *, cwd, env, log, timeout_s):
            log = Path(log)
            log.write_text(text, encoding='utf-8')
            return {'command': list(command), 'cwd': str(cwd), 'log': log.relative_to(self.root).as_posix(),
                    'completion': completion, 'exit_code': exit_code, 'timeout_s': timeout_s,
                    'log_sha256': hashlib.sha256(log.read_bytes()).hexdigest(), **fields}
        return execute

    def test_constructor_does_not_probe_go_or_exit(self):
        with mock.patch.object(reporting, 'run_command', side_effect=AssertionError('constructor launched a process')) as command:
            run = reporting.Run('fixture', 'COMMAND_RECEIPT')
        command.assert_not_called()
        self.assertEqual(run.report['working_directory'], str(self.root))
        self.assertEqual(run.report['commands'], [])
        self.assertNotIn('environment_commands', run.report)

    def test_go_probe_is_explicit_and_separate_from_work_commands(self):
        run = reporting.Run('normal', 'NORMAL_OFFICIAL_CLIENT')
        with mock.patch.object(reporting, 'run_command', side_effect=self.receipt()) as command:
            run.probe_go()
        self.assertEqual(command.call_args.args[0], ['go', 'version'])
        self.assertEqual(command.call_args.kwargs['env']['GOTOOLCHAIN'], 'local')
        self.assertEqual(command.call_args.kwargs['env']['GOPROXY'], 'off')
        self.assertEqual(command.call_args.kwargs['cwd'], self.root)
        self.assertEqual(run.report['commands'], [])
        self.assertEqual(run.report['environment_commands'][0]['name'], 'go-version')
        self.assertEqual(run.report['tool_versions']['go'], 'go version go1.26.3 windows/amd64')

    def test_probe_failure_is_recorded_and_blocks_further_commands(self):
        cases = [('unavailable', None, '', 'BLOCKED', 2), ('timeout', None, '', 'FAIL', 1),
                 ('interrupted', None, '', 'FAIL', 1), ('exited', 1, '', 'FAIL', 1),
                 ('exited', 0, 'unrecognized output', 'FAIL', 1)]
        for index, (completion, exit_code, text, status, code) in enumerate(cases):
            with self.subTest(completion=completion, exit_code=exit_code):
                run = reporting.Run('probe'+str(index), 'NORMAL_OFFICIAL_CLIENT')
                with mock.patch.object(reporting, 'run_command', side_effect=self.receipt(completion, exit_code, text)) as command:
                    self.assertEqual(reporting.run_cli(run.probe_go), code)
                    with self.assertRaises(reporting.RunStopped):
                        run.command('must-not-run', ['go', 'test'], 1)
                    command.assert_called_once()
                stored = json.loads(next(run.directory.glob('report-*.json')).read_text(encoding='utf-8'))
                self.assertEqual(stored['status'], status)
                self.assertEqual(stored['environment_commands'][0]['completion'], completion)
                self.assertEqual(stored['tool_versions']['go'], 'unavailable')

    def test_incomplete_cleanup_is_saved_and_latched(self):
        run = reporting.Run('cleanup', 'NORMAL_OFFICIAL_CLIENT')
        with mock.patch.object(reporting, 'run_command', side_effect=self.receipt('timeout', None, '', cleanup_incomplete=True)) as command:
            with self.assertRaises(reporting.RunStopped) as stopped:
                run.command('first', ['go', 'test'], 1)
            self.assertEqual(stopped.exception.exit_code, 1)
            with self.assertRaises(reporting.RunStopped):
                run.command('second', ['go', 'test'], 1)
            command.assert_called_once()
        self.assertEqual(run.finish('PASS'), 1)
        self.assertEqual(run.report['status'], 'FAIL')
        self.assertTrue(run.report['commands'][0]['cleanup_incomplete'])
        self.assertIn('cleanup_incomplete', run.report['reason'])

    def test_wrong_profile_toolchain_stops_before_work(self):
        run = reporting.Run('minimum-normal-windows', 'MINIMUM_GO_COMPATIBILITY')
        with mock.patch.object(reporting, 'run_command', side_effect=self.receipt()) as command:
            self.assertEqual(reporting.run_cli(lambda: run.probe_go(expected_version='1.25.0')), 2)
            with self.assertRaises(reporting.RunStopped):
                run.command('must-not-run', ['go', 'test'], 1)
            command.assert_called_once()
        self.assertEqual(run.report['status'], 'BLOCKED')
        self.assertEqual(run.report['commands'], [])
        self.assertEqual(run.report['tool_versions']['go'], 'go version go1.26.3 windows/amd64')

    def test_matching_minimum_toolchain_probe_accepts(self):
        run = reporting.Run('minimum-normal-windows', 'MINIMUM_GO_COMPATIBILITY')
        with mock.patch.object(reporting, 'run_command', side_effect=self.receipt(text='go version go1.25.0 windows/amd64\n')):
            run.probe_go(expected_version='1.25.0')
        self.assertEqual(run.report['commands'], [])

    def test_probe_cleanup_failure_is_not_only_an_environment_block(self):
        run = reporting.Run('probe-cleanup', 'NORMAL_OFFICIAL_CLIENT')
        with mock.patch.object(reporting, 'run_command', side_effect=self.receipt('unavailable', None, '', cleanup_incomplete=True)):
            self.assertEqual(reporting.run_cli(run.probe_go), 1)
        self.assertEqual(run.report['status'], 'FAIL')

    def test_source_change_stays_failed_after_bytes_are_restored(self):
        run = reporting.Run('identity', 'COMMAND_RECEIPT')
        original = (self.root / 'go.mod').read_bytes()
        (self.root / 'go.mod').write_bytes(original + b'\n')
        run.finish('PASS')
        (self.root / 'go.mod').write_bytes(original)
        self.assertEqual(run.finish('PASS'), 1)
        self.assertIn('baseline_changed', run.report['problems'])

    def test_immutable_runs_and_latest_copy_keep_distinct_roles(self):
        run = reporting.Run('immutable', 'COMMAND_RECEIPT', run_id='fixture-fixed')
        run.finish('FAIL', 'synthetic first result')
        first = next(run.directory.glob('report-*.json'))
        original = first.read_bytes()
        run.finish('PASS')
        self.assertEqual(first.read_bytes(), original)
        self.assertEqual(len(list(run.directory.glob('report-*.json'))), 2)
        latest = json.loads((self.root / '.artifacts/immutable-report.json').read_text(encoding='utf-8'))
        self.assertEqual(latest['report_path'], run.report['report_path'])
        with self.assertRaises(FileExistsError):
            reporting.Run('immutable', 'COMMAND_RECEIPT', run_id='fixture-fixed')

    def test_generic_record_runs_python_with_no_go_on_path(self):
        record = reporting.RunRecord('COMMAND_RECEIPT', root=self.root, run_id='fixture-python')
        env = {**os.environ, 'PATH': ''}
        receipt = record.command('python', [sys.executable, '-B', '-c', 'print(42)'], 10,
                                 log=self.root/'python.log', env=env)
        self.assertEqual(receipt['exit_code'], 0)
        self.assertEqual(receipt['completion'], 'exited')
        record.finalize('PASS')
        target = self.root/'receipt.json'
        record.write(target)
        original = target.read_bytes()
        with self.assertRaises(FileExistsError):
            record.write(target)
        self.assertEqual(target.read_bytes(), original)
        self.assertNotIn('go', record.report['tool_versions'])
        self.assertEqual(record.report['source_before'], record.report['source_after'])

    def test_cli_adapter_does_not_hide_unexpected_errors(self):
        def fail():
            raise ValueError('unexpected fixture error')
        with self.assertRaisesRegex(ValueError, 'unexpected fixture error'):
            reporting.run_cli(fail)


if __name__ == '__main__':
    unittest.main()
