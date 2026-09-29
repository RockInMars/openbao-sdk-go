#!/usr/bin/env python3
"""Fixed-version external scanners; missing tools/database never imply clean."""
import datetime,json,os,subprocess,sys
from tooling import ROOT,source_hash
out=ROOT/'.artifacts';out.mkdir(exist_ok=True)
env=os.environ.copy();env.update({'GOWORK':'off','GOFLAGS':''})
for k in list(env):
 if k.startswith('GITLEAKS_'):env.pop(k)
commands=[('govulncheck',['go','run','golang.org/x/vuln/cmd/govulncheck@v1.1.4','./...']),('gitleaks',['go','run','github.com/zricethezav/gitleaks/v8@v8.24.3','dir','--redact=100','--no-banner','--config',str(ROOT/'deploy/test/gitleaks.toml'),'--report-format=json','--report-path',str(out/'gitleaks-redacted.json'),str(ROOT)])]
report={'evidence_class':'PINNED_SECURITY_SCANNERS','status':'NOT_RUN','source_sha256':source_hash(),'commands':[],'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat()}
for name,cmd in commands:
 with (out/(name+'.log')).open('w') as f:
  try:r=subprocess.run(cmd,cwd=ROOT,env=env,stdout=f,stderr=subprocess.STDOUT);code=r.returncode
  except OSError:code=127;f.write('Scanner command unavailable\n')
 report['commands'].append({'name':name,'command':cmd,'exit_code':code,'log':'.artifacts/'+name+'.log'})
 print(name+': exit '+str(code))
report['status']='PASS' if all(x['exit_code']==0 for x in report['commands']) else 'FAIL'
report['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat()
(out/'security-report.json').write_text(json.dumps(report,indent=2)+'\n')
sys.exit(0 if report['status']=='PASS' else 1)
