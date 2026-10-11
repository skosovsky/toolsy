#!/usr/bin/env python3
"""Hash candidate files including deleted paths; audit outputs are excluded."""
import hashlib
import pathlib
import subprocess

root = pathlib.Path(__file__).resolve().parents[3]
paths = set(subprocess.check_output(['git', 'ls-files', '-co', '--exclude-standard'], cwd=root).decode().splitlines())
paths.update(['.cursor/tasks/mcp-official-sdk-integration.md', '.cursor/docs/mcp.md'])
h = hashlib.sha256()
for name in sorted(paths):
    if name.startswith('docs/reviews/mcp-sdk-integration/'):
        continue
    p = root / name
    h.update(name.encode() + b'\0')
    h.update(hashlib.sha256(p.read_bytes()).digest() if p.is_file() else b'DELETED')
    h.update(b'\0')
print(h.hexdigest())
