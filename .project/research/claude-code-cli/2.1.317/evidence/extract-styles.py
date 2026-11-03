from pathlib import Path
import re

root = Path(__file__).parent
source = Path('/tmp/cc_code.txt').read_text(encoding='utf-8', errors='replace')
target = root / 'system-prompt'
target.mkdir(exist_ok=True)


def template_after(anchor):
    start = source.index(anchor) + len(anchor)
    end = start
    while end < len(source):
        if source[end] == '\\':
            end += 2
        elif source[end] == '`':
            return source[start:end]
        else:
            end += 1
    raise ValueError(f'unterminated template: {anchor}')


def decode_simple(text):
    text = re.sub(r'\\u([0-9a-fA-F]{4})', lambda m: chr(int(m.group(1), 16)), text)
    text = text.replace('\\`', '`')
    if '\\' in text or '${' in text:
        raise ValueError('unresolved escape or interpolation')
    return text


def style_at(prompt):
    start = prompt + len('prompt:`')
    end = start
    while end < len(source):
        if source[end] == '\\':
            end += 2
        elif source[end] == '`':
            return source[start:end]
        else:
            end += 1
    raise ValueError('unterminated style template')


parts = {
    'insights-guidance.source.txt': template_after('var sUt=`'),
    'proactive-guidance.source.txt': template_after('LRo=`'),
    'concise-guidance.source.txt': template_after('$Ro=`'),
    'lean-body.source.txt': template_after('if(s)return jEi(e==="output-style"?$dn:WEi,h);return`'),
}
for name in ('Proactive', 'Concise', 'Explanatory', 'Learning'):
    start = source.index(f'{name}:{{name:"{name}"')
    parts[f'{name.lower()}.source.txt'] = style_at(source.index('prompt:`', start))
for filename, text in parts.items():
    (target / filename).write_text(text, encoding='utf-8')
coordinator = 'You are Claude Code, an AI assistant that orchestrates software engineering tasks across multiple workers.'
(target / 'coordinator.source.txt').write_text(
    coordinator + template_after('return`' + coordinator), encoding='utf-8'
)
identities = {
    'identity-cli.txt': "You are Claude Code, Anthropic's official CLI for Claude.",
    'identity-sdk-cli.txt': "You are Claude Code, Anthropic's official CLI for Claude, running within the Claude Agent SDK.",
    'identity-agent-sdk.txt': "You are a Claude agent, built on Anthropic's Claude Agent SDK.",
}
for filename, text in identities.items():
    if '"' + text + '"' not in source:
        raise ValueError(f'identity not found: {filename}')
    (target / filename).write_text(text, encoding='utf-8')
security = re.search(r'var cpr="([^"\\\\]*)";', source)
opening = re.search(r'var Apr="([^"\\\\]*)",\$dn=\'([^\'\\\\]*)\';', source)
if not security or not opening:
    raise ValueError('intro fragments not found')
intro = template_after('function Ppr(e){return`')
for name, first in [('default', opening.group(1)), ('output-style', opening.group(2))]:
    text = intro.replace('${Hdn(e)}', first).replace('${cpr}', security.group(1))
    (target / f'intro-{name}.txt').write_text(decode_simple(text), encoding='utf-8')
for name, var, filename in (
    ('proactive', '${LRo}', 'proactive-guidance.source.txt'),
    ('concise', '${$Ro}', 'concise-guidance.source.txt'),
):
    text = parts[f'{name}.source.txt'].replace(var, parts[filename])
    (target / f'{name}.txt').write_text(decode_simple(text), encoding='utf-8')
for literal in (r'bullet:"\u25CF"', r'star:"\u2605"', r'star:"\u2736"'):
    if literal not in source:
        raise ValueError(f'glyph constant not found: {literal}')
for name in ('explanatory', 'learning'):
    for branch, star in [('unicode', chr(0x2605)), ('fallback', chr(0x2736))]:
        text = parts[f'{name}.source.txt'].replace('${sUt}', parts['insights-guidance.source.txt'])
        text = text.replace('${re.star}', star).replace('${re.bullet}', chr(0x25CF))
        (target / f'{name}-{branch}.txt').write_text(decode_simple(text), encoding='utf-8')
print('wrote 9 source templates, 6 resolved style prompts, 2 intros, and 3 identities')
