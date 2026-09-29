"""Small stdlib-only validation helpers. Not a substitute for Go or security scanners."""
from __future__ import annotations
import hashlib, json, os, pathlib, re, zipfile

ROOT=pathlib.Path(__file__).resolve().parents[1]
EXCLUDED={'.git','.artifacts','__pycache__','.tools','.pytest_cache'}

def source_files(root: pathlib.Path):
    for p in sorted(root.rglob('*')):
        if not p.is_file() or p.is_symlink(): continue
        rel=p.relative_to(root)
        if any(s in EXCLUDED for s in rel.parts): continue
        if rel.parts[:2] in (('docs','evidence'),('docs','spec')): continue
        if p.suffix in ('.go','.py','.hcl','.yaml','.toml') or p.name in ('go.mod','go.sum','Makefile'):
            yield p

def source_hash(root=ROOT):
    h=hashlib.sha256()
    for p in source_files(root):
        h.update(str(p.relative_to(root)).encode());h.update(b'\0');h.update(p.read_bytes());h.update(b'\0')
    return h.hexdigest()

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
    return {'version':version,'image':image or None,'binary_sha256':digest or None,'mode':'binary' if binary else 'docker'}

def go_test_passes(events,required):
    failed=any(x.get('Action') in ('fail','skip') for x in events)
    passed={x.get('Test') for x in events if x.get('Action')=='pass'}
    return not failed and all(x in passed for x in required)

def module_zip(root,out,module,version):
    if not re.fullmatch(r'[a-z0-9./_-]+',module) or not re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+(?:-[a-z0-9.-]+)?',version):
        raise ValueError('unsupported module/version')
    nested=[p.parent.relative_to(root) for p in root.rglob('go.mod') if p.parent!=root and not any(s in EXCLUDED for s in p.relative_to(root).parts)]
    with zipfile.ZipFile(out,'w',zipfile.ZIP_DEFLATED) as z:
        for p in sorted(root.rglob('*')):
            if not p.is_file() or p.is_symlink():continue
            rel=p.relative_to(root)
            if any(x in EXCLUDED for x in rel.parts) or any(rel.is_relative_to(n) for n in nested):continue
            if p.suffix!='.go' and p.name not in ('go.mod','go.sum','README.md','LICENSE','SECURITY.md','CHANGELOG.md'):continue
            z.writestr(module+'@'+version+'/'+rel.as_posix(),p.read_bytes())

def forbidden_material(raw):
    return bool(re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----\s+[A-Za-z0-9+/=]{20,}',raw))

def release_problems(ledger,acceptance,reports,current_hash):
    problems=[]
    tasks={t.get('id'):t for t in ledger.get('tasks',[])}
    for i in range(1,19):
        if tasks.get(f'OB-{i:03}',{}).get('status')!='VERIFIED':problems.append(f'OB-{i:03} not VERIFIED')
    ac={x.get('id'):x for x in acceptance.get('results',[])}
    for i in range(1,72):
        if ac.get(f'AC-{i:03}',{}).get('status')!='PASS':problems.append(f'AC-{i:03} not PASS')
    expected={'integration':'REAL_OPENBAO','consumers':'INDEPENDENT_CONSUMERS','normal':'NORMAL_OFFICIAL_CLIENT','scans':'PINNED_SECURITY_SCANNERS'}
    for name,kind in expected.items():
        r=reports.get(name,{})
        if r.get('status')!='PASS' or r.get('evidence_class')!=kind or r.get('source_sha256')!=current_hash:
            problems.append(name+' evidence missing, unsuccessful, supplemental or stale')
    return problems

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
