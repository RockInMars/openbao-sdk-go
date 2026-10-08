#!/usr/bin/env python3
"""Inspect a selected profile; release provenance additionally requires its race receipt."""
import argparse
import hashlib
import json
import pathlib
import re
import sys
from tooling import ROOT,parse_coverage,coverage_result,start_report,finish_report


def main(argv=None):
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--profile',type=pathlib.Path,default=ROOT/'coverage.out')
    parser.add_argument('--output',type=pathlib.Path,default=ROOT/'.artifacts/coverage-report.json')
    args=parser.parse_args(argv)
    report=start_report('COVERAGE_ANALYSIS')
    module=re.search(r'^module\s+(\S+)',(ROOT/'go.mod').read_text(encoding='utf-8'),re.M)[1]
    reason=None
    try:
        raw=args.profile.read_bytes()
        report['profile_sha256']=hashlib.sha256(raw).hexdigest()
        files=parse_coverage(raw.decode('utf-8'))
    except (OSError,ValueError) as exc:files={};reason=type(exc).__name__
    report.update(coverage_result(files,module))
    report['profile']=str(args.profile)
    if reason:report['reason']=reason
    report['status']='PASS' if report['passed'] else 'FAIL'
    finish_report(report)
    if report['status']!='PASS':report['passed']=False
    for item in report['groups']:
        print(item['name']+': '+('MISSING' if item['percentage'] is None else f"{item['percentage']:.2f}%")+' '+item['status'])
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(report,indent=2)+'\n',encoding='utf-8')
    return 0 if report['passed'] else 1


if __name__=='__main__':sys.exit(main())
