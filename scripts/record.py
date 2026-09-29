#!/usr/bin/env python3
"""Run one command and record its real, unmodified exit status. Never records environment values."""
import argparse, datetime, json, pathlib, subprocess, sys
from tooling import source_hash
p=argparse.ArgumentParser();p.add_argument('task');p.add_argument('name');p.add_argument('command', nargs=argparse.REMAINDER);a=p.parse_args()
root=pathlib.Path(__file__).resolve().parents[1];out=root/'docs'/'evidence'/a.task;out.mkdir(parents=True,exist_ok=True)
cmd=a.command
if cmd and cmd[0]=='--':cmd=cmd[1:]
started=datetime.datetime.now(datetime.timezone.utc).isoformat()
with (out/(a.name+'.log')).open('w') as f:
 try:
  r=subprocess.run(cmd,cwd=root,stdout=f,stderr=subprocess.STDOUT,check=False);code=r.returncode
 except OSError as e:
  f.write('COMMAND_UNAVAILABLE: '+type(e).__name__+'\n');code=127
entry={'source_sha256':source_hash(root),'command':cmd,'started_at':started,'finished_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'exit_code':code,'log':str((out/(a.name+'.log')).relative_to(root))}
(out/(a.name+'.json')).write_text(json.dumps(entry,ensure_ascii=False,indent=2)+'\n')
print(json.dumps(entry,ensure_ascii=False))
print('\n'.join((out/(a.name+'.log')).read_text().splitlines()[-16:]))
sys.exit(code)
