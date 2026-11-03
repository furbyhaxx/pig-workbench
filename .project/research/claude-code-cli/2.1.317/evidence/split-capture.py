from pathlib import Path
import json
import re

root = Path('.project/research/claude-code-cli/2.1.317/evidence')
writes = 0


def put(path, text):
    global writes
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        if path.read_text(encoding='utf-8') != text:
            raise SystemExit(f'refusing to overwrite different evidence: {path}')
        return
    path.write_text(text, encoding='utf-8')
    writes += 1


for mode in ('print', 'interactive'):
    request = json.loads((root / f'wire/{mode}-request.json').read_text(encoding='utf-8'))
    system = request['system']
    put(root / f'system-prompt/{mode}-system.json', json.dumps(system, indent=2, ensure_ascii=False) + '\n')
    put(root / f'tools/{mode}-tools.json', json.dumps(request['tools'], indent=2, ensure_ascii=False) + '\n')
    for i, block in enumerate(system):
        assert block['type'] == 'text' and isinstance(block['text'], str)
        put(root / f'system-prompt/{mode}-{i:02}.txt', block['text'])
    for i, tool in enumerate(request['tools']):
        name = tool.get('name', tool.get('type', 'unknown'))
        assert re.fullmatch(r'[A-Za-z0-9_-]+', name), name
        put(root / f'tools/{mode}/{i:02}-{name}.json', json.dumps(tool, indent=2, ensure_ascii=False) + '\n')
    print(f'{mode}: {len(system)} system blocks, {len(request["tools"])} ordered tool declarations')
print(f'created {writes} evidence files')
