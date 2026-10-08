import contextlib
import importlib.util
import io
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
from evidence import FORMAL_REPORT_NAMES, release_problems
from evidence_fixture import EvidenceFixture
from tooling import source_hash

MODULE = 'github.com/RockInMars/openbao-sdk-go'
SCRIPT = ROOT / 'scripts/release.py'


class ReleaseEvidenceFixture(EvidenceFixture):
    def write(self, name, value):
        if isinstance(value, str):
            value = value.replace('example.org/sdk', MODULE)
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(value if isinstance(value, str) else json.dumps(value), encoding='utf-8', newline='\n')
        return path


class GitReleaseTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(SCRIPT.is_file(), 'release CLI is not implemented')
        spec = importlib.util.spec_from_file_location('sdk_release', SCRIPT)
        self.release = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.release)
        temporary = tempfile.TemporaryDirectory(prefix='sdk-release-test-')
        self.addCleanup(temporary.cleanup)
        self.base = pathlib.Path(temporary.name)
        self.root = self.base / 'sdk'
        self.root.mkdir()
        environment = {key: value for key, value in os.environ.items()
                       if not key.upper().startswith(('GIT_', 'ECC_SKIP_'))}
        environment.update(GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=str(self.base / 'empty-config'),
                           GIT_TERMINAL_PROMPT='0', GIT_AUTHOR_NAME='SDK release tests',
                           GIT_AUTHOR_EMAIL='sdk-release@example.invalid',
                           GIT_COMMITTER_NAME='SDK release tests',
                           GIT_COMMITTER_EMAIL='sdk-release@example.invalid')
        environment_patch = mock.patch.dict(os.environ, environment, clear=True)
        environment_patch.start()
        self.addCleanup(environment_patch.stop)
        self.git('init', '-b', 'main')
        self.git('config', 'core.autocrlf', 'false')
        self.fixture = ReleaseEvidenceFixture(self.root)
        self.fixture.write('.gitignore', '.artifacts/\n.codex/\n.serena/\n')
        self.fixture.write('LICENSE', 'Apache-2.0 synthetic fixture\n')
        self.fixture.write('.codex/config.toml', 'local = 1\n')
        self.bind_evidence()
        paths = sorted(path.relative_to(self.root).as_posix() for path in self.root.rglob('*')
                       if path.is_file() and path.relative_to(self.root).parts[0] not in {'.git', '.artifacts', '.codex', '.serena'})
        self.git('add', '--', *paths)
        self.git('commit', '-m', 'test: prepare synthetic release evidence')
        self.commit = self.git('rev-parse', 'HEAD').strip()
        self.destination = self.base / 'destination.git'
        self.git('init', '--bare', str(self.destination))
        self.git('remote', 'add', 'origin', str(self.destination))

    def git(self, *arguments, cwd=None):
        result = subprocess.run(['git', *arguments], cwd=cwd or self.root, capture_output=True,
                                text=True, encoding='utf-8', timeout=30, check=True)
        return result.stdout

    def bind_evidence(self):
        digest = source_hash(self.root)

        def bind(value):
            if isinstance(value, dict):
                if 'source_sha256' in value:
                    value['source_sha256'] = digest
                for child in value.values():
                    bind(child)
            elif isinstance(value, list):
                for child in value:
                    bind(child)

        for record in (self.fixture.identity, self.fixture.ledger, self.fixture.acceptance,
                       self.fixture.reports):
            bind(record)
        self.fixture.save()
        self.fixture.write('task-status.json', self.fixture.ledger)
        self.fixture.write('acceptance-results.json', self.fixture.acceptance)
        for name, alias in FORMAL_REPORT_NAMES.items():
            self.fixture.write('.artifacts/' + alias + '-report.json', self.fixture.reports[name])
        self.assertEqual(release_problems(self.root, self.fixture.ledger, self.fixture.acceptance,
                                         self.fixture.reports, digest), [])

    def call(self, *arguments):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            try:
                code = self.release.main(['--version', 'v0.1.0', *arguments], root=self.root)
            except SystemExit as error:
                code = error.code
        return code, output.getvalue()

    def tags(self, remote=False):
        return self.git('for-each-ref', '--format=%(refname)', 'refs/tags',
                        cwd=self.destination if remote else self.root).splitlines()

    def assert_no_tags(self):
        self.assertEqual(self.tags(), [])
        self.assertEqual(self.tags(remote=True), [])

    def hook(self, body):
        path = self.root / '.git/hooks/pre-push'
        path.write_text('#!/bin/sh\n' + body + '\n', encoding='utf-8')
        path.chmod(0o755)

    def test_default_preflight_is_read_only(self):
        before = {path: path.read_bytes() for path in self.root.rglob('*')
                  if path.is_file() and '.git' not in path.relative_to(self.root).parts}
        code, output = self.call()
        self.assertEqual(code, 0, output)
        self.assertIn('DRY-RUN: PASS', output)
        self.assertIn(self.commit, output)
        self.assert_no_tags()
        self.assertEqual(self.git('rev-parse', 'HEAD').strip(), self.commit)
        self.assertEqual(before, {path: path.read_bytes() for path in before})

    def test_publish_creates_annotated_tag_and_verifies_destination(self):
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 0, output)
        self.assertIn('RELEASE: PUBLISHED', output)
        self.assertEqual(self.git('cat-file', '-t', 'refs/tags/v0.1.0').strip(), 'tag')
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0^{}', cwd=self.destination).strip(), self.commit)
        self.assertEqual(self.tags(remote=True), ['refs/tags/v0.1.0'])
        self.assertEqual(self.git('for-each-ref', '--format=%(refname)', 'refs/heads', cwd=self.destination), '')
        self.assertEqual(self.git('rev-parse', 'HEAD').strip(), self.commit)

    def test_publish_requires_explicit_full_commit(self):
        for arguments in [('--publish',), ('--publish', '--commit', self.commit[:12]),
                          ('--publish', '--commit', 'HEAD')]:
            with self.subTest(arguments=arguments):
                code, output = self.call(*arguments)
                self.assertNotEqual(code, 0, output)
                self.assert_no_tags()

    def test_other_commit_is_not_checked_using_current_worktree(self):
        self.git('commit', '--allow-empty', '-m', 'test: move HEAD')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('HEAD', output)
        self.assert_no_tags()

    def test_noncanonical_versions_are_rejected(self):
        for version in ('0.1.0', 'v0.01.0', 'v1.0.0-01', 'v1.0.0+build', 'v2.0.0', '-f'):
            with self.subTest(version=version):
                code, output = self.call('--version=' + version, '--publish', '--commit', self.commit)
                self.assertNotEqual(code, 0, output)
                self.assert_no_tags()

    def test_prerelease_is_a_valid_go_module_tag(self):
        code, output = self.call('--version=v0.1.0-rc.1')
        self.assertEqual(code, 0, output)
        self.assert_no_tags()

    def test_failed_acceptance_blocks_publication(self):
        self.fixture.acceptance['results'][0]['status'] = 'FAIL'
        self.fixture.write('acceptance-results.json', self.fixture.acceptance)
        self.git('add', '--', 'acceptance-results.json')
        self.git('commit', '-m', 'test: invalidate acceptance')
        candidate = self.git('rev-parse', 'HEAD').strip()
        code, output = self.call('--publish', '--commit', candidate)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE GATE: FAIL', output)
        self.assert_no_tags()

    def test_stale_report_blocks_even_dry_run(self):
        path = self.root / '.artifacts/normal-report.json'
        report = json.loads(path.read_text(encoding='utf-8'))
        report['source_sha256'] = 'b' * 64
        self.fixture.write('.artifacts/normal-report.json', report)
        code, output = self.call()
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE GATE: FAIL', output)
        self.assert_no_tags()

    def test_dirty_sdk_source_blocks_publication(self):
        with (self.root / 'go.mod').open('a', encoding='utf-8') as stream:
            stream.write('// uncommitted\n')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('uncommitted', output)
        self.assert_no_tags()

    def test_staged_changes_block_publication(self):
        self.fixture.write('notes.md', 'uncommitted release notes\n')
        self.git('add', '--', 'notes.md')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('index', output)
        self.assert_no_tags()

    def test_ignored_source_cannot_be_tagged_as_committed_source(self):
        self.fixture.write('.gitignore', '.artifacts/\nignored.go\n')
        self.git('add', '--', '.gitignore')
        self.git('commit', '-m', 'test: ignore a source input')
        self.fixture.write('ignored.go', 'package sdk\n')
        code, output = self.call()
        self.assertEqual(code, 1, output)
        self.assertIn('not in commit', output)
        self.assert_no_tags()

    def test_machine_local_changes_are_preserved_and_not_published(self):
        self.fixture.write('.codex/config.toml', 'local = 2\n')
        self.fixture.write('.serena/project.yml', 'local: true\n')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 0, output)
        self.assertEqual((self.root / '.codex/config.toml').read_text(encoding='utf-8'), 'local = 2\n')
        self.assertTrue((self.root / '.serena/project.yml').is_file())
        published = self.git('ls-tree', '-r', '--name-only', 'refs/tags/v0.1.0', cwd=self.destination).splitlines()
        self.assertFalse(any(path.startswith(('.codex/', '.serena/')) for path in published))

    def test_committed_machine_configuration_is_refused(self):
        self.git('add', '--force', '--', '.codex/config.toml')
        self.git('commit', '-m', 'test: include forbidden machine configuration')
        self.commit = self.git('rev-parse', 'HEAD').strip()
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('excluded path committed: .codex/config.toml', output)
        self.assert_no_tags()

    def test_committed_runtime_artifact_is_refused(self):
        self.fixture.write('.artifacts/private.txt', 'synthetic local artifact\n')
        self.git('add', '--force', '--', '.artifacts/private.txt')
        self.git('commit', '-m', 'test: include forbidden runtime artifact')
        self.commit = self.git('rev-parse', 'HEAD').strip()
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('excluded path committed: .artifacts/private.txt', output)
        self.assert_no_tags()

    def test_git_normalized_line_endings_are_not_source_identity(self):
        self.git('config', 'core.autocrlf', 'true')
        path = self.root / 'go.mod'
        path.write_bytes(path.read_bytes().replace(b'\n', b'\r\n'))
        self.assertEqual(self.git('diff', '--name-only', 'HEAD'), '')
        code, output = self.call()
        self.assertEqual(code, 1, output)
        self.assertIn('source bytes differ from commit', output)
        self.assert_no_tags()

    def test_local_tag_collision_is_never_overwritten(self):
        self.git('tag', 'v0.1.0', self.commit)
        original = self.git('rev-parse', 'refs/tags/v0.1.0').strip()
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0').strip(), original)
        self.assertEqual(self.tags(remote=True), [])

    def test_remote_tag_collision_is_never_overwritten(self):
        self.git('push', 'origin', self.commit + ':refs/tags/v0.1.0')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertEqual(self.tags(), [])
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0', cwd=self.destination).strip(), self.commit)

    def test_checks_push_destination_not_fetch_url(self):
        self.git('remote', 'set-url', 'origin', str(self.base / 'absent-fetch.git'))
        self.git('remote', 'set-url', '--push', 'origin', str(self.destination))
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.tags(remote=True), ['refs/tags/v0.1.0'])

    def test_multiple_push_destinations_are_refused(self):
        self.git('remote', 'set-url', '--add', '--push', 'origin', str(self.destination))
        self.git('remote', 'set-url', '--add', '--push', 'origin', str(self.base / 'other.git'))
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('one push destination', output)
        self.assert_no_tags()

    def test_follow_tags_configuration_cannot_publish_unrequested_tags(self):
        self.git('tag', '-a', 'v0.0.1', self.commit, '-m', 'Unrequested fixture tag')
        self.git('config', 'push.followTags', 'true')
        self.git('config', 'remote.origin.mirror', 'true')
        self.git('config', 'push.recurseSubmodules', 'on-demand')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 0, output)
        self.assertEqual(self.tags(remote=True), ['refs/tags/v0.1.0'])

    def test_pre_push_hook_runs_and_failure_preserves_local_tag(self):
        self.hook('printf ran > .artifacts/pre-push-ran\nexit 1')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: PARTIAL', output)
        self.assertTrue((self.root / '.artifacts/pre-push-ran').is_file())
        self.assertEqual(self.tags(), ['refs/tags/v0.1.0'])
        self.assertEqual(self.tags(remote=True), [])
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('already exists', output)

    def test_timed_out_hook_is_not_retried_or_hidden(self):
        self.hook('sleep 5\nexit 1')
        code, output = self.call('--publish', '--commit', self.commit, '--timeout', '1')
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: PARTIAL', output)
        self.assertIn('timed out', output)
        self.assertEqual(self.tags(), ['refs/tags/v0.1.0'])
        self.assertEqual(self.tags(remote=True), [])

    def test_source_change_during_remote_preflight_blocks_tag_creation(self):
        read_tags = self.release.remote_tags

        def change_source(git, destination, tag_ref):
            observed = read_tags(git, destination, tag_ref)
            self.fixture.write('go.mod', 'module ' + MODULE + '\ngo 1.25.0\n// changed\n')
            return observed

        with mock.patch.object(self.release, 'remote_tags', change_source):
            code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('uncommitted', output)
        self.assert_no_tags()

    def test_evidence_change_during_remote_preflight_is_rechecked(self):
        read_tags = self.release.remote_tags

        def invalidate_evidence(git, destination, tag_ref):
            observed = read_tags(git, destination, tag_ref)
            self.fixture.write('.artifacts/normal-report.json', {})
            return observed

        with mock.patch.object(self.release, 'remote_tags', invalidate_evidence):
            code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE GATE: FAIL', output)
        self.assert_no_tags()

    def test_remote_verification_mismatch_is_partial_not_success(self):
        read_tags = self.release.remote_tags

        def mismatching_remote(git, destination, tag_ref):
            observed = read_tags(git, destination, tag_ref)
            if observed:
                observed[tag_ref + '^{}'] = '0' * len(self.commit)
            return observed

        with mock.patch.object(self.release, 'remote_tags', mismatching_remote):
            code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: PARTIAL', output)
        self.assertIn('mismatch', output)
        self.assertNotIn('RELEASE: PUBLISHED', output)
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0^{}', cwd=self.destination).strip(), self.commit)

    def test_hook_changing_head_does_not_change_the_fixed_remote_tag(self):
        self.hook('git commit --allow-empty -m "test: move HEAD during hook"\nexit 0')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: PARTIAL', output)
        self.assertNotEqual(self.git('rev-parse', 'HEAD').strip(), self.commit)
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0^{}', cwd=self.destination).strip(), self.commit)

    def test_git_failure_does_not_echo_private_endpoint(self):
        destination = str(self.base / 'private-remote-address.git')
        self.git('remote', 'set-url', '--push', 'origin', destination)
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('git ls-remote failed', output)
        self.assertNotIn(destination, output)
        self.assert_no_tags()

    def test_direction_dependent_url_rewrite_cannot_push_to_an_unchecked_repository(self):
        alternate = self.base / 'rewritten.git'
        self.git('init', '--bare', str(alternate))
        self.git('remote', 'set-url', '--push', 'origin', str(self.destination))
        self.git('config', 'url.' + str(alternate) + '.pushInsteadOf', str(self.destination))
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: BLOCKED', output)
        self.assert_no_tags()
        self.assertEqual(self.git('for-each-ref', '--format=%(refname)', 'refs/tags', cwd=alternate), '')

    def test_replacement_refs_cannot_bind_evidence_to_a_different_commit_tree(self):
        self.fixture.write('go.mod', 'module ' + MODULE + '\ngo 1.25.0\n\n')
        self.bind_evidence()
        self.git('add', '--', 'go.mod', 'task-status.json', 'acceptance-results.json', 'evidence')
        self.git('commit', '-m', 'test: prepare replacement tree')
        replacement = self.git('rev-parse', 'HEAD').strip()
        self.git('update-ref', 'HEAD', self.commit)
        self.git('replace', self.commit, replacement)
        self.assertEqual(self.git('diff', '--name-only', 'HEAD'), '')
        code, output = self.call('--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertNotIn('DRY-RUN: PASS', output)
        self.assert_no_tags()

    def test_committed_source_hidden_by_skip_worktree_is_rejected(self):
        self.fixture.write('hidden.go', 'package sdk\n')
        self.git('add', '--', 'hidden.go')
        self.git('commit', '-m', 'test: include hidden source')
        self.git('update-index', '--skip-worktree', '--', 'hidden.go')
        (self.root / 'hidden.go').unlink()
        self.assertEqual(self.git('diff', '--name-only', 'HEAD'), '')
        code, output = self.call()
        self.assertEqual(code, 1, output)
        self.assertNotIn('DRY-RUN: PASS', output)
        self.assert_no_tags()
        with self.assertRaisesRegex(self.release.ReleaseError, 'missing from checkout'):
            self.release.check_source_bytes(self.release.Git(self.root, 30), self.git('rev-parse', 'HEAD').strip())

    def test_assume_unchanged_cannot_hide_uncommitted_documentation(self):
        self.fixture.write('README.md', 'Committed documentation\n')
        self.git('add', '--', 'README.md')
        self.git('commit', '-m', 'test: include documentation')
        self.git('update-index', '--assume-unchanged', '--', 'README.md')
        self.fixture.write('README.md', 'Uncommitted documentation\n')
        self.assertEqual(self.git('diff', '--name-only', 'HEAD'), '')
        code, output = self.call()
        self.assertEqual(code, 1, output)
        self.assert_no_tags()

    def test_interrupt_after_tag_creation_reports_partial_and_preserves_refs(self):
        check_candidate = self.release.check_candidate

        def interrupt_after_tag(*arguments, **options):
            if self.tags():
                raise KeyboardInterrupt
            return check_candidate(*arguments, **options)

        with mock.patch.object(self.release, 'check_candidate', interrupt_after_tag):
            try:
                code, output = self.call('--publish', '--commit', self.commit)
            except KeyboardInterrupt:
                self.fail('publish interruption escaped without a PARTIAL result')
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: PARTIAL', output)
        self.assertIn('interrupted', output)
        self.assertEqual(self.tags(), ['refs/tags/v0.1.0'])
        self.assertEqual(self.tags(remote=True), [])

    def test_hook_retargeting_local_tag_does_not_report_complete_success(self):
        self.hook('git update-ref refs/tags/v0.1.0 ' + self.commit + '\nexit 0')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: PARTIAL', output)
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0').strip(), self.commit)
        self.assertEqual(self.git('cat-file', '-t', 'refs/tags/v0.1.0', cwd=self.destination).strip(), 'tag')
        self.assertEqual(self.git('rev-parse', 'refs/tags/v0.1.0^{}', cwd=self.destination).strip(), self.commit)

    def test_push_destination_cannot_resolve_as_another_remote(self):
        alternate = self.base / 'alternate.git'
        additional = self.base / 'additional.git'
        for path in (alternate, additional):
            self.git('init', '--bare', str(path))
        self.git('remote', 'add', 'secondary', str(self.destination))
        self.git('remote', 'set-url', '--add', '--push', 'secondary', str(alternate))
        self.git('remote', 'set-url', '--add', '--push', 'secondary', str(additional))
        self.git('remote', 'set-url', '--push', 'origin', 'secondary')
        code, output = self.call('--publish', '--commit', self.commit)
        self.assertEqual(code, 1, output)
        self.assertIn('RELEASE: BLOCKED', output)
        self.assert_no_tags()
        for destination in (alternate, additional):
            self.assertEqual(self.git('for-each-ref', '--format=%(refname)', 'refs/tags', cwd=destination), '')


if __name__ == '__main__':
    unittest.main()
