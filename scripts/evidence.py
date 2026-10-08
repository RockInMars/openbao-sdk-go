"""Read-only, fail-closed validation of report provenance and actual log targets."""
import hashlib
import json
import pathlib
import re

from tooling import SOURCE_HASH_VERSION, coverage_result, parse_coverage

FORMAL_CLASSES = {'normal':'NORMAL_OFFICIAL_CLIENT','integration':'REAL_OPENBAO',
                  'consumers':'INDEPENDENT_CONSUMERS','scans':'PINNED_SECURITY_SCANNERS',
                  'minimum-normal-linux':'MINIMUM_GO_COMPATIBILITY',
                  'minimum-normal-windows':'MINIMUM_GO_COMPATIBILITY',
                  'minimum-consumer-linux':'MINIMUM_GO_CONSUMERS',
                  'minimum-consumer-windows':'MINIMUM_GO_CONSUMERS'}
FORMAL_REPORT_NAMES = {'normal':'normal','integration':'integration','consumers':'consumer','scans':'security',
                      **{name:name for name in FORMAL_CLASSES if name.startswith('minimum-')}}
MINIMUM_GO_CLASSES = {'MINIMUM_GO_COMPATIBILITY','MINIMUM_GO_CONSUMERS'}
FUZZ_TARGETS = {'FuzzPath':'./internal/engine','FuzzDocument':'./kv','FuzzPKICSR':'./pki',
                'FuzzEncoding':'./internal/transitutil','FuzzDecode':'./internal/pemutil'}
INTEGRATION_TESTS = ['TestIntegrationRuntime'] + ['TestIntegrationRuntime/'+x for x in
    ('TestIntegrationKV','TestIntegrationKVScheduledDeletion','TestIntegrationPKI','TestIntegrationTransit','TestIntegrationAuthNamespace')]


def safe_file(root, name):
    if not isinstance(name,str) or not name or '\\' in name or ':' in name:
        raise ValueError('invalid evidence path')
    rel=pathlib.PurePosixPath(name)
    if rel.is_absolute() or '..' in rel.parts:
        raise ValueError('evidence path escapes repository')
    p=root
    for part in rel.parts:
        p=p/part
        if p.is_symlink():raise ValueError('evidence symlink rejected')
    if not p.resolve().is_relative_to(root.resolve()) or not p.is_file():
        raise ValueError('evidence missing or outside repository: '+name)
    return p


def read_json(path):
    def unique(pairs):
        result={}
        for key,value in pairs:
            if key in result:raise ValueError('duplicate JSON key')
            result[key]=value
        return result
    return json.loads(path.read_text(encoding='utf-8'),object_pairs_hook=unique)


def identity_matches(record, current_hash):
    return isinstance(record,dict) and record.get('source_hash_version')==SOURCE_HASH_VERSION and record.get('source_sha256')==current_hash


def receipt_log(root, receipt, *, allow_export=False):
    if not isinstance(receipt,dict) or receipt.get('completion')!='exited' or type(receipt.get('exit_code')) is not int or receipt['exit_code']!=0 or receipt.get('cleanup_incomplete'):
        raise ValueError('command did not naturally succeed')
    cmd=receipt.get('command')
    if not isinstance(cmd,list) or not cmd or not all(isinstance(x,str) and x for x in cmd):
        raise ValueError('missing command')
    export=allow_export and receipt.get('export_kind') in {'go-events-v1','fuzz-progress-v1','unittest-summary-v1','command-status-v1','tool-version-v1','scanner-source-v1'}
    p=safe_file(root,receipt.get('exported_log') if export else receipt.get('log'))
    raw=p.read_bytes()
    if hashlib.sha256(raw).hexdigest()!=receipt.get('exported_log_sha256' if export else 'log_sha256'):
        raise ValueError('log digest mismatch')
    return raw.decode('utf-8')


def no_test_packages(events, *, allow_export=False):
    empty=set()
    for event in events:
        package=event.get('Package')
        if 'Test' in event or not isinstance(package,str):continue
        if event.get('Action')=='output' and re.fullmatch(r'\?\s+'+re.escape(package)+r'\s+\[no test files\]\s*',event.get('Output','')):
            empty.add(package)
        if allow_export and event.get('Action')=='skip' and event.get('NoTestFiles') is True:empty.add(package)
    return empty-{e.get('Package') for e in events if 'Test' in e}


