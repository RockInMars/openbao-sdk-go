"""Shared immutable run artifacts; only the well-known latest copy is replaced."""
import json
import os
import pathlib
import platform
import re
import uuid

from command_runner import run_command, succeeded, utc_now
from tooling import ROOT, start_report, finish_report


def go_environment(*,allow_network=False):
    env=os.environ.copy()
    env.update(GOWORK='off',GOFLAGS='',GOTOOLCHAIN='local',GOENV='off')
    if not allow_network:env['GOPROXY']='off'
    return env


class RunStopped(Exception):
    """An already-recorded failure forbids further commands in this run."""
    def __init__(self, exit_code):
        super().__init__('run stopped; inspect its saved report')
        self.exit_code=exit_code


class RunRecord:
    """Tool-independent receipts and immutable report persistence."""
    def __init__(self, kind, *, root=None, run_id=None):
        self.root=pathlib.Path(ROOT if root is None else root).resolve()
        self.report=start_report(kind,self.root)
        self.report.update(platform=platform.system().lower(),python_version=platform.python_version(),
                           run_id=run_id or utc_now().replace(':','').replace('+0000','Z')+'-'+uuid.uuid4().hex,
                           tool_versions={'python':platform.python_version()},commands=[])
        self.failure=None

    def stop(self, status, reason):
        self.failure=(status,reason)
        self.report.update(status=status,reason=reason)

    def ensure_active(self):
        if self.failure is not None:
            raise RunStopped(2 if self.report['status']=='BLOCKED' else 1)

    def command(self, name, command, timeout_s, *, log, cwd=None, env=None, environment=False, discard_output=False):
        self.ensure_active()
        options={'discard_output':True} if discard_output else {}
        receipt=run_command(command,cwd=self.root if cwd is None else cwd,
                            env=os.environ.copy() if env is None else env,log=log,timeout_s=timeout_s,**options)
        receipt['name']=name
        self.report.setdefault('environment_commands' if environment else 'commands',[]).append(receipt)
        if receipt.get('cleanup_incomplete'):
            self.stop('FAIL','cleanup_incomplete; inspect owned_pid before starting further checks')
        return receipt

    def finalize(self, status=None, reason=None):
        if status is not None:self.report['status']=status
        if reason:self.report['reason']=reason
        if self.failure is not None:
            self.report['status'],self.report['reason']=self.failure
        return finish_report(self.report,self.root)

    def write(self, target, *, latest=None):
        self.finalize()
        raw=json.dumps(self.report,ensure_ascii=False,indent=2)+'\n'
        with pathlib.Path(target).open('x',encoding='utf-8',newline='\n') as stream:
            stream.write(raw)
        if latest is not None:
            latest=pathlib.Path(latest)
            latest.parent.mkdir(parents=True,exist_ok=True)
            temporary=latest.with_suffix('.'+uuid.uuid4().hex+'.tmp')
            temporary.write_text(raw,encoding='utf-8',newline='\n')
            os.replace(temporary,latest)


class Run:
    """Versioned runner reports; construction never launches a tool."""
    def __init__(self,name,kind,*,root=None,run_id=None):
        self.name=name
        self.record=RunRecord(kind,root=root,run_id=run_id)
        self.root=self.record.root
        self.report=self.record.report
        self.directory=self.root/'.artifacts/runs'/self.report['run_id']/name
        self.directory.mkdir(parents=True,exist_ok=False)
        self.sequence=0

    def probe_go(self, *, expected_version=None):
        version=self.record.command('go-version',['go','version'],30,
                                    log=self.directory/'go-version.log',env=go_environment(),environment=True)
        raw=(self.directory/'go-version.log').read_text(encoding='utf-8')
        recognized=succeeded(version) and re.fullmatch(r'go version go[\w.+-]+ [\w]+/[\w]+\s*',raw)
        self.report['tool_versions']['go']=raw.strip() if recognized else 'unavailable'
        if version.get('cleanup_incomplete'):
            self.record.stop('FAIL','cleanup_incomplete in tool version probe')
            raise RunStopped(self.finish())
        if not recognized:
            status='BLOCKED' if version['completion']=='unavailable' else 'FAIL'
            self.record.stop(status,'Go version probe did not naturally succeed with recognized output')
            raise RunStopped(self.finish())
        if expected_version is not None and not re.fullmatch(r'go version go'+re.escape(expected_version)+r' [\w]+/[\w]+\s*',raw):
            self.record.stop('BLOCKED','this profile requires Go '+expected_version+'; prepare that toolchain explicitly')
            raise RunStopped(self.finish())

    def command(self,name,command,timeout_s,*,cwd=None,env=None):
        receipt=self.record.command(name,command,timeout_s,cwd=cwd,
                                    log=self.directory/(name+'.log'),env=go_environment() if env is None else env)
        self.save()
        print(name+': '+receipt['completion']+' '+str(receipt['exit_code']),flush=True)
        self.record.ensure_active()
        return receipt

    def save(self):
        self.sequence+=1
        target=self.directory/f'report-{self.sequence:03}.json'
        self.report['report_path']=target.relative_to(self.root).as_posix()
        self.record.write(target,latest=self.root/'.artifacts'/(self.name+'-report.json'))

    def finish(self,status=None,reason=None):
        self.record.finalize(status,reason)
        self.save()
        print(self.report['evidence_class']+': '+self.report['status'],flush=True)
        return 0 if self.report['status']=='PASS' else (2 if self.report['status']=='BLOCKED' else 1)


def run_cli(main):
    """Translate recorded stop conditions; unexpected exceptions remain visible."""
    try:
        return main()
    except RunStopped as stopped:
        return stopped.exit_code


def command_status(commands):
    if not commands:return 'FAIL'
    if any(x.get('completion')=='unavailable' for x in commands):return 'BLOCKED'
    return 'PASS' if all(succeeded(x) for x in commands) else 'FAIL'
