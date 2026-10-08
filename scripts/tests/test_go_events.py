import json
import pathlib
import sys
import unittest

sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]))
from evidence import go_results


class GoEventsTests(unittest.TestCase):
    def events(self, extra):
        base=[{'Action':'run','Package':'sdk','Test':'TestActual'},
              {'Action':'pass','Package':'sdk','Test':'TestActual'}, {'Action':'pass','Package':'sdk'}]
        return '\n'.join(json.dumps(e) for e in base+extra)

    def test_no_test_files_package_is_not_a_skipped_test(self):
        raw=self.events([{'Action':'output','Package':'sdk/types','Output':'?   \tsdk/types\t[no test files]\n'},
                         {'Action':'skip','Package':'sdk/types'}])
        self.assertEqual(go_results(raw),{('sdk','TestActual')})
        fixture=pathlib.Path(__file__).parent/'fixtures/go1.26.3-no-test-files.jsonl'
        self.assertEqual(go_results(self.events([])+'\n'+fixture.read_text(encoding='utf-8')),{('sdk','TestActual')})

    def test_actual_skip_unexplained_package_skip_and_zero_targets_fail(self):
        for extra in [[{'Action':'skip','Package':'sdk','Test':'TestSkipped'}],
                      [{'Action':'skip','Package':'sdk/types'}],
                      [{'Action':'skip','Package':'sdk/types','NoTestFiles':True}]]:
            with self.subTest(extra=extra),self.assertRaises(ValueError):go_results(self.events(extra))
        raw='\n'.join(json.dumps(e) for e in [
            {'Action':'output','Package':'sdk/types','Output':'?   \tsdk/types\t[no test files]\n'},
            {'Action':'skip','Package':'sdk/types'}])
        with self.assertRaises(ValueError):go_results(raw)

    def test_empty_or_null_test_field_is_malformed(self):
        for value in ['',None]:
            raw=self.events([{'Action':'output','Package':'sdk/types','Test':value,'Output':'?   \tsdk/types\t[no test files]\n'},
                             {'Action':'skip','Package':'sdk/types'}])
            with self.subTest(value=value),self.assertRaises(ValueError):go_results(raw)
