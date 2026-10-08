#!/usr/bin/env python3
"""Validate separate job artifacts; no report replaces ledger/release approval."""
import argparse
import json
import pathlib
import sys

from evidence import read_json, report_results, safe_file
from ci_export import build_export
from tooling import ROOT, source_hash, start_report, finish_report

JOBS={'tooling-linux':('tooling','COMMAND_TOOLING','linux'),
      'tooling-windows':('tooling','COMMAND_TOOLING','windows'),
      'normal-linux':('normal','NORMAL_OFFICIAL_CLIENT','linux'),
      'normal-windows':('normal','NORMAL_OFFICIAL_CLIENT','windows'),
      'minimum-normal-linux':('minimum-normal-linux','MINIMUM_GO_COMPATIBILITY','linux'),
      'minimum-normal-windows':('minimum-normal-windows','MINIMUM_GO_COMPATIBILITY','windows'),
      'minimum-consumer-linux':('minimum-consumer-linux','MINIMUM_GO_CONSUMERS','linux'),
      'minimum-consumer-windows':('minimum-consumer-windows','MINIMUM_GO_CONSUMERS','windows'),
      'fuzz':('fuzz','FUZZ_RUNTIME','linux'),
      'consumers':('consumer','INDEPENDENT_CONSUMERS','linux'),
      'security':('security','PINNED_SECURITY_SCANNERS','linux'),
      'real-integration':('integration','REAL_OPENBAO','linux')}


def summarize(artifacts,current_hash,*,reference_root=ROOT):
    result={'status':'PASS','jobs':{}}
    for job,(name,kind,platform) in JOBS.items():
        root=pathlib.Path(artifacts)/job
        try:
            report_file=safe_file(root,'.artifacts/'+name+'-report.json')
            if set(root.rglob('*-report.json'))!={report_file}:raise ValueError('missing or duplicate report artifact')
            report=read_json(report_file)
            if report.get('evidence_class')!=kind or report.get('platform')!=platform:
                raise ValueError('job class or platform mismatch')
            if kind=='REAL_OPENBAO' and safe_file(root,'deploy/test/server-lock.json').read_bytes()!=safe_file(pathlib.Path(reference_root),'deploy/test/server-lock.json').read_bytes():
                raise ValueError('artifact lock differs from current checkout')
            report_results(root,report,current_hash,allow_export=report.get('ci_export')=='structured-v1')
            result['jobs'][job]={'status':'PASS','run_id':report['run_id']}
        except (ValueError,OSError,KeyError,TypeError,AttributeError) as exc:
            result['status']='FAIL';result['jobs'][job]={'status':'FAIL','reason':str(exc)}
    return result


def package_report(root,report_name,output):
    """Whitelist structured evidence; keep original logs only in the isolated job."""
    root=pathlib.Path(root);output=pathlib.Path(output)
    report=read_json(safe_file(root,report_name))
    files=build_export(root,report_name,report)
    output.mkdir(parents=True,exist_ok=False)
    for name,raw in files.items():
        destination=output/name;destination.parent.mkdir(parents=True,exist_ok=True)
        destination.write_bytes(raw)


def main(argv=None):
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--artifacts',type=pathlib.Path,default=ROOT/'.artifacts/downloads')
    p.add_argument('--output',type=pathlib.Path,default=ROOT/'.artifacts/ci-summary.json')
    p.add_argument('--package-report',help='repository-relative report path to whitelist for upload')
    args=p.parse_args(argv)
    if args.package_report:
        package_report(ROOT,args.package_report,args.output)
        return 0
    report=start_report('CI_SUMMARY')
    report.update(summarize(args.artifacts,report['source_sha256']))
    finish_report(report)
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print('CI SUMMARY: '+report['status'])
    for name,item in report['jobs'].items():print(name+': '+item['status']+' '+item.get('reason',''))
    return 0 if report['status']=='PASS' else 1


if __name__=='__main__':sys.exit(main())
