#!/usr/bin/env python3
"""Preflight a Go SDK tag; only --publish creates and pushes it."""
import argparse
import json
import os
import pathlib
import re
import subprocess

from command_runner import _stop_owned
from evidence import FORMAL_REPORT_NAMES, release_problems, safe_file
from tooling import EXCLUDED, ROOT, _source_input, source_files, source_hash

MODULE = 'github.com/RockInMars/openbao-sdk-go'


class ReleaseError(Exception):
    pass


class Git:
    def __init__(self, root, timeout):
        self.root = root
        self.timeout = timeout

    def run(self, *arguments, input_data=None, allowed=(0,)):
        environment = dict(os.environ, GIT_TERMINAL_PROMPT='0', GCM_INTERACTIVE='never')
        try:
            process = subprocess.Popen(['git', '--no-replace-objects', *arguments], cwd=self.root, env=environment,
                                       stdin=subprocess.PIPE if input_data is not None else subprocess.DEVNULL,
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                       start_new_session=os.name != 'nt')
        except OSError as error:
            raise ReleaseError('cannot start git; check installation and repository access') from error
        try:
            output, _ = process.communicate(input_data, timeout=self.timeout)
        except (subprocess.TimeoutExpired, KeyboardInterrupt) as error:
            cleanup = 'owned process tree stopped'
            try:
                _stop_owned(process)
                process.communicate(timeout=10)
            except (OSError, subprocess.SubprocessError):
                cleanup = 'process cleanup incomplete; inspect running processes'
            reason = 'interrupted' if isinstance(error, KeyboardInterrupt) else 'timed out'
            raise ReleaseError('git ' + arguments[0] + ' ' + reason + '; ' + cleanup +
                               '; inspect local and remote refs before retrying') from error
        if process.returncode not in allowed:
            raise ReleaseError('git ' + arguments[0] + ' failed (exit ' + str(process.returncode) +
                               '); inspect authentication, hooks and repository state')
        return output

    def text(self, *arguments, **options):
        return self.run(*arguments, **options).decode('utf-8').strip()


def go_version(value):
    match = re.fullmatch(r'v[01]\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?', value)
    if match is None or any(part.isdigit() and len(part) > 1 and part.startswith('0')
                            for part in (match.group(3) or '').split('.')):
        raise argparse.ArgumentTypeError('use a canonical v0/v1 SemVer tag without build metadata')
    return value


def read_record(root, name):
    try:
        value = json.loads(safe_file(root, name).read_text(encoding='utf-8'))
        return value if isinstance(value, dict) else {}
    except (OSError, ValueError):
        return {}


def check_worktree(git):
    if git.run('diff', '--cached', '--name-only', '-z'):
        raise ReleaseError('index contains staged changes; commit the approved candidate first')
    for entry in git.run('ls-files', '-v', '-z').split(b'\0'):
        if entry and (entry[:1] == b'S' or entry[:1].islower()):
            relative = pathlib.PurePosixPath(entry[2:].decode('utf-8'))
            if relative.parts[0] not in EXCLUDED:
                raise ReleaseError('SDK index uses skip-worktree or assume-unchanged; use a complete release checkout')
    changed = git.run('diff', '--name-only', '-z')
    changed += git.run('ls-files', '--others', '--exclude-standard', '-z')
    for raw_path in changed.split(b'\0'):
        if raw_path and pathlib.PurePosixPath(raw_path.decode('utf-8')).parts[0] not in EXCLUDED:
            raise ReleaseError('uncommitted SDK files; use a committed, frozen release checkout')


