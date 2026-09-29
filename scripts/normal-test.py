#!/usr/bin/env python3
"""Normal unmodified SDK gates. No overlays, replacements or skip-to-green."""
import datetime,json,os,pathlib,re,subprocess,sys
from tooling import ROOT,source_hash,parse_coverage,coverage_result
out=ROOT/'.artifacts';out.mkdir(exist_ok=True)
profile=ROOT/'coverage.out'
if profile.exists():profile.unlink() # only this gate's previous coverage output
commands=[('unit',['go','test','-count=1','./...']),('race-coverage',['go','test','-race','-count=1','-coverprofile=coverage.out','./...']),('vet',['go','vet','./...']),('modules',['go','mod','verify']),('build',['go','build','./...']),('module-graph',['go','list','-m','-json','all']),('test-list',['go','test','-json','-count=1','./...'])]
report={'evidence_class':'NORMAL_OFFICIAL_CLIENT','status':'NOT_RUN','source_sha256':source_hash(),'started_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'commands':[]}
env=os.environ.copy();env.update({'GOWORK':'off','GOFLAGS':''})
for name,cmd in commands:
 log=out/('normal-'+name+'.log')
 with log.open('w') as f:
  try:r=subprocess.run(cmd,cwd=ROOT,env=env,stdout=f,stderr=subprocess.STDOUT);code=r.returncode
  except OSError:code=127;f.write('Go command unavailable\n')
 report['commands'].append({'name':name,'command':cmd,'exit_code':code,'log':str(log.relative_to(ROOT))})
 print(name+': exit '+str(code),flush=True)
module=re.search(r'^module\s+(\S+)',(ROOT/'go.mod').read_text(),re.M)[1]
try:
 coverage=coverage_result(parse_coverage(profile.read_text()),module)
except (OSError,ValueError):coverage=coverage_result({},module)
(out/'coverage-report.json').write_text(json.dumps(coverage,indent=2)+'\n')
report['coverage_passed']=coverage['passed'];report['coverage_report']='.artifacts/coverage-report.json'
events=[]
for line in (out/'normal-test-list.log').read_text().splitlines():
 try:events.append(json.loads(line))
 except ValueError:pass
report['passed_tests']=sum(1 for e in events if e.get('Test') and e.get('Action')=='pass')
report['failed_tests']=sum(1 for e in events if e.get('Test') and e.get('Action')=='fail')
report['skipped_tests']=sum(1 for e in events if e.get('Test') and e.get('Action')=='skip')
report['official_baseline_passed']=any(e.get('Test')=='TestDependencyBaseline' and e.get('Action')=='pass' for e in events)
report['status']='PASS' if all(x['exit_code']==0 for x in report['commands']) and report['official_baseline_passed'] and report['skipped_tests']==0 and coverage['passed'] else 'FAIL'
report['finished_at']=datetime.datetime.now(datetime.timezone.utc).isoformat()
(out/'normal-report.json').write_text(json.dumps(report,indent=2)+'\n')
print('NORMAL_OFFICIAL_CLIENT: '+report['status'])
sys.exit(0 if report['status']=='PASS' else 1)