def go_results(raw, *, allow_export=False):
    started=set();passed=set();packages=set()
    events=[json.loads(line) for line in raw.splitlines() if line.startswith('{')]
    empty=no_test_packages(events,allow_export=allow_export)
    for event in events:
        action=event.get('Action');package=event.get('Package');test=event.get('Test')
        if 'Test' in event and (not isinstance(test,str) or not test):raise ValueError('malformed Go Test field')
        if action=='fail' or (action=='skip' and (test or package not in empty)):
            raise ValueError('failed or skipped Go target')
        if test:
            if not isinstance(package,str) or not package:raise ValueError('Go event lacks package')
            if action=='run':started.add((package,test))
            if action=='pass':passed.add((package,test))
        elif action=='pass':packages.add(package)
    if not passed or passed!=started or not {p for p,_ in passed}.issubset(packages):
        raise ValueError('zero or incomplete Go tests')
    return passed


def fuzz_executions(raw):
    counts=[int(n) for n in re.findall(r'fuzz: elapsed: [^\n]*execs: (\d+)',raw)]
    if not counts or max(counts)<=0 or not re.search(r'^PASS\s*$',raw,re.M) or re.search(r'^FAIL\b',raw,re.M):
        raise ValueError('missing successful timed fuzz executions')
    return max(counts)


def recorded_path(value):
    if not isinstance(value,str) or not value:raise ValueError('working directory missing')
    p=pathlib.PureWindowsPath(value) if re.match(r'^[A-Za-z]:',value) else pathlib.PurePosixPath(value)
    if not p.is_absolute() or '..' in p.parts:raise ValueError('working directory must be absolute')
    return p


def require_command(receipt, command, cwd):
    if receipt.get('command')!=command or recorded_path(receipt.get('cwd'))!=recorded_path(str(cwd)):
        raise ValueError('command argv or working directory mismatch: '+str(receipt.get('name')))


def _report_envelope(root, report, current_hash, *, allow_export):
    """Check shared provenance before interpreting any evidence category."""
    if not identity_matches(report,current_hash) or report.get('status')!='PASS' or not report.get('run_id'):
        raise ValueError('report unsuccessful, missing version or stale')
    identity={'source_hash_version':SOURCE_HASH_VERSION,'source_sha256':current_hash}
    if report.get('source_before')!=identity or report.get('source_after')!=identity:
        raise ValueError('baseline changed or not observed before and after')
    if 'baseline_changed' in report.get('problems',[]):raise ValueError('observed baseline change is latched')
    commands=report.get('commands',[])
    if not isinstance(commands,list):raise ValueError('commands must be a list')
    logs={};receipts={}
    for command in commands:
        name=command.get('name')
        if not isinstance(name,str) or name in logs:raise ValueError('missing or duplicate command name')
        logs[name]=receipt_log(root,command,allow_export=allow_export)
        receipts[name]=command
    workspace=recorded_path(report.get('working_directory')) if report.get('evidence_class')!='STATIC_REVIEW' else None
    return logs,receipts,workspace


def _require_go_version(root, report, workspace, *, allow_export):
    probes=report.get('environment_commands',[])
    if len(probes)!=1 or probes[0].get('name')!='go-version':raise ValueError('Go version receipt missing')
    require_command(probes[0],['go','version'],workspace)
    version=receipt_log(root,probes[0],allow_export=allow_export).strip()
    expected='1.25.0' if report.get('evidence_class') in MINIMUM_GO_CLASSES else '1.26.8'
    match=re.fullmatch(r'go version go'+re.escape(expected)+r' (linux|windows)/[A-Za-z0-9_]+',version)
    if not match or report.get('tool_versions',{}).get('go')!=version:
        raise ValueError('verification requires recorded Go '+expected+'; toolchain differs or is unverified')
    if report.get('platform')!=match[1]:
        raise ValueError('recorded platform differs from Go version receipt')


