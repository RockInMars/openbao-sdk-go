import json
import pathlib
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0,str(pathlib.Path(__file__).resolve().parents[1]))
import tooling


class IntegrationLockTests(unittest.TestCase):
    def test_pinned_input_must_match_the_checked_in_lock(self):
        env={'BAO_TEST_VERSION':'2.7.0','BAO_TEST_IMAGE':'openbao/openbao@sha256:'+'a'*64}
        with tempfile.TemporaryDirectory() as d, mock.patch.object(tooling,'ROOT',pathlib.Path(d)):
            lock=pathlib.Path(d)/'deploy/test/server-lock.json'
            with self.assertRaisesRegex(ValueError,'lock'):tooling.integration_lock(env)
            lock.parent.mkdir(parents=True)
            data={'status':'PINNED','version':'2.7.0','image':env['BAO_TEST_IMAGE'],'binary_sha256':None}
            lock.write_text(json.dumps(data),encoding='utf-8')
            self.assertEqual(tooling.integration_lock(env)['version'],'2.7.0')
            for field,value in [('status','NOT_VERIFIED'),('version','2.8.0'),('image','openbao/openbao@sha256:'+'b'*64)]:
                bad=dict(data);bad[field]=value;lock.write_text(json.dumps(bad),encoding='utf-8')
                with self.assertRaisesRegex(ValueError,'lock'):tooling.integration_lock(env)


if __name__=='__main__':unittest.main()
