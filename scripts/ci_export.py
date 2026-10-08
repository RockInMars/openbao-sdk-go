"""Export structured evidence only; raw command output never enters CI artifacts."""
import hashlib
import json
import re

from evidence import safe_file, INTEGRATION_TESTS, no_test_packages
from tooling import source_files

REPORT_FIELDS = {'source_hash_version','source_sha256','source_before','source_after','evidence_class',
                 'status','started_at','finished_at','platform','python_version','run_id',
                 'working_directory','coverage_profile','baseline','tool_versions','findings_sha256'}
RECEIPT_FIELDS = {'name','command','cwd','completion','exit_code','started_at','finished_at',
                  'timeout_s','elapsed_s','log','log_sha256','owned_pid','termination_exit_code',
                  'cleanup_incomplete'}


def approved_selectors(root):
    packages=set();tests=set()
    module=re.search(r'^module\s+(\S+)',safe_file(root,'go.mod').read_text(encoding='utf-8'),re.M)[1]
    packages.add(module)
    packages.add(module+'/tests/integration')
    tests.update((module+'/tests/integration',name) for name in INTEGRATION_TESTS)
    for path in source_files(root):
        if path.suffix!='.go':continue
        directory=path.parent
        while directory!=root and not (directory/'go.mod').is_file():directory=directory.parent
        mod=re.search(r'^module\s+(\S+)',(directory/'go.mod').read_text(encoding='utf-8'),re.M)[1]
        relative=path.parent.relative_to(directory).as_posix()
        package=mod if relative=='.' else mod+'/'+relative
        packages.add(package)
        for name in re.findall(r'^func\s+((?:Test|Fuzz)\w+|Example\w*)\s*\(',path.read_text(encoding='utf-8'),re.M):tests.add((package,name))
    rules=json.loads(safe_file(root,'scripts/acceptance-rules.json').read_text(encoding='utf-8'))
    for rule in rules['rules'].values():
        for requirement in rule.get('required_evidence',[]):
            for selector in requirement.get('tests',[]):
                relative=selector['package'];package=module if relative=='.' else module+'/'+relative
                packages.add(package);tests.add((package,selector['name']))
    return module,packages,tests


def structured_log(receipt, raw, selectors):
    name=receipt['name']
    if name=='go-version':
        lines=[line for line in raw.splitlines() if re.fullmatch(r'go version go[\w.+-]+ [\w]+/[\w]+',line)]
        return 'tool-version-v1','\n'.join(lines)+'\n'
    if name in {'unit','integration','reader-test','signer-test'}:
        lines=[]
        module,packages,tests=selectors
        events=[json.loads(line) for line in raw.splitlines() if line.startswith('{')]
        empty=no_test_packages(events)
        for event in events:
            if event.get('Action') not in {'run','pass','fail','skip'}:continue
            # Output and arbitrary fields are deliberately discarded, including
            # panic diagnostics, secrets, request bodies and environment dumps.
            package=event.get('Package');name=event.get('Test')
            if package not in packages:
                lines.append(json.dumps({'Action':'fail','Package':module,'Test':'EvidenceUnknownPackage'}))
                continue
            clean={'Action':event['Action'],'Package':package}
            if event['Action']=='skip' and 'Test' not in event and package in empty:clean['NoTestFiles']=True
            if 'Test' in event and (not isinstance(name,str) or not name):
                clean.update(Action='fail',Test='EvidenceInvalidTest')
            if name:
                if (package,name) in tests:clean['Test']=name
                elif isinstance(name,str) and (package,name.split('/')[0]) in tests:clean['Test']=name.split('/')[0]
                else:clean.update(Action='fail',Test='EvidenceUnknownTest')
            lines.append(json.dumps(clean))
        return 'go-events-v1','\n'.join(lines)+'\n'
    if name.startswith('Fuzz'):
        lines=[line for line in raw.splitlines() if re.fullmatch(r'fuzz: elapsed: \d+(?:\.\d+)?s, execs: \d+ \([\d.]+/sec\)(?:, new interesting: \d+ \(total: \d+\))?|PASS|FAIL',line)]
        return 'fuzz-progress-v1','\n'.join(lines)+'\n'
    if name=='tooling':
        lines=[line for line in raw.splitlines() if re.fullmatch(r'Ran \d+ tests? in [\d.]+s|OK(?: \(skipped=\d+\))?|FAILED \([a-z=, \d]+\)',line)]
        return 'unittest-summary-v1','\n'.join(lines)+'\n'
    if name=='govulncheck':
        lines=[line for line in raw.splitlines() if re.fullmatch(
            r'Scanner: govulncheck@v1\.1\.4|DB: https://vuln\.go\.dev|DB updated: \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? \+0000 UTC',line)]
        return 'scanner-source-v1','\n'.join(lines)+'\n'
    # Vet/build/module verification and pinned scanners need their command
    # receipts, not uncontrolled human-readable output. Findings are separate.
    return 'command-status-v1',json.dumps({k:receipt.get(k) for k in ('completion','exit_code')})+'\n'