def _normal_tests(root, report, logs, receipts, workspace, *, allow_export):
    if set(logs)!={'unit','race-coverage','vet','modules','build','module-graph'}:
        raise ValueError('normal command set incomplete')
    tests=go_results(logs['unit'],allow_export=allow_export)
    if not any(n=='TestDependencyBaseline' for _,n in tests):raise ValueError('official dependency baseline not executed')
    profile=report.get('coverage_profile',{})
    raw=safe_file(root,profile.get('path')).read_bytes()
    # These approved expectations are deliberately independent of runner output.
    expected={'unit':['go','test','-mod=readonly','-json','-count=1','-timeout=120s','./...'],
        'race-coverage':['go','test','-mod=readonly','-race','-count=1','-timeout=180s','-coverprofile='+str(workspace/profile['path']),'./...'],
        'vet':['go','vet','-mod=readonly','./...'], 'modules':['go','mod','verify'],
        'build':['go','build','-mod=readonly','./...'], 'module-graph':['go','list','-mod=readonly','-m','-json','all']}
    for name,cmd in expected.items():require_command(receipts[name],cmd,workspace)
    if profile.get('run_id')!=report['run_id'] or hashlib.sha256(raw).hexdigest()!=profile.get('sha256'):
        raise ValueError('coverage profile not bound to this run')
    module=re.search(r'^module\s+(\S+)',(root/'go.mod').read_text(encoding='utf-8'),re.M)[1]
    if not coverage_result(parse_coverage(raw.decode('utf-8')),module)['passed']:
        raise ValueError('critical coverage below threshold')
    return tests


def _integration_tests(root, report, logs, receipts, workspace, *, allow_export):
    if set(logs)!={'integration'}:raise ValueError('integration command missing')
    require_command(receipts['integration'],['go','test','-mod=readonly','-json','-race','-tags=integration','-timeout=5m','-count=1','./tests/integration'],workspace)
    tests=go_results(logs['integration'],allow_export=allow_export)
    if not set(INTEGRATION_TESTS).issubset({n for _,n in tests}):raise ValueError('integration targets missing')
    baseline=report.get('baseline',{})
    locked=read_json(safe_file(root,'deploy/test/server-lock.json'))
    if locked.get('status')!='PINNED' or any(locked.get(k)!=baseline.get(k) for k in ('version','image','binary_sha256')):
        raise ValueError('integration report does not match frozen server lock')
    if not re.fullmatch(r'2\.\d+\.\d+',baseline.get('version','')):raise ValueError('server version missing')
    if baseline.get('mode')=='docker':
        if not re.fullmatch(r'.+@sha256:[a-f0-9]{64}',baseline.get('image','')):raise ValueError('server digest missing')
    elif baseline.get('mode')!='binary' or not re.fullmatch(r'[a-f0-9]{64}',baseline.get('binary_sha256','')):
        raise ValueError('server binary digest missing')
    return tests


def _consumer_tests(root, report, logs, workspace, *, allow_export):
    if set(logs)!={'reader-tidy','reader-test','signer-tidy','signer-test'}:raise ValueError('consumer command set incomplete')
    consumers=report.get('consumers',[])
    if len(consumers)!=2 or {x.get('name') for x in consumers}!={'reader','signer'}:raise ValueError('consumer set incomplete')
    tests=set();names=set()
    for item in consumers:
        if item.get('status')!='PASS' or len(item.get('commands',[]))!=2:raise ValueError('consumer incomplete')
        receipts=item['commands']
        directory=recorded_path(item.get('directory'))
        if directory.name!=item['name'] or directory.is_relative_to(workspace):raise ValueError('consumer directory is not isolated')
        require_command(receipts[0],['go','mod','tidy'],directory)
        require_command(receipts[1],['go','test','-mod=readonly','-json','-race','-count=1','-timeout=180s','./...'],directory)
        if any(receipt not in report['commands'] for receipt in receipts):raise ValueError('consumer receipt mismatch')
        receipt_log(root,receipts[0],allow_export=allow_export)
        result=go_results(receipt_log(root,receipts[1],allow_export=allow_export),allow_export=allow_export)
        expected='TestIndependent'+item['name'].title()
        if not any(n==expected for _,n in result):raise ValueError('consumer target missing')
        names.add(item['name']);tests.update(result)
    return tests,names


