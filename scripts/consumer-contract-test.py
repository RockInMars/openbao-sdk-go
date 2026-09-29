#!/usr/bin/env python3
"""Supplemental consumer-source tests in the SDK module, NOT independent builds.

Copies only consumer .go files to isolated temporary directories in .artifacts.
No go.mod rewrite and no replace; genuine consumer-test remains a separate gate.
"""
import pathlib,shutil,subprocess,sys,tempfile
from tooling import ROOT
out=ROOT/'.artifacts';out.mkdir(exist_ok=True)
with tempfile.TemporaryDirectory(prefix='consumer-contract-',dir=out) as d:
 packages=[]
 for name in ('reader','signer'):
  dest=pathlib.Path(d)/name;dest.mkdir()
  for source in (ROOT/'tests/consumers'/name).glob('*.go'):shutil.copy2(source,dest/source.name)
  packages.append('./'+dest.relative_to(ROOT).as_posix())
 print('SUPPLEMENTAL_SHARED_MODULE_CONSUMER_CONTRACTS; NOT independent module/build/publication evidence',flush=True)
 sys.exit(subprocess.call([sys.executable,str(ROOT/'scripts/contract-test.py'),'-race','-count=1',*packages],cwd=ROOT))
