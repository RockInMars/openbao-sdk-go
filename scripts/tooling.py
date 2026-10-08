"""Small stdlib-only validation helpers. Not a substitute for Go or security scanners."""
from __future__ import annotations
import datetime, hashlib, json, os, pathlib, re, zipfile

ROOT=pathlib.Path(__file__).resolve().parents[1]
EXCLUDED={'.git','.artifacts','__pycache__','.tools','.pytest_cache','.codex','.serena','.code-review-graph'}
SOURCE_HASH_VERSION=2
SOURCE_DIRS={'auth','baoerr','diagnostics','examples','internal','kv','observe','pki','sensitive','tests','transit'}

def _source_input(rel):
    if any(part in EXCLUDED for part in rel.parts):return False
    if len(rel.parts)==1:return rel.suffix=='.go' or rel.name in ('go.mod','go.sum','Makefile')
    if rel.parts[0] in SOURCE_DIRS:
        return rel.suffix=='.go' or rel.name in ('go.mod','go.sum') or 'testdata' in rel.parts
    if rel.parts[0]=='scripts':
        return rel.suffix=='.py' or rel.as_posix()=='scripts/acceptance-rules.json' or rel.parts[:3]==('scripts','tests','fixtures')
    if rel.parts[:2]==('.github','workflows'):return rel.suffix in ('.yaml','.yml')
    if rel.parts[:2]==('deploy','test'):return rel.suffix in ('.hcl','.yaml','.yml','.toml') or rel.name=='server-lock.json'
    return False

def source_files(root: pathlib.Path):
    selected=[]
    for base,dirs,files in os.walk(root,followlinks=False):
        directory=pathlib.Path(base)
        for name in list(dirs):
            p=directory/name;rel=p.relative_to(root)
            if name in EXCLUDED:dirs.remove(name)
            elif p.is_symlink():
                if rel.parts[0] in SOURCE_DIRS|{'scripts','.github','deploy'}:
                    raise ValueError('source symlink rejected: '+rel.as_posix())
                dirs.remove(name)
        for name in files:
            p=directory/name;rel=p.relative_to(root)
            if not _source_input(rel):continue
            if p.is_symlink():raise ValueError('source symlink rejected: '+rel.as_posix())
            if p.is_file():selected.append(p)
    yield from sorted(selected,key=lambda p:p.relative_to(root).as_posix())

def source_hash(root=ROOT):
    root=pathlib.Path(root)
    h=hashlib.sha256(b'openbao-sdk-go/source-hash/v2\0')
    for p in source_files(root):
        h.update(p.relative_to(root).as_posix().encode('utf-8'));h.update(b'\0');h.update(p.read_bytes());h.update(b'\0')
    return h.hexdigest()

def source_identity(root=ROOT):
    return {'source_hash_version':SOURCE_HASH_VERSION,'source_sha256':source_hash(root)}

