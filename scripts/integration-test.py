#!/usr/bin/env python3
"""Run genuine official-client integration tests against an OWNED new cluster only."""
import datetime,json,os,pathlib,subprocess,sys
from tooling import ROOT,integration_lock,go_test_passes,source_hash
out=ROOT/'.artifacts';out.mkdir(exist_ok=True)
report={'evidence_class':'REAL_OPENBAO','status':'NOT_RUN','source_sha256':source_hash(),'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat()}
try:
 report['baseline']=integration_lock(os.environ)
 cmd=['go','test','-json','-race','-tags=integration','-timeout=5m','-count=1','./tests/integration']
 env=os.environ.copy();env.update({'GOWORK':'off','GOFLAGS':''})
 with (out/'integration-tests.jsonl').open('w') as f:r=subprocess.run(cmd,cwd=ROOT,env=env,stdout=f,stderr=subprocess.STDOUT)
 report['command']=cmd;report['exit_code']=r.returncode
 events=[]
 for line in (out/'integration-tests.jsonl').read_text().splitlines():
  try:events.append(json.loads(line))
  except ValueError:pass
 required=['TestIntegrationRuntime']+['TestIntegrationRuntime/'+x for x in ('TestIntegrationKV','TestIntegrationKVScheduledDeletion','TestIntegrationPKI','TestIntegrationTransit','TestIntegrationAuthNamespace')]
 passed=r.returncode==0 and go_test_passes(events,required)
 report['status']='PASS' if passed else 'FAIL';code=0 if passed else 1
 report['passed_tests']=sum(1 for e in events if e.get('Action')=='pass' and e.get('Test'))
 print('Real OpenBao integration: '+report['status'])
except (ValueError,OSError) as e:
 report['status']='BLOCKED';report['exit_code']=2;report['reason']=str(e) if isinstance(e,ValueError) else 'required integration executable is unavailable';code=2
 print('INTEGRATION BLOCKED: '+report['reason'],file=sys.stderr)
report['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat()
(out/'integration-report.json').write_text(json.dumps(report,indent=2)+'\n')
sys.exit(code)
