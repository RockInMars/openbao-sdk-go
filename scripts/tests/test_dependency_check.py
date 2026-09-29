"""Local validator tests; these do NOT download or validate an upstream module."""
import base64
import importlib.util
import json
import pathlib
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/dependency-check.py'
MODULE = 'github.com/openbao/openbao/api/v2'
VERSION = 'v2.7.0'
SUM = 'h1:' + base64.b64encode(bytes(range(32))).decode()
MODSUM = 'h1:' + base64.b64encode(bytes(reversed(range(32)))).decode()


class DependencyCheckTests(unittest.TestCase):
    def load(self):
        self.assertTrue(SCRIPT.is_file(), 'OB-001 lacks the real-dependency validation entrypoint')
        spec = importlib.util.spec_from_file_location('dependency_check', SCRIPT)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module

    def fixture(self, directory):
        p = pathlib.Path(directory)
        source = p / 'source'
        source.mkdir()
        for name in ('client.go', 'request.go', 'response.go'):
            (source / name).write_text('// synthetic validator fixture, NOT OpenBao source\n')
        mod = p / 'upstream.mod'
        mod.write_text('module ' + MODULE + '\n\ngo 1.23.2\n')
        zipped = p / 'upstream.zip'
        zipped.write_bytes(b'synthetic validator fixture, NOT a module ZIP')
        return dict(Path=MODULE, Version=VERSION, Sum=SUM, GoModSum=MODSUM,
                    GoMod=str(mod), Dir=str(source), Zip=str(zipped))

    def test_error_or_missing_checksum_never_passes(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as tmp:
            record = self.fixture(tmp)
            for changed in ({'Error': 'unavailable'}, {'Sum': ''}, {'GoModSum': 'h1:bad'},
                            {'Version': 'v2.6.0'}, {'Path': 'untrusted.invalid/module'},
                            {'Dir': str(pathlib.Path(tmp) / 'missing')}):
                with self.subTest(fields=tuple(changed)):
                    with self.assertRaises(ValueError):
                        m.validate_download({**record, **changed})

    def test_records_real_files_but_does_not_claim_checksum_verification(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as tmp:
            record = self.fixture(tmp)
            result = m.validate_download(record)
            self.assertEqual(result['module'], MODULE)
            self.assertEqual(result['version'], VERSION)
            self.assertEqual(result['sum'], SUM)
            self.assertEqual(set(result['source_sha256']), {'client.go', 'request.go', 'response.go'})
            self.assertEqual(len(result['go_mod_sha256']), 64)
            self.assertNotIn('verified', result)  # Go commands, not this parser, verify sums.

    def test_symlink_or_missing_source_is_rejected(self):
        m = self.load()
        with tempfile.TemporaryDirectory() as tmp:
            record = self.fixture(tmp)
            source = pathlib.Path(record['Dir']) / 'client.go'
            source.unlink()
            with self.assertRaises(ValueError):
                m.validate_download(record)
            source.symlink_to(record['GoMod'])
            with self.assertRaises(ValueError):
                m.validate_download(record)

    def test_go_sum_must_bind_both_direct_records(self):
        m = self.load()
        record = dict(Sum=SUM, GoModSum=MODSUM)
        good = f'{MODULE} {VERSION} {SUM}\n{MODULE} {VERSION}/go.mod {MODSUM}\n'
        m.validate_sums(good, record)
        for raw in ('', good.splitlines()[0] + '\n', good.replace(MODSUM, SUM)):
            with self.subTest(raw_length=len(raw)):
                with self.assertRaises(ValueError):
                    m.validate_sums(raw, record)

    def test_module_graph_replacements_and_missing_pin_are_rejected(self):
        m = self.load()
        upstream = dict(Path=MODULE, Version=VERSION)
        root = {'Path': 'git.example.com/infra/openbao-sdk-go', 'Main': True}
        m.validate_graph([root, upstream])
        for graph in ([root], [root, {**upstream, 'Version': 'v2.6.0'}],
                      [root, {**upstream, 'Replace': {'Dir': '/tmp/fake'}}],
                      [root, upstream, {'Path': 'dependency.invalid/x', 'Replace': {}}],
                      [root, {**upstream, 'Error': {'Err': 'unavailable'}}]):
            with self.subTest(graph_length=len(graph)):
                with self.assertRaises(ValueError):
                    m.validate_graph(graph)

    def test_root_module_configuration_and_minimum_go_are_not_guessed(self):
        m = self.load()
        parsed = {'Module': {'Path': MODULE}, 'Go': '1.23.2'}
        self.assertEqual(m.validate_upstream_mod(parsed), '1.23.2')
        for value in ({**parsed, 'Go': ''}, {**parsed, 'Go': 'latest'},
                      {**parsed, 'Module': {'Path': 'other.invalid/module'}}):
            with self.assertRaises(ValueError):
                m.validate_upstream_mod(value)
        root = {'Require': [{'Path': MODULE, 'Version': VERSION}], 'Replace': None}
        m.validate_root(root)
        for value in ({'Require': []}, {**root, 'Replace': [{'Old': {'Path': MODULE}}]},
                      {'Require': [{'Path': MODULE, 'Version': 'v2.6.0'}]}):
            with self.assertRaises(ValueError):
                m.validate_root(value)

    def test_preflight_cannot_inherit_overlay_or_checksum_bypass(self):
        m = self.load()
        original = {'GOENV': '/tmp/custom-goenv', 'GOWORK': '/tmp/go.work',
                    'GOFLAGS': '-overlay=/tmp/fake.json', 'GOSUMDB': 'off',
                    'GONOSUMDB': '*', 'GOINSECURE': '*', 'GOPRIVATE': '*',
                    'GOPROXY': 'https://proxy.golang.org', 'GOTOOLCHAIN': 'auto'}
        env = m.preflight_environment(original)
        self.assertEqual(env['GOENV'], 'off')
        self.assertEqual(env['GOWORK'], 'off')
        self.assertEqual(env['GOSUMDB'], 'sum.golang.org')
        self.assertEqual(env['GOTOOLCHAIN'], 'local')
        for name in ('GOFLAGS', 'GONOSUMDB', 'GOPRIVATE', 'GOINSECURE'):
            self.assertEqual(env[name], '')
        self.assertEqual(original['GOSUMDB'], 'off')  # no caller environment mutation
        self.assertEqual(env['GOPROXY'], original['GOPROXY'])

    def test_decode_concatenated_go_json_and_reject_truncated_output(self):
        m = self.load()
        records = [{'Path': 'a'}, {'Path': 'b'}]
        raw = '\n'.join(json.dumps(x) for x in records)
        self.assertEqual(m.decode_stream(raw), records)
        for value in ('', '{}\n{', '[]', '{"Path":"a"} trailing text'):
            with self.assertRaises(ValueError):
                m.decode_stream(value)


if __name__ == '__main__':
    unittest.main()