def start_report(kind,root=ROOT):
    identity=source_identity(root)
    return {**identity,'source_before':identity.copy(),'evidence_class':kind,'status':'NOT_RUN',
            'working_directory':str(pathlib.Path(root).resolve()),
            'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat()}

def finish_report(report,root=ROOT):
    report['source_after']=source_identity(root)
    report['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat()
    if report.get('source_before')!=report['source_after']:
        if 'baseline_changed' not in report.setdefault('problems',[]):report['problems'].append('baseline_changed')
    if 'baseline_changed' in report.get('problems',[]):report['status']='FAIL'
    return report

def integration_lock(env):
    if env.get('BAO_TEST_ADDRESS') or env.get('BAO_TEST_TOKEN'):
        raise ValueError('External OpenBao servers/credentials are not allowed in this fixture')
    version=env.get('BAO_TEST_VERSION','');binary=env.get('BAO_TEST_BINARY','');image=env.get('BAO_TEST_IMAGE','');digest=env.get('BAO_TEST_BINARY_SHA256','')
    if not re.fullmatch(r'2\.[0-9]+\.[0-9]+',version) or bool(binary)==bool(image):
        raise ValueError('Set exact BAO_TEST_VERSION and ONE verified binary or digest-pinned image')
    if image:
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[a-f0-9]{64}',image) or digest:
            raise ValueError('BAO_TEST_IMAGE must include a verified sha256 digest; no floating tag')
    else:
        p=pathlib.Path(binary)
        if not p.is_absolute() or not p.is_file() or not os.access(p,os.X_OK) or not re.fullmatch(r'[a-f0-9]{64}',digest):
            raise ValueError('Absolute executable BAO_TEST_BINARY and exact SHA256 required')
        h=hashlib.sha256()
        with p.open('rb') as f:
            for block in iter(lambda:f.read(1024*1024),b''):h.update(block)
        if h.hexdigest()!=digest: raise ValueError('OpenBao binary checksum mismatch')
    baseline={'version':version,'image':image or None,'binary_sha256':digest or None,'mode':'binary' if binary else 'docker'}
    try:
        locked=json.loads((ROOT/'deploy/test/server-lock.json').read_text(encoding='utf-8'))
    except (OSError,ValueError):raise ValueError('server lock is missing or invalid') from None
    if not isinstance(locked,dict) or locked.get('status')!='PINNED' or any(locked.get(k)!=baseline[k] for k in ('version','image','binary_sha256')):
        raise ValueError('server lock does not match the explicitly pinned input')
    return baseline

def go_test_passes(events,required):
    failed=any(x.get('Action') in ('fail','skip') for x in events)
    passed={x.get('Test') for x in events if x.get('Action')=='pass'}
    return not failed and all(x in passed for x in required)

def module_proxy_path(module):
    if not re.fullmatch(r'[A-Za-z0-9./_-]+',module) or any(p in ('','.','..') for p in module.split('/')):
        raise ValueError('unsupported module path')
    return ''.join('!'+c.lower() if c.isupper() else c for c in module)

def module_zip(root,out,module,version):
    module_proxy_path(module)
    if not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[a-z0-9.-]+)?',version):
        raise ValueError('unsupported module/version')
    selected=[]
    for base,dirs,files in os.walk(root,followlinks=False):
        directory=pathlib.Path(base)
        dirs[:]=[name for name in dirs if name not in EXCLUDED and not (directory/name).is_symlink()]
        if directory!=root and 'go.mod' in files:
            dirs.clear()
            continue
        for name in files:
            p=directory/name
            if p.suffix!='.go' and name not in ('go.mod','go.sum','README.md','LICENSE','SECURITY.md','CHANGELOG.md'):continue
            if p.is_file() and not p.is_symlink():selected.append(p)
    with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED) as z:
        for p in sorted(selected):
            rel=p.relative_to(root)
            z.writestr(module+'@'+version+'/'+rel.as_posix(),p.read_bytes())

def forbidden_material(raw):
    return bool(re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----\s+[A-Za-z0-9+/=]{20,}',raw))

def parse_coverage(raw: str):
    """Merge Go coverage blocks (multiple test binaries may cover the same block)."""
    lines=raw.splitlines()
    if not lines or lines[0] not in ('mode: set','mode: count','mode: atomic'):
        raise ValueError('invalid Go coverage mode')
    blocks={}
    for line in lines[1:]:
        m=re.fullmatch(r'(.+):(\d+\.\d+,\d+\.\d+) (\d+) (\d+)',line)
        if not m:raise ValueError('invalid Go coverage block')
        file,span,n,count=m.groups();key=(file,span);n=int(n);count=int(count)
        if key in blocks and blocks[key][0]!=n:raise ValueError('inconsistent coverage block')
        previous=blocks.get(key,(n,0))[1];blocks[key]=(n,previous+count)
    files={}
    for (file,_),(n,count) in blocks.items():
        yes,total=files.get(file,(0,0));files[file]=(yes+(n if count>0 else 0),total+n)
    return files

def coverage_result(files,module,minimum=85.0):
    """No type-only package masquerades as tested runtime behavior.

    Root facades share one Go package; enforce both physical packages and domain
    groups including those facade files. This is stricter than a single root mean.
    """
    prefix=module+'/'
    rel={k[len(prefix):]:v for k,v in files.items() if k.startswith(prefix)}
    packages=['.','internal/engine','internal/authn','auth','kv','pki','baoerr','sensitive','internal/jsondoc','internal/pkiutil','internal/transitutil','internal/pemutil']
    result=[]
    for package in packages:
        names=[n for n in rel if str(pathlib.PurePosixPath(n).parent)==package]
        covered=sum(rel[n][0] for n in names);total=sum(rel[n][1] for n in names)
        percentage=100.0*covered/total if total else None
        result.append({'name':'package:'+package,'covered_statements':covered,'statements':total,'percentage':percentage,'status':'MISSING' if not total else ('PASS' if percentage>=minimum else 'FAIL')})
    for domain in ['kv','pki','transit']:
        names=[n for n in rel if n.startswith(domain+'/') or n.startswith('internal/'+domain+'util/') or ('/' not in n and n.startswith(domain+'_'))]
        covered=sum(rel[n][0] for n in names);total=sum(rel[n][1] for n in names)
        percentage=100.0*covered/total if total else None
        result.append({'name':'domain:'+domain,'covered_statements':covered,'statements':total,'percentage':percentage,'status':'MISSING' if not total else ('PASS' if percentage>=minimum else 'FAIL')})
    return {'threshold':minimum,'unit':'Go statements','passed':all(x['status']=='PASS' for x in result),'groups':result}
