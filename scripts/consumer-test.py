#!/usr/bin/env python3
"""Two independent go.mod consumers via a temporary module proxy; NEVER replace."""
import datetime,json,os,pathlib,re,shutil,subprocess,sys,tempfile
from tooling import ROOT,module_zip,source_hash,go_test_passes
out=ROOT/'.artifacts';out.mkdir(exist_ok=True)
module_match=re.search(r'^module\s+(\S+)',(ROOT/'go.mod').read_text(),re.M)
if not module_match:raise SystemExit('Invalid SDK go.mod')
module=module_match[1];version='v0.1.0-dev.0'
report={'evidence_class':'INDEPENDENT_CONSUMERS','status':'NOT_RUN','source_sha256':source_hash(),'delivery':'temporary file GOPROXY, NOT a remote publication','consumers':[]}
with tempfile.TemporaryDirectory(prefix='bao-consumers-') as tmp:
 tmp=pathlib.Path(tmp);proxy=tmp/'proxy';vdir=proxy/module/'@v';vdir.mkdir(parents=True)
 module_zip(ROOT,vdir/(version+'.zip'),module,version)
 (vdir/(version+'.mod')).write_bytes((ROOT/'go.mod').read_bytes())
 (vdir/(version+'.info')).write_text(json.dumps({'Version':version,'Time':'2026-09-28T00:00:00Z'}))
 (vdir/'list').write_text(version+'\n')
 env=os.environ.copy();env.update({'GOWORK':'off','GOFLAGS':'','GOMODCACHE':str(tmp/'module-cache'),'GOPROXY':proxy.as_uri()+',https://proxy.golang.org','GONOSUMDB':module,'GONOPROXY':'none','GOPRIVATE':''})
 for name,expected in [('reader','TestIndependentReader'),('signer','TestIndependentSigner')]:
  work=tmp/name;shutil.copytree(ROOT/'tests/consumers'/name,work)
  mod=work/'go.mod'
  if re.search(r'^replace\b',mod.read_text(),re.M):raise SystemExit('Consumer replace directive forbidden')
  commands=[['go','mod','tidy'],['go','test','-json','-race','-count=1','./...']]
  result={'name':name,'status':'NOT_RUN','commands':[]};events=[]
  for index,cmd in enumerate(commands):
   log=out/(f'consumer-{name}-{index}.log')
   with log.open('w') as f:
    try:r=subprocess.run(cmd,cwd=work,env=env,stdout=f,stderr=subprocess.STDOUT);code=r.returncode
    except OSError:code=127;f.write('Go command unavailable\n')
   result['commands'].append({'command':cmd,'exit_code':code,'log':str(log.relative_to(ROOT))})
   if code!=0:result['status']='FAIL';break
   if index==1:
    for line in log.read_text().splitlines():
     try:events.append(json.loads(line))
     except ValueError:pass
    result['status']='PASS' if go_test_passes(events,[expected]) else 'FAIL'
  report['consumers'].append(result);print(name+': '+result['status'])
report['status']='PASS' if len(report['consumers'])==2 and all(x['status']=='PASS' for x in report['consumers']) else 'FAIL'
report['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat()
(out/'consumer-report.json').write_text(json.dumps(report,indent=2)+'\n')
sys.exit(0 if report['status']=='PASS' else 1)
