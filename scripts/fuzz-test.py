#!/usr/bin/env python3
"""Time-bounded, individually evidenced fuzz targets."""
import sys
from evidence import FUZZ_TARGETS, report_results
from reporting import Run, command_status, run_cli
from tooling import ROOT


def fuzz_commands():
    return {name:['go','test','-mod=readonly','-run=^$','-fuzz='+name,'-fuzztime=30s','-parallel=1','-timeout=90s',package]
            for name,package in FUZZ_TARGETS.items()}


def main():
    run=Run('fuzz','FUZZ_RUNTIME')
    run.probe_go()
    for name,cmd in fuzz_commands().items():run.command(name,cmd,120)
    status=command_status(run.report['commands'])
    if status=='PASS':
        run.report['status']='PASS'
        try:run.report['passed_targets']=sorted(report_results(ROOT,run.report,run.report['source_sha256'])['fuzz_targets'])
        except (ValueError,OSError,KeyError,TypeError) as exc:return run.finish('FAIL',str(exc))
    return run.finish(status)


if __name__=='__main__':sys.exit(run_cli(main))
