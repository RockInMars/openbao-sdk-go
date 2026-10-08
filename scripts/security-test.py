#!/usr/bin/env python3
"""Pinned scanners; installing tools and refreshing databases require explicit opt-in."""
import argparse
import hashlib
import sys
from evidence import report_results
from reporting import Run, go_environment, command_status, run_cli
from tooling import ROOT


def scanner_environment():
    env=go_environment(allow_network=True)
    for key in list(env):
        if key.startswith('GITLEAKS_') or key == 'GOVULNDB':env.pop(key)
    env.update(GOPROXY='https://proxy.golang.org,direct',GOSUMDB='sum.golang.org',
               GOPRIVATE='',GONOPROXY='',GONOSUMDB='',GOINSECURE='')
    return env


def main(argv=None):
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--allow-network',action='store_true',help='authorize pinned scanner/module and vulnerability database downloads')
    args=parser.parse_args(argv)
    run=Run('security','PINNED_SECURITY_SCANNERS')
    run.probe_go()
    if not args.allow_network:return run.finish('BLOCKED','scanner and database downloads need explicit --allow-network authorization')
    env=scanner_environment()
    findings=run.directory/'gitleaks-redacted.json'
    run.report['findings']=findings.relative_to(ROOT).as_posix()
    run.command('govulncheck',['go','run','golang.org/x/vuln/cmd/govulncheck@v1.1.4',
                '-db=https://vuln.go.dev','-show=version','./...'],600,env=env)
    run.command('gitleaks',['go','run','github.com/zricethezav/gitleaks/v8@v8.24.3','dir','--redact=100','--no-banner',
        '--config',str(ROOT/'deploy/test/gitleaks.toml'),'--report-format=json','--report-path',str(findings),'.'],600,env=env)
    if findings.is_file():run.report['findings_sha256']=hashlib.sha256(findings.read_bytes()).hexdigest()
    status=command_status(run.report['commands'])
    if status=='PASS':
        run.report['status']='PASS'
        try:report_results(ROOT,run.report,run.report['source_sha256'])
        except (ValueError,OSError,KeyError,TypeError) as exc:return run.finish('FAIL',str(exc))
    return run.finish(status)


if __name__=='__main__':sys.exit(run_cli(main))
