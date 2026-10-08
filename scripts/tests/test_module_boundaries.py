"""Dependency direction includes imports inside functions."""
import ast
from pathlib import Path
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]


class ModuleBoundaryTests(unittest.TestCase):
    def test_foundations_do_not_import_acceptance_or_runners(self):
        for name in ('tooling.py', 'command_runner.py'):
            tree = ast.parse((SCRIPTS / name).read_text(encoding='utf-8'))
            imports = set()
            for node in ast.walk(tree):
                if isinstance(node, ast.ImportFrom):
                    imports.add(node.module)
                elif isinstance(node, ast.Import):
                    imports.update(alias.name for alias in node.names)
            with self.subTest(module=name):
                self.assertFalse(imports & {'evidence', 'reporting', 'ci_export'}, imports)

    def test_test_modules_do_not_import_other_test_cases(self):
        for path in (SCRIPTS / 'tests').glob('test_*.py'):
            tree = ast.parse(path.read_text(encoding='utf-8'))
            for node in ast.walk(tree):
                if isinstance(node, ast.Import):
                    imports = [alias.name for alias in node.names]
                elif isinstance(node, ast.ImportFrom):
                    imports = [node.module or '']
                else:
                    continue
                with self.subTest(module=path.name, line=node.lineno):
                    self.assertFalse(any(name.split('.')[-1].startswith('test_') for name in imports), imports)


if __name__ == '__main__':
    unittest.main()
