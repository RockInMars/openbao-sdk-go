#!/usr/bin/env python3
"""Independent consumers through a temporary module proxy, never replace."""
import argparse
import json
import pathlib
import platform
import re
import shutil
import sys
import tempfile
from urllib.parse import urlsplit

from command_runner import succeeded
from evidence import go_results, report_results
from reporting import Run, go_environment, run_cli
from tooling import ROOT, module_zip, module_proxy_path


def consumer_environment(tmp, sdk_proxy, module, offline_proxy):
    env=go_environment()
    dependency_proxy='https://proxy.golang.org'
    sumdb='sum.golang.org'
    if offline_proxy is not None:
        if str(offline_proxy).startswith(('//','\\\\')):
            raise ValueError('offline proxy must be a local directory, not a network share')
        offline_proxy=offline_proxy.resolve()
        dependency_proxy=offline_proxy.as_uri()
        parsed=urlsplit(dependency_proxy)
        if str(offline_proxy).startswith(('//','\\\\')) or parsed.scheme!='file' or parsed.netloc:
            raise ValueError('resolved offline proxy must stay on the local filesystem')
        if not offline_proxy.is_dir() or not (offline_proxy/'sumdb/sum.golang.org/supported').is_file():
            raise ValueError('offline proxy requires prepared signed sumdb mirror with supported marker')
        # Explicit local checksum URL prevents fallback to sum.golang.org on cache misses.
        sumdb+=' '+dependency_proxy+'/sumdb/sum.golang.org'
    env.update(GOMODCACHE=str(tmp/'module-cache'),GOPATH=str(tmp/'gopath'),
        GOPROXY=sdk_proxy.as_uri()+','+dependency_proxy,GONOSUMDB=module,
        GONOPROXY='none',GOPRIVATE='',GOINSECURE='',GOSUMDB=sumdb,GOAUTH='off',GOVCS='*:off')
    return env


def main(argv=None):
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile',choices=['main','minimum-go'],default='main')
    mode=parser.add_mutually_exclusive_group()
    mode.add_argument('--allow-network',action='store_true',help='authorize public dependency downloads into an isolated cache')
    mode.add_argument('--offline-proxy',type=pathlib.Path,help='prepared local module and signed sumdb proxy; never falls back to the network')
    args=parser.parse_args(argv)
    minimum=args.profile=='minimum-go'
    run=Run('minimum-consumer-'+platform.system().lower() if minimum else 'consumer',
            'MINIMUM_GO_CONSUMERS' if minimum else 'INDEPENDENT_CONSUMERS')
    run.probe_go(expected_version='1.25.0' if minimum else '1.26.8')
    run.report.update(delivery='temporary file GOPROXY, NOT a remote publication',consumers=[])
    if not args.allow_network and args.offline_proxy is None:
        return run.finish('BLOCKED','independent consumers require prepared --offline-proxy or explicit --allow-network authorization')
    module=re.search(r'^module\s+(\S+)',(ROOT/'go.mod').read_text(encoding='utf-8'),re.M)[1]
    version='v0.1.0-dev.0'
    with tempfile.TemporaryDirectory(prefix='bao-consumers-') as tmp:
        tmp=pathlib.Path(tmp);proxy=tmp/'proxy';vdir=proxy/module_proxy_path(module)/'@v';vdir.mkdir(parents=True)
        module_zip(ROOT,vdir/(version+'.zip'),module,version)
        (vdir/(version+'.mod')).write_bytes((ROOT/'go.mod').read_bytes())
        (vdir/(version+'.info')).write_text(json.dumps({'Version':version,'Time':'2026-09-28T00:00:00Z'}),encoding='utf-8')
        (vdir/'list').write_text(version+'\n',encoding='utf-8')
        try:env=consumer_environment(tmp,proxy,module,args.offline_proxy)
        except ValueError as exc:return run.finish('BLOCKED',str(exc))
        run.report['dependency_mode']='offline-proxy' if args.offline_proxy is not None else 'authorized-network'
        run.report['dependency_proxy']=args.offline_proxy.resolve().as_uri() if args.offline_proxy is not None else 'https://proxy.golang.org'
        run.report['checksum_database']=env['GOSUMDB']
        run.report['fresh_module_cache']=True
        for name,expected in [('reader','TestIndependentReader'),('signer','TestIndependentSigner')]:
            work=tmp/name;shutil.copytree(ROOT/'tests/consumers'/name,work)
            if re.search(r'^replace\b',(work/'go.mod').read_text(encoding='utf-8'),re.M):
                return run.finish('FAIL','consumer replace directive forbidden')
            if (work/'go.sum').exists():
                return run.finish('FAIL','consumer must authenticate dependencies without preseeded go.sum')
            item={'name':name,'directory':str(work),'status':'NOT_RUN','commands':[]};run.report['consumers'].append(item)
            for suffix,cmd,budget in [('tidy',['go','mod','tidy'],180),('test',['go','test','-mod=readonly','-json','-race','-count=1','-timeout=180s','./...'],600)]:
                receipt=run.command(name+'-'+suffix,cmd,budget,cwd=work,env=env)
                item['commands'].append(receipt)
                if not succeeded(receipt):item['status']='FAIL';break
                if suffix=='test':
                    try:
                        results=go_results((ROOT/receipt['log']).read_text(encoding='utf-8'))
                        item['status']='PASS' if any(n==expected for _,n in results) else 'FAIL'
                    except (ValueError,OSError):item['status']='FAIL'
            run.save()
    run.report['status']='PASS' if all(x['status']=='PASS' for x in run.report['consumers']) else 'FAIL'
    if run.report['status']=='PASS':
        try:report_results(ROOT,run.report,run.report['source_sha256'])
        except (ValueError,OSError,KeyError,TypeError) as exc:return run.finish('FAIL',str(exc))
    return run.finish()


if __name__=='__main__':sys.exit(run_cli(main))