def _scanner_results(root, report, logs, receipts, workspace, *, allow_export=False):
    pins={'govulncheck':'golang.org/x/vuln/cmd/govulncheck@v1.1.4','gitleaks':'github.com/zricethezav/gitleaks/v8@v8.24.3'}
    if set(logs)!=set(pins):raise ValueError('scanner set incomplete')
    require_command(receipts['govulncheck'],['go','run',pins['govulncheck'],'-db=https://vuln.go.dev','-show=version','./...'],workspace)
    if (not re.search(r'^Scanner: govulncheck@v1\.1\.4\s*$',logs['govulncheck'],re.M)
            or not re.search(r'^DB: https://vuln\.go\.dev\s*$',logs['govulncheck'],re.M)
            or not re.search(r'^DB updated: \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)? \+0000 UTC\s*$',logs['govulncheck'],re.M)):
        raise ValueError('scanner version or official database receipt missing')
    require_command(receipts['gitleaks'],['go','run',pins['gitleaks'],'dir','--redact=100','--no-banner','--config',str(workspace/'deploy/test/gitleaks.toml'),'--report-format=json','--report-path',str(workspace/report['findings']),'.'],workspace)
    raw=safe_file(root,report.get('findings')).read_bytes()
    original_digest=report.get('findings_sha256')
    if not isinstance(original_digest,str) or not re.fullmatch(r'[a-f0-9]{64}',original_digest):
        raise ValueError('findings digest missing or invalid')
    if allow_export and report.get('ci_export')=='structured-v1':
        metadata=report.get('findings_export')
        if (not isinstance(metadata,dict) or metadata.get('kind')!='count-only-v1'
                or metadata.get('raw_sha256')!=original_digest
                or metadata.get('sha256')!=hashlib.sha256(raw).hexdigest()):
            raise ValueError('exported findings digest mismatch')
    elif original_digest!=hashlib.sha256(raw).hexdigest():
        raise ValueError('raw findings digest mismatch')
    if json.loads(raw)!=[]:raise ValueError('secret scan findings unresolved')


def _fuzz_targets(report, logs, workspace):
    if set(logs)!=set(FUZZ_TARGETS):raise ValueError('fuzz target set incomplete')
    targets=set()
    for item in report['commands']:
        name=item['name']
        require_command(item,['go','test','-mod=readonly','-run=^$','-fuzz='+name,'-fuzztime=30s','-parallel=1','-timeout=90s',FUZZ_TARGETS[name]],workspace)
        fuzz_executions(logs[name]);targets.add(name)
    return targets


def _tooling_results(logs, receipts, workspace):
    if set(logs)!={'tooling'}:raise ValueError('tooling command missing')
    argv=receipts['tooling']['command']
    executable=re.split(r'[/\\]',argv[0])[-1]
    if not re.fullmatch(r'python(?:3(?:\.\d+)?)?(?:\.exe)?',executable,re.I):raise ValueError('tooling interpreter mismatch')
    require_command(receipts['tooling'],[argv[0],'-B','-m','unittest','discover','-s','scripts/tests','-v'],workspace)
    raw=logs['tooling']
    if not re.search(r'Ran [1-9]\d* tests? in ',raw) or not re.search(r'^OK\s*$',raw,re.M) or re.search(r'\b(?:FAILED|skipped=|ERROR:|FAIL:)\b',raw):
        raise ValueError('tooling targets failed, skipped or absent')


def _static_documents(root, report):
    documents=set()
    for item in report.get('documents',[]):
        p=safe_file(root,item.get('path'))
        if hashlib.sha256(p.read_bytes()).hexdigest()!=item.get('sha256'):raise ValueError('static document changed')
        documents.add(item['path'])
    if not documents or not report.get('reviewed_at') or not report.get('rationale'):
        raise ValueError('static review lacks rationale or documents')
    return documents


def report_results(root, report, current_hash, *, allow_export=False):
    """Return independently parsed selectors, or raise for any invalid report."""
    logs,receipts,workspace=_report_envelope(root,report,current_hash,allow_export=allow_export)
    kind=report.get('evidence_class')
    if kind in {'NORMAL_OFFICIAL_CLIENT','REAL_OPENBAO','INDEPENDENT_CONSUMERS','PINNED_SECURITY_SCANNERS','FUZZ_RUNTIME'} | MINIMUM_GO_CLASSES:
        _require_go_version(root,report,workspace,allow_export=allow_export)
    tests=set();names=set(logs);fuzz=set();documents=set()
    if kind in {'NORMAL_OFFICIAL_CLIENT','MINIMUM_GO_COMPATIBILITY'}:
        tests=_normal_tests(root,report,logs,receipts,workspace,allow_export=allow_export)
    elif kind=='REAL_OPENBAO':
        tests=_integration_tests(root,report,logs,receipts,workspace,allow_export=allow_export)
    elif kind in {'INDEPENDENT_CONSUMERS','MINIMUM_GO_CONSUMERS'}:
        tests,consumers=_consumer_tests(root,report,logs,workspace,allow_export=allow_export)
        names.update(consumers)
    elif kind=='PINNED_SECURITY_SCANNERS':
        _scanner_results(root,report,logs,receipts,workspace,allow_export=allow_export)
    elif kind=='FUZZ_RUNTIME':
        fuzz=_fuzz_targets(report,logs,workspace)
    elif kind=='COMMAND_TOOLING':
        _tooling_results(logs,receipts,workspace)
    elif kind=='STATIC_REVIEW':
        documents=_static_documents(root,report)
    else:
        raise ValueError('unsupported evidence class')
    return {'tests':tests,'commands':names,'fuzz_targets':fuzz,'documents':documents}


