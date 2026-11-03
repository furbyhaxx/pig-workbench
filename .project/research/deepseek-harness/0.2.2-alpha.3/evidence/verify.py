#!/usr/bin/env python3
"""Check dossier metadata, cited paths, inventory rows and verbatim source excerpts."""
from datetime import datetime
from pathlib import Path
import re
import sys

import yaml

ROOT = Path(__file__).resolve().parent.parent
SOURCE = Path('/tmp/re-recon/dsh')
CHAPTERS = [
    '04-instructions-and-injections.md',
    '05-modes-and-approvals.md',
    '06-skills-and-commands.md',
    '07-session-and-compaction.md',
]
EXCERPTS = [
    'injection-templates.txt',
    'state-templates.txt',
    'system-section-templates.txt',
]
errors = []

for name in CHAPTERS:
    path = ROOT / name
    if not path.is_file():
        errors.append(f'missing chapter: {name}')
        continue
    content = path.read_text()
    if not content.endswith('\n') or content.endswith('\n\n'):
        errors.append(f'{name}: incorrect final newline')
    parts = content.split('---\n', 2)
    if len(parts) != 3 or parts[0] != '':
        errors.append(f'{name}: invalid frontmatter delimiters')
        continue
    document = yaml.safe_load(parts[1])['document']
    required = {
        'type': 'research',
        'id': 'RES-deepseek-harness-0.2.2-alpha.3-' + name[:-3],
        'status': 'published',
        'revision': 1,
        'created_by': 'worker',
        'updated_by': 'worker',
    }
    for key, value in required.items():
        if document.get(key) != value:
            errors.append(f'{name}: incorrect {key}')
    for key in ('created_at', 'updated_at'):
        stamp = datetime.fromisoformat(document[key])
        if stamp.utcoffset() is None:
            errors.append(f'{name}: {key} lacks timezone')
    changelog = document.get('changelog', [])
    if len(changelog) != 1 or changelog[0].get('revision') != 1 or changelog[0].get('reason') != 'document creation' or not changelog[0].get('changes'):
        errors.append(f'{name}: incorrect revision-one changelog')
print(f'frontmatter: {len(CHAPTERS)} chapters checked')

citations = 0
for path in [*(ROOT / name for name in CHAPTERS), ROOT / 'evidence/injection-inventory.tsv']:
    for match in re.finditer(r'\b(packages/[A-Za-z0-9_./-]+\.ts)(?::(\d+))?', path.read_text()):
        relative, line = match.groups()
        target = SOURCE / relative
        if not target.is_file():
            errors.append(f'{path.name}: missing source file {relative}')
        elif line is not None:
            citations += 1
            if not 1 <= int(line) <= len(target.read_text().splitlines()):
                errors.append(f'{path.name}: invalid source line {relative}:{line}')
print(f'source citations: {citations} explicit path:line references checked')

count = 0
for name in EXCERPTS:
    text = (ROOT / 'evidence' / name).read_text()
    parts = re.split(r'^=== (packages/[^:\n]+):(\d+)-(\d+) ===\n', text, flags=re.M)
    for index in range(1, len(parts), 4):
        relative, start, end, recorded = parts[index:index + 4]
        lines = (SOURCE / relative).read_text().splitlines()
        original = '\n'.join(lines[int(start)-1:int(end)]).rstrip('\n')
        if original != recorded.rstrip('\n'):
            errors.append(f'{name}: excerpt differs from {relative}:{start}-{end}')
        count += 1
print(f'verbatim evidence: {count} source ranges compared')

rows = (ROOT / 'evidence/injection-inventory.tsv').read_text().splitlines()
for number, row in enumerate(rows, 1):
    if len(row.split('\t')) != 6:
        errors.append(f'inventory row {number}: expected six columns')
print(f'injection inventory: {len(rows)-1} rows, six columns checked')

if errors:
    print('\n'.join(errors))
    print('FAIL')
    sys.exit(1)
print('PASS')
