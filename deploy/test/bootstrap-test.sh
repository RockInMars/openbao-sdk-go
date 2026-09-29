#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)"
# Bootstrap lives in internal/testenv and cannot attach to an existing server.
exec python3 scripts/integration-test.py
