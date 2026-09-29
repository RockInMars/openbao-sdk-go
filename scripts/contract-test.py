#!/usr/bin/env python3
"""Supplemental own-code tests only. Never use this result as upstream/integration proof."""
import json, os, pathlib, subprocess, sys, tempfile
root=pathlib.Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix='bao-contract-overlay-') as tmp:
    d=pathlib.Path(tmp);sender=d/'official_sender.go'
    sender.write_text('''package bao
import ("net/http"; "git.example.com/infra/openbao-sdk-go/internal/engine"; "git.example.com/infra/openbao-sdk-go/internal/testutil")
func newProtocolSender(address,namespace string,h *http.Client,t *engine.Transport)(engine.Sender,error){return testutil.NewHTTPSender(address,namespace,h,t),nil}
''')
    overlay=d/'overlay.json';overlay.write_text(json.dumps({'Replace':{str(root/'official_sender.go'):str(sender)}}))
    args=sys.argv[1:]
    if not args:
        args=['-race','-timeout=90s','.','./auth','./kv','./pki','./transit','./baoerr','./sensitive','./diagnostics','./observe','./internal/engine','./internal/authn','./internal/jsondoc','./internal/pkiutil','./internal/pemutil','./internal/transitutil','./internal/testenv','./internal/testutil','./examples/...']
    env=os.environ.copy();env['GOWORK']='off';env['GOTOOLCHAIN']='local'
    print('EVIDENCE_CLASS=CONTRACT_WITH_TEST_SENDER; official api/v2 factory NOT EXECUTED; vet NOT RUN (Go 1.23 overlay limitation)',flush=True)
    sys.exit(subprocess.call(['go','test','-vet=off','-overlay',str(overlay),*args],cwd=root,env=env))
