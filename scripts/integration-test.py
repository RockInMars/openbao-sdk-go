#!/usr/bin/env python3
"""Real official-client tests against an owned, checksum-pinned cluster only."""
import os
import sys
from evidence import report_results
from reporting import Run, command_status, run_cli
from tooling import ROOT, integration_lock


def main():
    run=Run('integration','REAL_OPENBAO')
    run.probe_go()
    try:run.report['baseline']=integration_lock(os.environ)
    except (ValueError,OSError) as exc:return run.finish('BLOCKED',str(exc))
    run.command('integration',['go','test','-mod=readonly','-json','-race','-tags=integration','-timeout=5m','-count=1','./tests/integration'],600)
    status=command_status(run.report['commands'])
    if status=='PASS':
        run.report['status']='PASS'
        try:run.report['passed_tests']=len(report_results(ROOT,run.report,run.report['source_sha256'])['tests'])
        except (ValueError,OSError,KeyError,TypeError) as exc:return run.finish('FAIL',str(exc))
    return run.finish(status)


if __name__=='__main__':sys.exit(run_cli(main))
