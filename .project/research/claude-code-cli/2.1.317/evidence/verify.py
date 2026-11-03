from pathlib import Path
import json
import re

root = Path('.project/research/claude-code-cli/2.1.317')
evidence = root / 'evidence'
code = Path('/tmp/cc_code.txt').read_text(encoding='utf-8', errors='replace')
lines = code.splitlines()

checks = [
    (18027, 25, 'DeferredToolPlaceholder'),
    (19772, 691, "You are Claude Code, Anthropic's official CLI for Claude."),
    (14519, 153318, 'program-use-case-ids-2026-09-17'),
    (14519, 153750, 'claude-code-side-query-2026-09-29'),
    (30242, 315, 'En=`Use this tool to retrieve a task'),
    (30264, 47, 'An=`Use this tool to update a task'),
]
for line, offset, literal in checks:
    assert lines[line - 1][offset:].startswith(literal), (line, offset, literal)

chapters = [root / name for name in (
    '09-delta-from-2.1.310.md', '10-system-prompt.md', '11-tool-declarations.md'
)]
reference_count = 0
for path in chapters:
    text = path.read_text(encoding='utf-8')
    assert text.startswith('---\ndocument:\n')
    for field in ('type: "research"', 'status: "published"', 'revision: 1',
                  'created_by: "worker"', 'updated_by: "worker"',
                  'reason: "document creation"'):
        assert field in text, (path, field)
    assert re.search(r'created_at: "\d{4}-\d\d-\d\dT[^" ]+[+-]\d\d:\d\d"', text)
    for line, offset, end in re.findall(r'\bC(\d+):(\d+)(?:[–-](\d+))?', text):
        line, offset = int(line), int(offset)
        end = int(end) if end else offset
        assert 1 <= line <= len(lines) and 0 <= offset <= end < len(lines[line - 1]), (path, line, offset, end)
        reference_count += 1
    for target in re.findall(r'\]\(([^)#]+)\)', text):
        if not target.startswith(('http:', 'https:', 'host:')):
            assert (path.parent / target).is_file(), (path, target)

placeholder = json.loads((evidence / 'tools/deferred-tool-placeholder.json').read_text())
assert placeholder == {
    'name': 'DeferredToolPlaceholder',
    'description': 'Reserved placeholder that keeps deferred tool loading active; never call this tool.',
    'input_schema': {'type': 'object', 'properties': {}},
    'defer_loading': True,
}
assert 'return{name:B6e,description:egr,input_schema:{type:"object",properties:{}},defer_loading:!0}' in code

for path in (evidence / 'system-prompt').glob('*.source.txt'):
    text = path.read_text(encoding='utf-8')
    assert text and text in code, path
for path in (evidence / 'tools').glob('*-description.source.txt'):
    text = path.read_text(encoding='utf-8')
    assert text and text in code, path
    rendered = re.sub(r'\\u([0-9a-fA-F]{4})', lambda m: chr(int(m.group(1), 16)), text).replace('\\`', '`')
    assert rendered == path.with_name(path.name.replace('.source.txt', '.txt')).read_text(encoding='utf-8')

for name, variable in [('proactive', '${LRo}'), ('concise', '${$Ro}')]:
    template = (evidence / f'system-prompt/{name}.source.txt').read_text()
    guidance = (evidence / f'system-prompt/{name}-guidance.source.txt').read_text()
    rendered = re.sub(r'\\u([0-9a-fA-F]{4})', lambda m: chr(int(m.group(1), 16)), template.replace(variable, guidance))
    assert rendered == (evidence / f'system-prompt/{name}.txt').read_text()

for name in ('explanatory', 'learning'):
    template = (evidence / f'system-prompt/{name}.source.txt').read_text()
    guidance = (evidence / 'system-prompt/insights-guidance.source.txt').read_text()
    for branch, star in [('unicode', chr(0x2605)), ('fallback', chr(0x2736))]:
        text = template.replace('${sUt}', guidance).replace('${re.star}', star).replace('${re.bullet}', chr(0x25CF))
        text = re.sub(r'\\u([0-9a-fA-F]{4})', lambda m: chr(int(m.group(1), 16)), text).replace('\\`', '`')
        assert text == (evidence / f'system-prompt/{name}-{branch}.txt').read_text()

for item in re.split(r'\n\n(?=C\d+:\d+\n)', (evidence / 'tools/schema-factories.source.txt').read_text()):
    address, snippet = item.rstrip('\n').split('\n', 1)
    assert snippet in code, address

expected_names = {
    'print': ['Agent', 'Bash', 'Edit', 'ListAgents', 'Read', 'ReportFindings',
              'ScheduleWakeup', 'Skill', 'ToolSearch', 'Workflow', 'DeferredToolPlaceholder', 'Write'],
    'interactive': ['Agent', 'AskUserQuestion', 'Bash', 'Edit', 'ListAgents', 'Read', 'ReportFindings',
                    'ScheduleWakeup', 'Skill', 'ToolSearch', 'Workflow', 'DeferredToolPlaceholder', 'Write'],
}
for mode, names in expected_names.items():
    body = json.loads((evidence / f'wire/{mode}-request.json').read_text())
    assert [tool['name'] for tool in body['tools']] == names
    assert json.loads((evidence / f'tools/{mode}-tools.json').read_text()) == body['tools']
    assert json.loads((evidence / f'system-prompt/{mode}-system.json').read_text()) == body['system']
    assert len(body['system']) == 3
    standard_intro = (evidence / 'system-prompt/intro-default.txt').read_text()
    lean_prefix = standard_intro.split('\nIMPORTANT: You must NEVER', 1)[0].replace(
        ' Use the instructions below and the tools available to you to assist the user.', '') + '\n\n# Harness'
    assert body['system'][2]['text'].startswith(lean_prefix)
    for i, block in enumerate(body['system']):
        assert (evidence / f'system-prompt/{mode}-{i:02}.txt').read_bytes() == block['text'].encode('utf-8')
    for i, tool in enumerate(body['tools']):
        declaration = evidence / f'tools/{mode}/{i:02}-{tool["name"]}.json'
        assert json.loads(declaration.read_text()) == tool
    assert body['tools'][-2] == placeholder
    assert 'offline-capture-dummy' not in json.dumps(body)
    assert str(Path.home()) not in json.dumps(body)
    assert 'Say hello.' in json.dumps(body)

for mode, count in [('print-beta', 8), ('interactive-beta', 11)]:
    headers = json.loads((evidence / f'wire/{mode}-headers.json').read_text())
    body = json.loads((evidence / f'wire/{mode}-request.json').read_text())
    assert len(headers['anthropic-beta'].split(',')) == count
    assert [t['name'] for t in body['tools']] == expected_names[mode.split('-')[0]]
    assert set(headers).issubset({'anthropic-beta', 'anthropic-version', 'user-agent', 'x-app'})

print(f'PASS: {len(chapters)} frontmatter blocks, {reference_count} valid citation offsets and all Markdown links')
print('PASS: placeholder schema, 9 prompt source templates, 9 schema factories and 2 tool descriptions')
print('PASS: all 6 resolved output-style strings match source-template substitutions')
print('PASS: captured 12/13 ordered tools and 3+3 system blocks match all extracted files')
print('PASS: two additional beta header captures contain 8/11 entries and matching tool names')
