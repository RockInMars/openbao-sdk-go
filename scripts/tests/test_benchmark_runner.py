import importlib.util
from pathlib import Path
import unittest
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))


class BenchmarkEvidenceTests(unittest.TestCase):
    def load(self):
        spec = importlib.util.spec_from_file_location("bench_runner", Path(__file__).resolve().parents[1] / "benchmark-test.py")
        mod = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(mod)
        return mod

    def test_zero_matching_or_missing_round_is_not_pass(self):
        mod = self.load()
        with self.assertRaises(ValueError):
            mod.benchmark_results("ok sdk 0.1s [no tests to run]\n")
        text = self.fixture(mod)
        self.assertEqual(len(mod.benchmark_results(text)), 6)
        with self.assertRaises(ValueError):
            mod.benchmark_results("\n".join(text.splitlines()[1:]))
        with self.assertRaises(ValueError):
            mod.benchmark_results(text.replace(" 10 ", " 0 ", 1))

    def test_percentiles_need_independent_sample_metrics(self):
        mod = self.load()
        with self.assertRaises(ValueError):
            mod.benchmark_results(self.fixture(mod).replace(" 256 latency-samples", ""))

    @staticmethod
    def fixture(mod):
        lines = []
        for name in mod.EXPECTED:
            extra = " 256 latency-samples 50 p50-ns 90 p95-ns 8 workers" if name.startswith("BenchmarkExecutor") else ""
            lines.extend([f"{name}-8 10 100 ns/op 20 B/op 1 allocs/op{extra}"] * 3)
        return "\n".join(lines)