def check_source_bytes(git, commit):
    entries = {}
    for entry in git.run('ls-tree', '-rz', '--full-tree', commit).split(b'\0'):
        if entry:
            metadata, raw_path = entry.split(b'\t', 1)
            entries[raw_path.decode('utf-8')] = metadata.split()
    for relative in entries:
        if any(part in EXCLUDED for part in pathlib.PurePosixPath(relative).parts):
            raise ReleaseError('excluded path committed: ' + relative)
    files = list(source_files(git.root))
    present = {path.relative_to(git.root).as_posix() for path in files}
    missing = sorted(relative for relative in entries
                     if _source_input(pathlib.PurePosixPath(relative)) and relative not in present)
    if missing:
        raise ReleaseError('committed source inputs missing from checkout: ' + missing[0])
    selected = []
    for path in files:
        relative = path.relative_to(git.root).as_posix()
        metadata = entries.get(relative)
        if metadata is None:
            raise ReleaseError('source input not in commit: ' + relative)
        mode, kind, object_id = metadata
        if mode not in (b'100644', b'100755') or kind != b'blob':
            raise ReleaseError('source input is not a regular committed file: ' + relative)
        selected.append((path, relative, object_id))
    batch = git.run('cat-file', '--batch', input_data=b''.join(object_id + b'\n'
                                                            for _, _, object_id in selected))
    offset = 0
    for path, relative, object_id in selected:
        boundary = batch.find(b'\n', offset)
        header = batch[offset:boundary].split()
        if boundary < 0 or len(header) != 3 or header[:2] != [object_id, b'blob']:
            raise ReleaseError('invalid committed source response')
        size = int(header[2])
        start = boundary + 1
        end = start + size
        if size < 0 or batch[end:end + 1] != b'\n':
            raise ReleaseError('incomplete committed source response')
        if path.read_bytes() != batch[start:end]:
            raise ReleaseError('source bytes differ from commit: ' + relative)
        offset = end + 1
    if offset != len(batch):
        raise ReleaseError('unexpected committed source response')


def check_transport(git, destination=None):
    if git.run('config', '--name-only', '--get-regexp', r'^url\..*\.pushinsteadof$', allowed=(0, 1)):
        raise ReleaseError('pushInsteadOf URL rewrites are refused; queries and push must use the same destination')
    if destination is not None and destination in git.text('remote').splitlines():
        raise ReleaseError('push destination must not resolve as a configured remote name; use an unambiguous endpoint')


def check_candidate(git, commit, expected_hash=None):
    check_transport(git)
    if git.text('rev-parse', '--verify', 'HEAD^{commit}') != commit:
        raise ReleaseError('candidate commit must equal HEAD; use a dedicated release checkout')
    check_worktree(git)
    module = re.search(r'^module\s+(\S+)', (git.root / 'go.mod').read_text(encoding='utf-8'), re.M)
    if module is None or module.group(1) != MODULE:
        raise ReleaseError('module path must remain ' + MODULE)
    digest = source_hash(git.root)
    if expected_hash is not None and digest != expected_hash:
        raise ReleaseError('source changed after preflight; rebuild verification evidence')
    check_source_bytes(git, commit)
    reports = {name: read_record(git.root, '.artifacts/' + alias + '-report.json')
               for name, alias in FORMAL_REPORT_NAMES.items()}
    problems = release_problems(git.root, read_record(git.root, 'task-status.json'),
                                read_record(git.root, 'acceptance-results.json'), reports, digest)
    if problems:
        details = '\n'.join('- ' + problem for problem in problems[:10])
        if len(problems) > 10:
            details += '\n- ' + str(len(problems) - 10) + ' more refusals; run scripts/verify-release.py'
        raise ReleaseError('RELEASE GATE: FAIL\n' + details)
    if source_hash(git.root) != digest or git.text('rev-parse', '--verify', 'HEAD^{commit}') != commit:
        raise ReleaseError('candidate changed during verification; freeze and recheck it')
    check_worktree(git)
    return digest


def remote_tags(git, destination, tag_ref):
    check_transport(git, destination)
    output = git.text('ls-remote', destination, tag_ref, tag_ref + '^{}')
    result = {}
    for line in output.splitlines():
        object_id, reference = line.split('\t', 1)
        if reference not in (tag_ref, tag_ref + '^{}') or reference in result:
            raise ReleaseError('unexpected remote tag response')
        if re.fullmatch(r'[a-f0-9]{40}|[a-f0-9]{64}', object_id) is None:
            raise ReleaseError('invalid remote tag object')
        result[reference] = object_id
    return result


