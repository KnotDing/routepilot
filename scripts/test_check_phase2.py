import copy
import unittest
from check_phase2 import check


class CorrelationTests(unittest.TestCase):
    def setUp(self):
        self.journal = [{'kind': 'start', 'run_id': 'fresh', 'payload': {'source4': '192.0.2.2'}}]
        self.probes = []
        for index, protocol in enumerate(('udp', 'tcp')):
            self.probes.append({'protocol': protocol, 'destination': '8.8.8.8:53',
                                'id': 10 + index, 'domain': 'example.com.', 'rcode': 0, 'answers': ['A']})
            self.journal.append({'kind': 'query', 'run_id': 'fresh', 'payload': {
                'id': 10 + index, 'rcode': 0, 'answer_count': 1, 'query': {
                    'transport': protocol, 'client_ip': '192.0.2.50', 'intercepted': True,
                    'original_destination_ip': '8.8.8.8', 'original_destination_port': 53,
                    'question': {'Name': 'example.com.'}}}})

    def test_valid(self):
        self.assertTrue(check(self.journal, self.probes, '192.0.2.50')['correlated_udp_tcp'])

    def test_direct_reply_does_not_prove_interception(self):
        self.journal[1]['payload']['query']['intercepted'] = False
        with self.assertRaises(ValueError):
            check(self.journal, self.probes, '192.0.2.50')

    def test_stale_duplicate_or_wrong_original_rejected(self):
        for change in ('stale', 'duplicate', 'destination'):
            journal = copy.deepcopy(self.journal)
            if change == 'stale':
                journal[1]['run_id'] = 'older'
            elif change == 'duplicate':
                journal.append(copy.deepcopy(journal[1]))
            else:
                journal[1]['payload']['query']['original_destination_ip'] = '1.1.1.1'
            with self.assertRaises(ValueError):
                check(journal, self.probes, '192.0.2.50')


if __name__ == '__main__':
    unittest.main()
