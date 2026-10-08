# Synthetic evidence for tests only; never certifies an OpenBao deployment.
import hashlib
import json
import pathlib
from evidence import release_problems


class EvidenceFixture:
    def __init__(self, root):
        self.root = pathlib.Path(root)
        self.identity = {'source_hash_version': 2, 'source_sha256': 'a'*64}
        self.write('go.mod', 'module example.org/sdk\ngo 1.25.0\n')
        self.ledger = {**self.identity, 'tasks': [dict(self.identity, id=f'OB-{i:03}', status='VERIFIED',
                       evidence=['evidence/normal.json'], acceptance_ids=[f'AC-{min(i,71):03}']) for i in range(1,19)]}
        self.acceptance = {**self.identity, 'results': [dict(self.identity, id=f'AC-{i:03}', status='PASS',
                           evidence=['evidence/normal.json']) for i in range(1,72)]}
        self.rules = {'schema_version': 1, 'static_revalidation_allowlist': [], 'rules': {
            f'AC-{i:03}': {'kind': 'runtime', 'required_evidence': [
                {'allowed_classes': ['NORMAL_OFFICIAL_CLIENT'], 'tests': [{'package': 'tests/dependency', 'name': 'TestDependencyBaseline'}]}]}
            for i in range(1,72)}}
        self.reports = {}
        self.add_report('normal', 'NORMAL_OFFICIAL_CLIENT')
        self.add_report('integration', 'REAL_OPENBAO')
        self.add_report('consumers', 'INDEPENDENT_CONSUMERS')
        self.add_report('scans', 'PINNED_SECURITY_SCANNERS')
        for platform in ('linux','windows'):
            self.add_report('minimum-normal-'+platform, 'MINIMUM_GO_COMPATIBILITY', platform=platform)
            self.add_report('minimum-consumer-'+platform, 'MINIMUM_GO_CONSUMERS', platform=platform)
        self.save()

    def write(self, name, value):
        p = self.root/name
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(value if isinstance(value,str) else json.dumps(value), encoding='utf-8')
        return p

    def receipt(self, name, log, command=None):
        path = self.write('evidence/'+name+'.log', log)
        return {'name': name, 'command': command or ['go','test','-mod=readonly','-json','-count=1','-timeout=120s','./...'],
                'cwd': str(self.root),
                'completion':'exited','exit_code':0,'log':path.relative_to(self.root).as_posix(),
                'log_sha256':hashlib.sha256(path.read_bytes()).hexdigest()}

    def events(self, names, package='example.org/sdk/tests/dependency'):
        return '\n'.join(json.dumps({'Action':action,'Package':package,'Test':name})
                         for name in names for action in ('run','pass'))+'\n'+json.dumps({'Action':'pass','Package':package})+'\n'

    def add_report(self, name, kind, *, platform='windows'):
        r = {**self.identity, 'source_before':dict(self.identity), 'source_after':dict(self.identity),
             'status':'PASS','evidence_class':kind,'run_id':'fixture-'+name,'commands':[], 'working_directory':str(self.root), 'platform':platform}
        go_version='1.25.0' if kind in {'MINIMUM_GO_COMPATIBILITY','MINIMUM_GO_CONSUMERS'} else '1.26.8'
        version_text=f'go version go{go_version} {platform}/amd64'
        version=self.receipt(name+'-go-version',version_text+'\n',['go','version'])
        version['name']='go-version'
        r.update(environment_commands=[version],tool_versions={'go':version_text})
        if kind in {'NORMAL_OFFICIAL_CLIENT','MINIMUM_GO_COMPATIBILITY'}:
            r['commands']=[self.receipt('unit',self.events(['TestDependencyBaseline']))]
            r['commands'] += [self.receipt(x,'ok\n') for x in ['race-coverage','vet','modules','build','module-graph']]
            paths=['client.go','kv_client.go','pki_client.go','transit_client.go']+[p+'/file.go' for p in ['internal/engine','internal/authn','auth','kv','pki','baoerr','sensitive','internal/jsondoc','internal/pkiutil','internal/transitutil','internal/pemutil']]
            profile=self.write('evidence/coverage.out','mode: atomic\n'+''.join('example.org/sdk/'+p+':1.1,2.1 1 1\n' for p in paths))
            r['coverage_profile']={'path':'evidence/coverage.out','sha256':hashlib.sha256(profile.read_bytes()).hexdigest(),'run_id':r['run_id']}
            expected=[['go','test','-mod=readonly','-race','-count=1','-timeout=180s','-coverprofile='+str(profile),'./...'], ['go','vet','-mod=readonly','./...'], ['go','mod','verify'], ['go','build','-mod=readonly','./...'], ['go','list','-mod=readonly','-m','-json','all']]
            for receipt,command in zip(r['commands'][1:],expected):receipt['command']=command
        elif name == 'integration':
            names=['TestIntegrationRuntime']+['TestIntegrationRuntime/'+x for x in ['TestIntegrationKV','TestIntegrationKVScheduledDeletion','TestIntegrationPKI','TestIntegrationTransit','TestIntegrationAuthNamespace']]
            r['commands']=[self.receipt('integration',self.events(names,'example.org/sdk/tests/integration'))]
            r['commands'][0]['command']=['go','test','-mod=readonly','-json','-race','-tags=integration','-timeout=5m','-count=1','./tests/integration']
            r['baseline']={'version':'2.7.0','mode':'docker','image':'openbao/openbao@sha256:'+'b'*64,'binary_sha256':None}
            self.write('deploy/test/server-lock.json',{'status':'PINNED',**{k:v for k,v in r['baseline'].items() if k!='mode'}})
        elif kind in {'INDEPENDENT_CONSUMERS','MINIMUM_GO_CONSUMERS'}:
            r['consumers']=[{'name':n,'status':'PASS','commands':[self.receipt(n+'-tidy','ok'),self.receipt(n+'-test',self.events([test],'example.org/'+n))]} for n,test in [('reader','TestIndependentReader'),('signer','TestIndependentSigner')]]
            for item in r['consumers']:
                item['directory']=str(self.root.parent/'isolated'/item['name'])
                item['commands'][0]['command']=['go','mod','tidy']
                item['commands'][1]['command']=['go','test','-mod=readonly','-json','-race','-count=1','-timeout=180s','./...']
                for receipt in item['commands']:receipt['cwd']=item['directory']
                r['commands'].extend(item['commands'])
        else:
            r['commands']=[self.receipt('govulncheck','Scanner: govulncheck@v1.1.4\nDB: https://vuln.go.dev\nDB updated: 2026-09-29 00:00:00 +0000 UTC\nNo vulnerabilities found.\n',['go','run','golang.org/x/vuln/cmd/govulncheck@v1.1.4','-db=https://vuln.go.dev','-show=version','./...']),self.receipt('gitleaks','no leaks found\n',['go','run','github.com/zricethezav/gitleaks/v8@v8.24.3','dir','--redact=100'])]
            self.write('evidence/gitleaks.json',[])
            r['findings']='evidence/gitleaks.json'
            r['findings_sha256']=hashlib.sha256((self.root/r['findings']).read_bytes()).hexdigest()
            r['commands'][1]['command'] += ['--no-banner','--config',str(self.root/'deploy/test/gitleaks.toml'),'--report-format=json','--report-path',str(self.root/r['findings']),'.']
        self.reports[name]=r

    def save(self):
        self.write('scripts/acceptance-rules.json',self.rules)
        for name,report in self.reports.items():self.write('evidence/'+name+'.json',report)

    def problems(self):
        self.save()
        return release_problems(self.root,self.ledger,self.acceptance,self.reports,'a'*64)