def release_problems(root,ledger,acceptance,reports,current_hash):
    problems=[];cache={}
    for name,record in [('ledger',ledger),('acceptance',acceptance)]:
        if not identity_matches(record,current_hash):problems.append(name+' missing v2 identity or stale baseline')
    for name,kind in FORMAL_CLASSES.items():
        report=reports.get(name,{})
        try:
            if report.get('evidence_class')!=kind:raise ValueError('wrong evidence class')
            if name.startswith('minimum-') and report.get('platform')!=name.rsplit('-',1)[1]:
                raise ValueError('minimum verification platform mismatch')
            report_results(root,report,current_hash)
        except (ValueError,OSError,KeyError,TypeError,AttributeError) as e:problems.append(name+': '+str(e))
    try:
        rules=read_json(root/'scripts/acceptance-rules.json')
        if rules.get('schema_version')!=1:raise ValueError('unknown acceptance rule schema')
    except (ValueError,OSError,AttributeError):rules={};problems.append('acceptance rules missing or invalid')
    items={x.get('id'):x for x in acceptance.get('results',[])}
    tasks={x.get('id'):x for x in ledger.get('tasks',[])}
    if len(items)!=len(acceptance.get('results',[])) or len(tasks)!=len(ledger.get('tasks',[])):
        problems.append('duplicate OB/AC records')

    def evidence(record):
        references=record.get('evidence')
        if not isinstance(references,list) or not references:raise ValueError('nonempty evidence references required')
        result=[]
        for name in references:
            if name not in cache:
                r=read_json(safe_file(root,name));cache[name]=(r,report_results(root,r,current_hash))
            result.append(cache[name])
        return result

    module=re.search(r'^module\s+(\S+)',(root/'go.mod').read_text(encoding='utf-8'),re.M)[1]
    valid_ac=set()
    for i in range(1,72):
        key=f'AC-{i:03}';item=items.get(key,{})
        try:
            if item.get('status')!='PASS':raise ValueError('not PASS')
            if not identity_matches(item,current_hash):raise ValueError('missing v2 identity or stale baseline')
            rule=rules.get('rules',{}).get(key,{})
            if rule.get('kind') not in ('runtime','static') or not rule.get('required_evidence'):raise ValueError('missing evidence rule')
            if rule.get('blocked_reason'):raise ValueError(rule['blocked_reason'])
            review=item.get('revalidation')
            if review:
                if rule['kind']!='static' or key not in rules.get('static_revalidation_allowlist',[]):raise ValueError('impact review not allowed')
                if review.get('method')!='impact_review' or not identity_matches(review,current_hash) or not all(review.get(k) for k in ['previous_source_sha256','reviewed_at','rationale','evidence']):raise ValueError('incomplete impact review')
            candidates=evidence(item)
            for group in rule['required_evidence']:
                matched=False
                for report,parsed in candidates:
                    if report['evidence_class'] not in group.get('allowed_classes',[]):continue
                    required={(module if t['package']=='.' else module+'/'+t['package'],t['name']) for t in group.get('tests',[])}
                    if not required.issubset(parsed['tests']):continue
                    if any(not set(group.get(k,[])).issubset(parsed[k]) for k in ['commands','fuzz_targets','documents']):continue
                    matched=True;break
                if not matched:raise ValueError('required current evidence target missing')
            valid_ac.add(key)
        except (ValueError,OSError,KeyError,TypeError,AttributeError) as e:problems.append(key+': '+str(e))
    for i in range(1,19):
        key=f'OB-{i:03}';item=tasks.get(key,{})
        try:
            if item.get('status')!='VERIFIED':raise ValueError('not VERIFIED')
            if not identity_matches(item,current_hash):raise ValueError('missing v2 identity or stale baseline')
            if not item.get('acceptance_ids') or not set(item['acceptance_ids']).issubset(valid_ac):raise ValueError('associated AC not verified')
            if any(tasks.get(d,{}).get('status')!='VERIFIED' for d in item.get('depends_on',[])):raise ValueError('task dependency not verified')
            evidence(item)
        except (ValueError,OSError,KeyError,TypeError,AttributeError) as e:problems.append(key+': '+str(e))
    return problems
