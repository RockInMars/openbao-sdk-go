#!/usr/bin/env python3
"""Review the actual ledger and current evidence; never edits status to PASS."""
import json,sys
from tooling import ROOT,source_hash,release_problems

def read(name):
 try:
  value=json.loads((ROOT/name).read_text())
  return value if isinstance(value,dict) else {}
 except (ValueError,OSError):return {}
reports={k:read('.artifacts/'+v+'-report.json') for k,v in [('normal','normal'),('integration','integration'),('consumers','consumer'),('scans','security')]}
problems=release_problems(read('task-status.json'),read('acceptance-results.json'),reports,source_hash())
print('RELEASE GATE: '+('FAIL' if problems else 'PASS'))
for item in problems:print('- '+item)
print('AC-072 business migration is a separate gate; it is not inferred here.')
sys.exit(1 if problems else 0)
