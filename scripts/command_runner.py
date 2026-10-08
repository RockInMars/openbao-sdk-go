"""Bounded commands with immutable logs and explicit natural completion."""
from __future__ import annotations

import datetime
import hashlib
import math
import os
import pathlib
import signal
import subprocess
import time

from tooling import ROOT


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def file_sha256(path):
    with pathlib.Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def succeeded(receipt):
    return receipt.get('completion') == 'exited' and receipt.get('exit_code') == 0 and not receipt.get('cleanup_incomplete')


def _stop_owned(process):
    if os.name == 'nt':
        # PID belongs to the process we just created; never kill by executable name.
        # The helper must not inherit credentials from a remote test runner.
        helper_env = {key: value for key, value in os.environ.items()
                      if key.upper() in {'SYSTEMROOT', 'WINDIR', 'PATH', 'PATHEXT'}}
        subprocess.run(['taskkill', '/PID', str(process.pid), '/T', '/F'],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                       timeout=10, check=True, creationflags=subprocess.CREATE_NO_WINDOW,
                       env=helper_env)
    else:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    return process.wait(timeout=10)


def run_command(command, *, cwd, env, log, timeout_s, discard_output=False):
    if not command or not math.isfinite(timeout_s) or timeout_s <= 0:
        raise ValueError('nonempty command and positive deadline required')
    log = pathlib.Path(log)
    log.parent.mkdir(parents=True, exist_ok=True)
    try:
        log_name = log.resolve().relative_to(ROOT).as_posix()
    except ValueError:
        log_name = str(log.resolve())
    receipt = {'command': list(command), 'cwd':str(pathlib.Path(cwd).resolve()), 'log': log_name, 'started_at': utc_now(),
               'timeout_s': timeout_s, 'completion': 'unavailable', 'exit_code': None}
    started = time.monotonic()
    options = {'creationflags': subprocess.CREATE_NEW_PROCESS_GROUP | subprocess.CREATE_NO_WINDOW} if os.name == 'nt' else {'start_new_session': True}
    child_env = dict(env)
    child_env['PYTHONIOENCODING'] = 'utf-8'
    with log.open('xb') as stream:
        if discard_output:
            stream.write(b'COMMAND_OUTPUT_SUPPRESSED: structured evidence only\n')
            receipt['output_suppressed'] = True
        try:
            process = subprocess.Popen(command, cwd=cwd, env=child_env,
                                       stdout=subprocess.DEVNULL if discard_output else stream,
                                       stderr=subprocess.STDOUT, **options)
        except OSError as exc:
            stream.write(('COMMAND_UNAVAILABLE: ' + type(exc).__name__ + '\n').encode('utf-8'))
        else:
            receipt['owned_pid'] = process.pid
            try:
                receipt['exit_code'] = process.wait(timeout=timeout_s)
                receipt['completion'] = 'exited'
            except (subprocess.TimeoutExpired, KeyboardInterrupt) as exc:
                receipt['completion'] = 'timeout' if isinstance(exc, subprocess.TimeoutExpired) else 'interrupted'
                try:
                    receipt['termination_exit_code'] = _stop_owned(process)
                except (OSError, subprocess.SubprocessError) as cleanup:
                    receipt['cleanup_error'] = type(cleanup).__name__
                    receipt['cleanup_incomplete'] = True
                    try:
                        if process.poll() is None: process.kill()
                        process.wait(timeout=10)
                    except (OSError, subprocess.SubprocessError):
                        pass
                stream.write(('\nCOMMAND_' + receipt['completion'].upper() + '\n').encode('utf-8'))
    receipt.update(finished_at=utc_now(), elapsed_s=round(time.monotonic()-started, 3),
                   log_sha256=file_sha256(log))
    return receipt
