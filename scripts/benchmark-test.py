#!/usr/bin/env python3
"""Record a reproducible performance baseline; this is not a release threshold."""
import math
import platform
import re
import sys

from evidence import receipt_log
from reporting import Run, command_status, run_cli
from tooling import ROOT

EXPECTED = (
    'BenchmarkDocument/small_64', 'BenchmarkDocument/large_65536',
    'BenchmarkCertificateValidation/p256_chain2',
    'BenchmarkTransitEncoding/payload_32', 'BenchmarkTransitEncoding/payload_4096',
    'BenchmarkExecutorConcurrent/slots8',
)


def benchmark_results(text):
    results = {name: [] for name in EXPECTED}
    for line in text.splitlines():
        match = re.match(r'^(Benchmark\S+)-\d+\s+(\d+)\s+(.+)$', line)
        if not match:
            continue
        name, iterations, rest = match.groups()
        if name not in results:
            raise ValueError('unexpected benchmark: ' + name)
        fields = rest.split()
        if len(fields) % 2 or int(iterations) <= 0:
            raise ValueError('benchmark has no valid iterations')
        metrics = {fields[i+1]: float(fields[i]) for i in range(0, len(fields), 2)}
        if any(not math.isfinite(v) or v < 0 for v in metrics.values()):
            raise ValueError('invalid benchmark metric')
        if not {'ns/op', 'B/op', 'allocs/op'} <= metrics.keys() or metrics['ns/op'] <= 0:
            raise ValueError('missing allocation or timing metrics')
        if name.startswith('BenchmarkExecutor'):
            if not {'p50-ns', 'p95-ns', 'latency-samples', 'workers'} <= metrics.keys():
                raise ValueError('independent latency samples are required')
            if metrics['latency-samples'] != 256 or metrics['workers'] < 1 or not 0 < metrics['p50-ns'] <= metrics['p95-ns']:
                raise ValueError('invalid independent latency samples')
        results[name].append({'iterations': int(iterations), **metrics})
    if any(len(rounds) != 3 for rounds in results.values()):
        raise ValueError('all six enabled sub-benchmarks must run exactly three rounds')
    return results


def main():
    run = Run('benchmark', 'BENCHMARK_BASELINE')
    run.probe_go()
    run.report['cpu'] = platform.processor()
    run.report['machine'] = platform.machine()
    receipt = run.command('benchmark', ['go', 'test', '-mod=readonly', '-run=^$', '-bench=.', '-benchmem', '-benchtime=1s', '-count=3', './kv', './internal/pkiutil', './internal/transitutil', './internal/engine'], 240)
    status = command_status(run.report['commands'])
    if status == 'PASS':
        try:
            run.report['benchmarks'] = benchmark_results(receipt_log(ROOT, receipt))
        except (ValueError, OSError) as exc:
            return run.finish('FAIL', str(exc))
    return run.finish(status)


if __name__ == '__main__':
    sys.exit(run_cli(main))
