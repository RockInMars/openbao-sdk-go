import importlib.util
import pathlib
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'scripts'))
spec = importlib.util.spec_from_file_location('consumer_runner', ROOT / 'scripts/consumer-test.py')
consumer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(consumer)


class ConsumerOfflineTests(unittest.TestCase):
    def test_offline_uses_only_file_proxies_and_signed_sumdb_in_fresh_cache(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            proxy = root / 'prepared proxy'
            (proxy / 'sumdb/sum.golang.org').mkdir(parents=True)
            (proxy / 'sumdb/sum.golang.org/supported').write_bytes(b'')
            with patch.dict('os.environ', {'GOPROXY': 'https://unwanted.invalid', 'GOSUMDB': 'off', 'GOVCS': '*:all'}):
                env = consumer.consumer_environment(root / 'fresh', root / 'sdk-proxy', 'example.org/sdk', proxy)
            self.assertEqual(env['GOPROXY'], (root / 'sdk-proxy').as_uri() + ',' + proxy.as_uri())
            self.assertEqual(env['GOSUMDB'], 'sum.golang.org ' + proxy.as_uri() + '/sumdb/sum.golang.org')
            self.assertEqual(env['GONOSUMDB'], 'example.org/sdk')
            self.assertEqual(env['GOVCS'], '*:off')
            self.assertEqual(env['GOTOOLCHAIN'], 'local')
            self.assertEqual(env['GOAUTH'], 'off')
            self.assertFalse(pathlib.Path(env['GOMODCACHE']).exists())
            self.assertEqual(env['GOPATH'], str(root / 'fresh/gopath'))

    def test_offline_requires_local_sumdb_proxy(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            for proxy in [root / 'missing', root]:
                with self.subTest(proxy=proxy), self.assertRaises(ValueError):
                    consumer.consumer_environment(root / 'fresh', root / 'sdk', 'example.org/sdk', proxy)
            with self.assertRaises(ValueError):
                consumer.consumer_environment(root / 'fresh', root / 'sdk', 'example.org/sdk', pathlib.Path('//remote.invalid/share'))

    def test_network_mode_remains_explicit_and_sumdb_enabled(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            env = consumer.consumer_environment(root, root / 'sdk', 'example.org/sdk', None)
            self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
            self.assertEqual(env['GOPROXY'], (root / 'sdk').as_uri() + ',https://proxy.golang.org')

    def test_local_link_cannot_resolve_to_network_share(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            with patch.object(type(root), 'resolve', return_value=pathlib.Path('//remote.invalid/share')):
                with patch.object(type(root), 'is_dir', side_effect=AssertionError('network path was accessed')):
                    with self.assertRaises(ValueError):
                        consumer.consumer_environment(root / 'fresh', root / 'sdk', 'example.org/sdk', root / 'local-link')


if __name__ == '__main__':
    unittest.main()
