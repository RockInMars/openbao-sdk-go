#!/usr/bin/env python3
"""Per-package and per-domain coverage, always the normal SDK profile."""
import json,re,sys
from tooling import ROOT,parse_coverage,coverage_result
module=re.search(r'^module\s+(\S+)',(ROOT/'go.mod').read_text(),re.M)[1]
try: files=parse_coverage((ROOT/'coverage.out').read_text())
except (OSError,ValueError): files={}
report=coverage_result(files,module)
for x in report['groups']:
 print(x['name']+': '+('MISSING' if x['percentage'] is None else f"{x['percentage']:.1f}%")+' '+x['status'])
(ROOT/'.artifacts').mkdir(exist_ok=True)
(ROOT/'.artifacts/coverage-report.json').write_text(json.dumps(report,indent=2)+'\n')
sys.exit(0 if report['passed'] else 1)
