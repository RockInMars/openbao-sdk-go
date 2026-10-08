#!/usr/bin/env python3
"""Normal official-client checks with bounded commands and a fresh race profile."""
import argparse
import platform
import sys

from command_runner import file_sha256, succeeded
from evidence import report_results
from reporting import Run, command_status, run_cli
from tooling import ROOT


def main(argv=None):
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile',choices=['main','minimum-go'],default='main')
    args=parser.parse_args(argv)
    minimum=args.profile=='minimum-go'
    run=Run('minimum-normal-'+platform.system().lower() if minimum else 'normal',
            'MINIMUM_GO_COMPATIBILITY' if minimum else 'NORMAL_OFFICIAL_CLIENT')
    run.probe_go(expected_version='1.25.0' if minimum else '1.26.8')
    profile=run.directory/'coverage.out'
    commands=[('unit',['go','test','-mod=readonly','-json','-count=1','-timeout=120s','./...'],300),
              ('race-coverage',['go','test','-mod=readonly','-race','-count=1','-timeout=180s','-coverprofile='+str(profile),'./...'],600),
              ('vet',['go','vet','-mod=readonly','./...'],180),
              ('modules',['go','mod','verify'],180),
              ('build',['go','build','-mod=readonly','./...'],180),
              ('module-graph',['go','list','-mod=readonly','-m','-json','all'],180)]
    for name,cmd,budget in commands:
        receipt=run.command(name,cmd,budget)
        if name=='race-coverage' and succeeded(receipt) and profile.is_file():
            run.report['coverage_profile']={'path':profile.relative_to(ROOT).as_posix(),
                                           'sha256':file_sha256(profile),'run_id':run.report['run_id']}
    status=command_status(run.report['commands'])
    if status=='PASS':
        run.report['status']='PASS'
        try:
            parsed=report_results(ROOT,run.report,run.report['source_sha256'])
            run.report['passed_tests']=len(parsed['tests'])
        except (ValueError,OSError,KeyError,TypeError) as exc:
            status='FAIL';run.report['reason']=str(exc)
    return run.finish(status)


if __name__=='__main__':sys.exit(run_cli(main))