def build_plan(root, version, commit, remote, timeout):
    git = Git(pathlib.Path(root).resolve(), timeout)
    if pathlib.Path(git.text('rev-parse', '--show-toplevel')).resolve() != git.root:
        raise ReleaseError('release must use the SDK repository root')
    if re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]*', remote) is None:
        raise ReleaseError('remote must be a configured remote name, not a URL')
    candidate = commit or git.text('rev-parse', '--verify', 'HEAD^{commit}')
    if re.fullmatch(r'[a-f0-9]{40}|[a-f0-9]{64}', candidate) is None:
        raise ReleaseError('--commit must be a full lowercase commit SHA')
    digest = check_candidate(git, candidate)
    tag_ref = 'refs/tags/' + version
    if git.text('rev-parse', '--verify', '--quiet', tag_ref, allowed=(0, 1)):
        raise ReleaseError('local tag already exists; never overwrite or automatically delete it')
    destinations = git.run('remote', 'get-url', '--push', '--all', remote).decode('utf-8').splitlines()
    if (len(destinations) != 1 or not destinations[0] or destinations[0].startswith('-')
            or destinations[0] != destinations[0].strip()):
        raise ReleaseError('configured remote must have exactly one push destination')
    destination = destinations[0]
    if remote_tags(git, destination, tag_ref):
        raise ReleaseError('remote tag already exists; choose a new version after inspecting it')
    check_candidate(git, candidate, digest)
    return git, dict(version=version, commit=candidate, remote=remote, destination=destination,
                     tag_ref=tag_ref, source_sha256=digest)


def main(argv=None, *, root=ROOT):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True, type=go_version, help='new v0/v1 SemVer tag')
    parser.add_argument('--commit', help='full SHA of HEAD; required with --publish')
    parser.add_argument('--remote', default='origin', help='configured remote name (default: origin)')
    parser.add_argument('--publish', action='store_true', help='authorize annotated tag creation and one tag push')
    parser.add_argument('--timeout', type=int, default=300, help='seconds per Git operation (default: 300)')
    args = parser.parse_args(argv)
    if args.publish and not args.commit:
        parser.error('--publish requires an explicit --commit full SHA')
    if not 1 <= args.timeout <= 3600:
        parser.error('--timeout must be between 1 and 3600 seconds')
    writing = False
    try:
        git, plan = build_plan(root, args.version, args.commit, args.remote, args.timeout)
        print('Version: ' + plan['version'])
        print('Commit: ' + plan['commit'])
        print('Source v2: ' + plan['source_sha256'])
        print('Remote: ' + plan['remote'] + ' (one push destination verified)')
        print('RELEASE GATE: PASS (AC-072 business migration remains a separate gate)')
        if not args.publish:
            print('DRY-RUN: PASS; no tag or push performed')
            return 0
        check_candidate(git, plan['commit'], plan['source_sha256'])
        check_transport(git, plan['destination'])
        writing = True
        git.run('tag', '-a', plan['version'], plan['commit'], '-m', 'Release ' + plan['version'])
        tag_object = git.text('rev-parse', '--verify', plan['tag_ref'])
        if git.text('cat-file', '-t', tag_object) != 'tag' or git.text('rev-parse', plan['tag_ref'] + '^{}') != plan['commit']:
            raise ReleaseError('local annotated tag does not match the checked commit')
        check_candidate(git, plan['commit'], plan['source_sha256'])
        if git.text('rev-parse', '--verify', plan['tag_ref']) != tag_object:
            raise ReleaseError('local tag changed before push; preserve refs and inspect concurrent operations')
        check_transport(git, plan['destination'])
        git.run('push', '--porcelain', '--no-follow-tags', '--recurse-submodules=no',
                plan['destination'], tag_object + ':' + plan['tag_ref'])
        observed = remote_tags(git, plan['destination'], plan['tag_ref'])
        if observed != {plan['tag_ref']: tag_object, plan['tag_ref'] + '^{}': plan['commit']}:
            raise ReleaseError('remote annotated tag or peeled commit mismatch; publication is unverified')
        check_candidate(git, plan['commit'], plan['source_sha256'])
        if git.text('rev-parse', '--verify', plan['tag_ref']) != tag_object:
            raise ReleaseError('local tag changed during publication; remote tag was verified but local state differs')
        print('RELEASE: PUBLISHED; remote annotated tag and commit verified')
        print('Go proxy availability and real downstream migration are not inferred.')
        return 0
    except (ReleaseError, OSError, ValueError, KeyError, TypeError, AttributeError, KeyboardInterrupt) as error:
        print('RELEASE: ' + ('PARTIAL' if writing else 'BLOCKED'))
        if isinstance(error, KeyboardInterrupt):
            print('interrupted; inspect local and remote refs before retrying')
        else:
            print(str(error) if isinstance(error, ReleaseError) else 'invalid or unreadable candidate/evidence; recheck the frozen checkout')
        if writing:
            print('Preserve existing refs. Inspect the local and remote tag before any retry; no rollback was attempted.')
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
