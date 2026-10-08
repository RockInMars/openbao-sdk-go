#!/usr/bin/env python3
"""Review the actual ledger and current evidence; never edits status to PASS."""
import json,sys
from tooling import ROOT,source_hash
from evidence import FORMAL_REPORT_NAMES, release_problems

def read(name):
 try:
  value=json.loads((ROOT/name).read_text(encoding='utf-8'))
  return value if isinstance(value,dict) else {}
 except (ValueError,OSError):return {}
reports={k:read('.artifacts/'+v+'-report.json') for k,v in FORMAL_REPORT_NAMES.items()}
problems=release_problems(ROOT,read('task-status.json'),read('acceptance-results.json'),reports,source_hash())
print('RELEASE GATE: '+('FAIL' if problems else 'PASS'))
for item in problems:print('- '+item)
print('AC-072 business migration is a separate gate; it is not inferred here.')
sys.exit(1 if problems else 0)
