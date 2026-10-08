#!/usr/bin/env python3
"""Bounded command receipts; previous evidence is never overwritten."""
import argparse
import os
import pathlib
import re
import sys
import uuid

from command_runner import succeeded
from reporting import RunRecord
from tooling import ROOT


def main(argv=None):
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--timeout-s',type=float,default=180)
    p.add_argument('--run-id',default=uuid.uuid4().hex)
    p.add_argument('--output-dir',type=pathlib.Path)
    p.add_argument('--evidence-class',choices=['COMMAND_RECEIPT','COMMAND_TOOLING'],default='COMMAND_RECEIPT')
    p.add_argument('task');p.add_argument('name');p.add_argument('command',nargs=argparse.REMAINDER)
    a=p.parse_args(argv)
    if not re.fullmatch(r'OB-\d{3}',a.task) or any(not re.fullmatch(r'[A-Za-z0-9_-]+',x) for x in [a.name,a.run_id]):
        p.error('task/name/run-id must be safe identifiers')
    command=a.command[1:] if a.command[:1]==['--'] else a.command
    out=a.output_dir if a.output_dir else ROOT/'docs/evidence'/a.task
    out.mkdir(parents=True,exist_ok=True)
    name=a.name+'-'+a.run_id
    target=out/(name+'.json');log=out/(name+'.log')
    if target.exists() or log.exists():p.error('evidence already exists; use a new run-id')
    record=RunRecord(a.evidence_class,root=ROOT,run_id=a.run_id)
    report=record.report
    receipt=record.command('tooling' if a.evidence_class=='COMMAND_TOOLING' else 'command',
                           command,a.timeout_s,log=log,env=os.environ.copy())
    record.finalize('PASS' if succeeded(receipt) else ('BLOCKED' if receipt['completion']=='unavailable' else 'FAIL'))
    if a.evidence_class=='COMMAND_TOOLING' and report['status']=='PASS':
        from evidence import report_results
        try:report_results(ROOT,report,report['source_sha256'])
        except (ValueError,OSError,KeyError,TypeError) as exc:
            report['status']='FAIL';report['reason']=str(exc)
    record.write(target,latest=ROOT/'.artifacts/tooling-report.json' if a.evidence_class=='COMMAND_TOOLING' else None)
    print(str(target))
    print(report['status']+': '+receipt['completion']+' '+str(receipt['exit_code']))
    return 0 if report['status']=='PASS' else 1


if __name__=='__main__':sys.exit(main())
