from pathlib import Path
import re

code_path = Path('/tmp/cc_code.txt')
strings_path = Path('/tmp/cc_strings.txt')
baseline = Path('.project/research/claude-code-cli/2.1.310/findings/04-undocumented-apis.md')

code = code_path.read_text(encoding='utf-8', errors='replace')
string_lines = strings_path.read_text(encoding='utf-8', errors='replace').splitlines()
normalized = [re.sub(r'^\d+\t', '', line) for line in string_lines]
print('2.1.317 code lines:', code.count('\n'))
print('2.1.317 code run prefixes:', len(re.findall(r'^@\d+@', code, re.M)))
print('2.1.317 string runs:', len(string_lines))
print('2.1.317 unprefixed string runs:', sum(not re.match(r'^\d+\t', line) for line in string_lines))
print('2.1.317 distinct normalized string runs:', len(set(normalized)))
text = baseline.read_text(encoding='utf-8')
marker = 'Literal API beta vocabulary recovered (50 distinct values; registry + SDK/call-site extras):\n'
old_line = text.split(marker, 1)[1].splitlines()[0]
old = set(re.findall(r'`([^`]+)`', old_line))
missing = sorted(value for value in old if value not in code)
start = code.index('m7t=k("claude_code","claude-code-20250219")')
end = code.index('Igs=Object.freeze', start)
registry = set(re.findall(r'k\("[^"\n]+","([^"\n]+)"\)', code[start:end]))
new = sorted(value for value in registry - old if not value.startswith('x-cc-internal-'))
print('2.1.310 documented beta literals:', len(old))
print('2.1.310 literals absent from 2.1.317 code:', missing)
print('2.1.317 explicit registry literals not in 2.1.310 inventory:', new)
