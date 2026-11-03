from pathlib import Path
import re

root = Path(__file__).parent
source = Path('/tmp/cc_code.txt').read_text(encoding='utf-8', errors='replace')
target = root / 'tools'
target.mkdir(exist_ok=True)


def capture(start, end):
    left = source.index(start)
    right = source.index(end, left)
    line = source.count('\n', 0, left) + 1
    col = left - source.rfind('\n', 0, left) - 1
    return f'C{line}:{col}\n{source[left:right]}\n'


factories = [
    ('var oOr=p(()=>He({file_path:', ',mRn=p('),
    ('var bbe=p(()=>He({file_path:', ',hZ=p('),
    ('var LXt=p(()=>He({file_path:', ',FXt=p('),
    ('var rUo=p(()=>He({pattern:', ',sUo=p('),
    ('var t9o=p(()=>u({query:', ',n9o=p('),
    ('var Ja=p(()=>He({subject:', ',el=p('),
    ('var tl=p(()=>He({taskId:', ',ol=p('),
    ('var rl=p(()=>{let e=Dwe()', ',nl=p('),
    ('var sl=p(()=>He({}))', ',il=p('),
]
(target / 'schema-factories.source.txt').write_text(
    '\n'.join(capture(start, end) for start, end in factories), encoding='utf-8'
)


def read_template(anchor):
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


for name, anchor in [('task-get', 'En=`Use this tool to retrieve a task'),
                     ('task-update', 'An=`Use this tool to update a task')]:
    prefix = anchor.split('`', 1)[1]
    text = prefix + read_template(anchor)
    (target / f'{name}-description.source.txt').write_text(text, encoding='utf-8')
    text = re.sub(r'\\u([0-9a-fA-F]{4})', lambda m: chr(int(m.group(1), 16)), text)
    text = text.replace('\\`', '`')
    if '\\' in text or '${' in text:
        raise ValueError(f'unresolved escape or interpolation in {name}')
    (target / f'{name}-description.txt').write_text(text, encoding='utf-8')
print('wrote 9 schema factories and 2 resolved task descriptions')
