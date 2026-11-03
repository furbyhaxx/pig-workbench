from pathlib import Path
import re

import yaml

ROOT = Path(__file__).resolve().parent.parent
SOURCE = Path('/tmp/re-recon/codex-tag/codex-rs')
EVIDENCE = ROOT / 'evidence'
CHAPTERS = sorted(ROOT.glob('0[4567]-*.md'))
MARKDOWN = CHAPTERS + sorted(EVIDENCE.glob('0[4567]-*.md'))

for path in MARKDOWN:
    text = path.read_text()
    assert text.startswith('---\n'), path
    frontmatter = text.split('\n---\n', 1)[0][4:]
    document = yaml.safe_load(frontmatter)['document']
    for key in ('type', 'id', 'title', 'description', 'status', 'revision', 'created_at',
                'updated_at', 'created_by', 'updated_by', 'changelog'):
        assert key in document, (path, key)
    assert document['type'] == 'research' and document['status'] == 'published', path
    assert document['revision'] == 1, path
    assert document['created_by'] == document['updated_by'] == 'worker', path
    assert document['changelog'][0]['reason'] == 'document creation', path
print(f'PASS: frontmatter for {len(MARKDOWN)} Markdown files')

for path in (ROOT / '05-modes-and-approvals.md', EVIDENCE / '05-permission-templates.md'):
    pattern = r'#{2,3} `([^`]+)`\n\n~~~text\n(.*?)~~~\n'
    blocks = re.findall(pattern, path.read_text(), flags=re.DOTALL)
    assert len(blocks) == 7, (path, len(blocks))
    for relative, block in blocks:
        assert block == (SOURCE / relative).read_text(), (path, relative)
print('PASS: 7 permission templates match source in chapter and evidence')

path = EVIDENCE / '07-mechanism-templates.md'
pattern = r'## `([^`]+)`( \(source has no final newline\))?\n\n~~~~~~~~text\n(.*?)~~~~~~~~\n'
blocks = re.findall(pattern, path.read_text(), flags=re.DOTALL)
assert len(blocks) == 8, len(blocks)
for relative, no_newline, block in blocks:
    source = (SOURCE / relative).read_text()
    assert block == source + ('\n' if no_newline else ''), relative
    assert bool(no_newline) == (not source.endswith('\n')), relative
print('PASS: 8 mechanism templates match source')

count = 0
for path in (EVIDENCE / '04-context-builders.txt', EVIDENCE / '04-additional-builders.txt',
             EVIDENCE / '06-skills-builders.txt'):
    pattern = r'===== BEGIN ([^\n]+) =====\n(.*?)===== END \1 =====\n'
    blocks = re.findall(pattern, path.read_text(), flags=re.DOTALL)
    assert blocks, path
    for relative, block in blocks:
        source = (SOURCE / relative).read_text()
        assert block == source + ('' if source.endswith('\n') else '\n'), relative
        count += 1
print(f'PASS: {count} captured Rust sources match source')

chapter = (ROOT / '04-instructions-and-injections.md').read_text()
inventory = (EVIDENCE / '04-injection-inventory.md').read_text()
tables = lambda text: [line for line in text.splitlines() if line.startswith('|')]
assert tables(chapter) == tables(inventory)
print(f'PASS: inventory matches chapter ({sum(line.startswith("| Fragment |") for line in tables(chapter))} tables)')

count = 0
pattern = r'(?<![\w/])([\w./-]+\.(?:rs|py|md)):([0-9]+(?:-[0-9]+)?(?:,[0-9]+(?:-[0-9]+)?)*)'
for chapter in CHAPTERS:
    for match in re.finditer(pattern, chapter.read_text()):
        relative, ranges = match.groups()
        path = SOURCE / relative
        assert path.is_file(), (chapter.name, relative)
        lines = len(path.read_text().splitlines())
        for entry in ranges.split(','):
            first, *last = entry.split('-')
            assert 1 <= int(first) <= lines, (relative, first, lines)
            assert not last or int(first) <= int(last[0]) <= lines, (relative, last, lines)
        count += 1
print(f'PASS: {count} source citation paths and line ranges exist')
