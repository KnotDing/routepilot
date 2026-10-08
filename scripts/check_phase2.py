#!/usr/bin/env python3
"""Correlate a LAN probe transcript with an actual Controller PF journal.

This does NOT certify gateway path selection or replace pcap review.
"""
import argparse
import ipaddress
import json
import sys


def check(journal, probes, client):
    client = str(ipaddress.ip_address(client))
    starts = [x for x in journal if x.get('kind') == 'start']
    if len(starts) != 1 or not starts[0].get('run_id'):
        raise ValueError('journal must contain exactly one fresh Controller run')
    run_id = starts[0]['run_id']
    records = [x for x in journal if x.get('kind') == 'query' and x.get('run_id') == run_id]
    if {p.get('protocol') for p in probes} != {'udp', 'tcp'} or len(probes) != 2:
        raise ValueError('one successful LAN transaction in each protocol is required')
    for probe in probes:
        if probe.get('error') or probe.get('rcode') != 0 or not probe.get('answers'):
            raise ValueError('LAN probe did not receive a successful DNS answer')
        destination = probe['destination']
        host, port = destination.rsplit(':', 1)
        host = str(ipaddress.ip_address(host.strip('[]')))
        if int(port) != 53:
            raise ValueError('probe did not target original resolver port 53')
        matches = []
        for record in records:
            payload = record['payload']
            context = payload.get('query', {})
            question = context.get('question', {})
            if (payload.get('id') == probe['id'] and context.get('transport') == probe['protocol']
                    and question.get('Name', '').lower() == probe['domain'].lower()
                    and context.get('client_ip') == client):
                matches.append(payload)
        if len(matches) != 1:
            raise ValueError('LAN transaction must match exactly one Controller query')
        matched = matches[0]
        context = matched['query']
        if (not context.get('intercepted') or context.get('original_destination_ip') != host
                or context.get('original_destination_port') != 53):
            raise ValueError('PF did not preserve the original non-local resolver')
        if matched.get('rcode') != 0 or matched.get('answer_count', 0) == 0:
            raise ValueError('Controller did not successfully resolve the intercepted query')
    source = starts[0]['payload'].get('source4')
    for record in records:
        if record['payload']['query'].get('intercepted') and record['payload']['query'].get('client_ip') == source:
            raise ValueError('self-originated traffic was delivered to transparent DNS')
    return {'run_id': run_id, 'correlated_udp_tcp': True,
            'packet_capture_and_gateway_policy_review_required': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--journal', required=True)
    parser.add_argument('--probe', required=True)
    parser.add_argument('--client', required=True)
    args = parser.parse_args()
    try:
        with open(args.journal) as f:
            journal = [json.loads(line) for line in f if line.strip()]
        with open(args.probe) as f:
            probes = [json.loads(line) for line in f if line.strip()]
        print(json.dumps(check(journal, probes, args.client)))
    except (ValueError, OSError, KeyError, TypeError) as error:
        print(f'Acceptance correlation failed: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
