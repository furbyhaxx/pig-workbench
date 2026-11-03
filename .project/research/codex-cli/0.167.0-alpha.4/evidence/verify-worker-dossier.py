#!/usr/bin/env python3
from pathlib import Path
import json
import re
import sys

import yaml

root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path('/tmp/re-recon/codex-tag/codex-rs')
dossier = Path(__file__).resolve().parent.parent
errors = []


def check(condition, message):
    if not condition:
        errors.append(message)


markdown = [dossier / name for name in [
    '00-context.md',
    '01-architecture.md',
    '02-system-prompt.md',
    '03-tool-declarations.md',
    'evidence/prompt/base_instructions-default.md',
]]
for path in markdown:
    chunks = path.read_text().split('---\n', 2)
    check(len(chunks) == 3 and chunks[0] == '', f'frontmatter delimiter: {path}')
    if len(chunks) != 3:
        continue
    front = yaml.safe_load(chunks[1])['document']
    for key in ['type', 'id', 'title', 'description', 'status', 'revision',
                'created_at', 'updated_at', 'created_by', 'updated_by', 'changelog']:
        check(key in front, f'{path}: missing {key}')
    check(front['type'] == 'research' and front['status'] == 'published' and front['revision'] >= 1,
          f'frontmatter values: {path}')
    check(front['created_by'] == 'worker' and front['updated_by'] in ('worker', 'orchestrator'),
          f'frontmatter author: {path}')
    check(front['changelog'][0]['reason'] == 'document creation' and bool(front['changelog'][0]['changes']),
          f'changelog: {path}')
    check(any(e.get('revision') == front['revision'] for e in front['changelog'] if isinstance(e, dict)),
          f'changelog has no entry for revision {front["revision"]}: {path}')
print(f'frontmatter: checked {len(markdown)} owned Markdown files')

base = markdown[-1].read_bytes().split(b'---\n', 2)[2]
check(base == (root / 'protocol/src/prompts/base_instructions/default.md').read_bytes(),
      'fallback body differs')
print('fallback prompt: body byte-identical to protocol source')

count = 0
for folder in ['prompts/templates', 'collaboration-mode-templates/templates',
               'ext/goal/templates', 'ext/memories/templates']:
    for path in (root / folder).rglob('*'):
        if not path.is_file():
            continue
        dest = dossier / 'evidence/prompt' / (str(path.relative_to(root)) + '.txt')
        check(dest.is_file() and dest.read_bytes() == path.read_bytes(),
              f'template differs or missing: {path}')
        count += 1
check((dossier / 'evidence/prompt/unknown-model-fallback.txt').read_bytes()
      == (root / 'models-manager/prompt.md').read_bytes(), 'unknown-model fallback differs')
print(f'templates: {count} byte-identical copies; unknown-model fallback identical')

models = json.loads((root / 'models-manager/models.json').read_text())['models']
by_slug = {model['slug']: model for model in models}
index = (dossier / 'evidence/prompt/index.txt').read_text().splitlines()
catalog_source_lines = (root / 'models-manager/models.json').read_text().splitlines()
catalog_groups = 0
catalog_slugs = 0
for line in index:
    if not line.startswith('evidence/prompt/catalog-'):
        continue
    name = line.split(' <- ', 1)[0].removeprefix('evidence/prompt/')
    contents = (dossier / 'evidence/prompt' / name).read_bytes()
    for number, slug in re.findall(r'models-manager/models\.json:(\d+) \(([^)]+)\)', line):
        expected = by_slug[slug]['model_messages']['instructions_template'].encode()
        check(contents == expected, f'catalog prompt differs: {slug}')
        check('"instructions_template"' in catalog_source_lines[int(number) - 1],
              f'catalog prompt source line invalid: {slug}:{number}')
        catalog_slugs += 1
    catalog_groups += 1
print(f'catalog bases: {catalog_groups} distinct copies checked against {catalog_slugs} slugs')
other = json.loads((dossier / 'evidence/catalog-nonbase-messages.json').read_text())
for model in models:
    expected = dict(model.get('model_messages') or {})
    expected.pop('instructions_template', None)
    check(other[model['slug']] == expected, f'catalog messages differ: {model["slug"]}')
print(f'catalog nonbase messages: {len(models)} models matched')

source_cache = {}
line_count = 0
for name in ['tool-spec-source-dump.txt', 'tool-spec-source-supplement.txt',
             'tool-spec-message-board.txt']:
    source = None
    for line in (dossier / 'evidence' / name).read_text().splitlines():
        if line.startswith('===== ') and line.endswith(' ====='):
            source = line[6:-6]
            continue
        if source is None or not line.startswith(source + ':'):
            continue
        number, text = line[len(source) + 1:].split(': ', 1)
        if source not in source_cache:
            source_cache[source] = (root / source).read_text().splitlines()
        check(source_cache[source][int(number) - 1] == text,
              f'source quote mismatch: {source}:{number}')
        line_count += 1
print(f'tool source dumps: {line_count} quoted lines checked against {len(source_cache)} source files')

pattern = re.compile(r'(?<![\w./-])([\w./-]+\.(?:rs|json|md|lark|toml|snap)):(\d+)(?:-(\d+))?')
references = 0
for path in markdown[:4]:
    for match in pattern.finditer(path.read_text()):
        source = root / match.group(1)
        if not source.is_file():
            errors.append(f'{path.name}: missing source {match.group(0)}')
            continue
        end = int(match.group(3) or match.group(2))
        references += 1
        check(end <= len(source.read_bytes().splitlines()),
              f'{path.name}: line out of range {match.group(0)}')
print(f'chapter source citations: {references} paths/ranges checked')

links = 0
for path in markdown[:4]:
    for target in re.findall(r'\]\(([^)]+)\)', path.read_text()):
        if target.startswith(('http://', 'https://', '#')):
            continue
        links += 1
        check((path.parent / target.split('#', 1)[0]).is_file(),
              f'{path.name}: broken link {target}')
print(f'chapter relative links: {links} checked')

if errors:
    print(f'FAIL: {len(errors)} errors')
    for error in errors:
        print(' -', error)
    sys.exit(1)
print('PASS: all checks')