def build_export(root, report_name, report):
    files={'go.mod':safe_file(root,'go.mod').read_bytes()}
    selectors=approved_selectors(root)
    if report.get('evidence_class')=='REAL_OPENBAO':files['deploy/test/server-lock.json']=safe_file(root,'deploy/test/server-lock.json').read_bytes()
    exported={k:v for k,v in report.items() if k in REPORT_FIELDS}
    exported['ci_export']='structured-v1'
    exported['problems']=[p for p in report.get('problems',[]) if p in {'baseline_changed','cleanup_incomplete'}]
    exported['commands']=[]
    exported['environment_commands']=[]
    receipts={}
    for receipt in report.get('commands',[])+report.get('environment_commands',[]):
        raw=safe_file(root,receipt['log']).read_bytes()
        if hashlib.sha256(raw).hexdigest()!=receipt.get('log_sha256'):raise ValueError('raw log digest mismatch')
        clean={k:v for k,v in receipt.items() if k in RECEIPT_FIELDS}
        kind,text=structured_log(receipt,raw.decode('utf-8'),selectors)
        name=receipt['log']+'.events'
        encoded=text.encode('utf-8')
        clean.update(export_kind=kind,exported_log=name,exported_log_sha256=hashlib.sha256(encoded).hexdigest())
        files[name]=encoded
        receipts[receipt['name']]=clean
        exported['environment_commands' if receipt in report.get('environment_commands',[]) else 'commands'].append(clean)
    if 'consumers' in report:
        exported['consumers']=[{'name':item['name'],'status':item['status'],'directory':item.get('directory'),
            'commands':[receipts[c['name']] for c in item['commands']]} for item in report['consumers']]
    if report.get('coverage_profile'):
        name=report['coverage_profile']['path']
        raw=safe_file(root,name).read_bytes()
        if hashlib.sha256(raw).hexdigest()!=report['coverage_profile']['sha256']:raise ValueError('profile digest mismatch')
        files[name]=raw
    if report.get('findings'):
        name=report['findings']
        if (root/name).is_file():
            raw=safe_file(root,name).read_bytes()
            if hashlib.sha256(raw).hexdigest()!=report.get('findings_sha256'):raise ValueError('raw findings digest mismatch')
            findings=json.loads(raw)
            if not isinstance(findings,list):raise ValueError('invalid findings format')
            # Never export a scanner's Match, Secret, source Line or environment.
            files[name]=(json.dumps([{'finding':True} for _ in findings])+'\n').encode('utf-8')
            exported.update(findings=name,findings_export={'kind':'count-only-v1','raw_sha256':hashlib.sha256(raw).hexdigest(),'sha256':hashlib.sha256(files[name]).hexdigest()})
    files[report_name]=(json.dumps(exported,ensure_ascii=False,indent=2)+'\n').encode('utf-8')
    return files
