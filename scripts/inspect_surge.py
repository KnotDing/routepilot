#!/usr/bin/env python3
"""Read-only schema inspection. Never print profiles, tokens, or rule contents."""
import hashlib
import json
import subprocess

CLI = '/Applications/Surge.app/Contents/Applications/surge-cli'
for args in [('status',), ('dump', 'rule'), ('dump', 'temp-rule'), ('policy-group', 'list')]:
    p = subprocess.run([CLI, '--raw', *args], capture_output=True, timeout=10)
    try:
        data = json.loads(p.stdout)
    except ValueError:
        print(json.dumps({'command': args, 'exit_code': p.returncode, 'structured': False}))
        continue
    def shape(x, depth=0):
        if depth > 3:
            return type(x).__name__
        if isinstance(x, dict):
            return {k: shape(v, depth+1) for k, v in x.items()}
        if isinstance(x, list):
            return {'count': len(x), 'first_shape': shape(x[0], depth+1) if x else None}
        return type(x).__name__
    print(json.dumps({'command': args, 'shape': shape(data), 'digest': hashlib.sha256(p.stdout).hexdigest()}))
