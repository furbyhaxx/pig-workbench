import http.server
import json
import os
from pathlib import Path
import pty
import signal
import subprocess
import sys
import tempfile
import threading
import time

mode = sys.argv[1]
if mode not in ('print', 'interactive', 'print-beta', 'interactive-beta'):
    raise SystemExit('usage: capture-local.py print|interactive|print-beta|interactive-beta')
base_mode = mode.split('-', 1)[0]
root = Path('.project/research/claude-code-cli/2.1.317/evidence').resolve()
binary = Path.home() / '.local/share/claude/versions/2.1.317'
received = []
arrived = threading.Event()


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_HEAD(self):
        self.send_response(200)
        self.end_headers()

    def do_POST(self):
        length = int(self.headers.get('Content-Length', '0'))
        data = self.rfile.read(length)
        if self.path.startswith('/v1/messages'):
            try:
                body = json.loads(data)
            except (UnicodeDecodeError, json.JSONDecodeError):
                body = None
            if isinstance(body, dict):
                headers = {name: self.headers[name] for name in
                           ('anthropic-beta', 'anthropic-version', 'user-agent', 'x-app')
                           if name in self.headers}
                received.append((self.path, body, headers))
                if body.get('tools'):
                    arrived.set()
        response = b'{"type":"error","error":{"type":"invalid_request_error","message":"offline local capture"}}'
        self.send_response(400)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(response)))
        self.end_headers()
        self.wfile.write(response)


with tempfile.TemporaryDirectory(prefix='cc-offline-capture-') as directory:
    home = Path(directory)
    work = home / 'work'
    work.mkdir()
    config = home / 'config'
    config.mkdir()
    (home / '.claude.json').write_text(json.dumps({'hasCompletedOnboarding': True, 'theme': 'dark'}))
    if base_mode == 'interactive':
        (config / '.claude.json').write_text(json.dumps({'hasCompletedOnboarding': True, 'theme': 'dark'}))
    server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    env = {
        'PATH': '/usr/bin:/bin', 'HOME': str(home), 'TERM': 'xterm-256color',
        'XDG_CONFIG_HOME': str(home / 'xdg-config'), 'XDG_CACHE_HOME': str(home / 'xdg-cache'),
        'XDG_STATE_HOME': str(home / 'xdg-state'), 'XDG_DATA_HOME': str(home / 'xdg-data'),
        'CLAUDE_CONFIG_DIR': str(config), 'CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC': '1',
        'DISABLE_TELEMETRY': '1', 'DISABLE_ERROR_REPORTING': '1', 'DISABLE_AUTOUPDATER': '1',
        'ANTHROPIC_API_KEY': 'offline-capture-dummy',
        'ANTHROPIC_BASE_URL': f'http://127.0.0.1:{server.server_port}',
        'NO_PROXY': '*',
    }
    argv = [str(binary)]
    if base_mode == 'print':
        argv += ['-p', 'Say hello.']
        process = subprocess.Popen(argv, cwd=work, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        arrived.wait(35)
    else:
        master, slave = pty.openpty()
        process = subprocess.Popen(argv, cwd=work, env=env, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
        os.set_blocking(master, False)
        deadline = time.monotonic() + 45
        transcript = b''
        sent = 0
        changed_at = 0.0
        trust_seen_at = None
        key_seen_at = None
        while time.monotonic() < deadline and not arrived.is_set() and process.poll() is None:
            try:
                transcript += os.read(master, 65536)
            except BlockingIOError:
                pass
            if b'trust' in transcript.lower() and b'confirm' in transcript.lower() and trust_seen_at is None:
                trust_seen_at = time.monotonic()
            if b'ANTHROPIC_API_KEY' in transcript and key_seen_at is None:
                key_seen_at = time.monotonic()
            if sent == 0 and trust_seen_at is not None and time.monotonic() - trust_seen_at > 3:
                os.write(master, b'\x1b[B')
                sent = 1
                changed_at = time.monotonic()
            elif sent == 1 and time.monotonic() - changed_at > .3:
                os.write(master, b'\r')
                sent = 2
                changed_at = time.monotonic()
            elif sent == 2 and key_seen_at is not None and time.monotonic() - key_seen_at > 3:
                os.write(master, b'\x1b[A')
                sent = 3
                changed_at = time.monotonic()
            elif sent == 3 and time.monotonic() - changed_at > .3:
                os.write(master, b'\r')
                sent = 4
                changed_at = time.monotonic()
            elif sent == 4 and time.monotonic() - changed_at > 5:
                os.write(master, b'Say hello.\r')
                sent = 5
                changed_at = time.monotonic()
            elif sent == 5 and time.monotonic() - changed_at > 5:
                break
            time.sleep(.1)
        os.close(master)
    if process.poll() is None:
        os.killpg(process.pid, signal.SIGTERM)
    try:
        output = process.communicate(timeout=4)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        output = process.communicate()
    server.shutdown()
    server.server_close()
    thread.join()
    selected = next(((path, body, headers) for path, body, headers in received if body.get('tools')), None)
    if selected is None:
        print(f'NO CAPTURE mode={mode} http_messages={len(received)} exit={process.returncode}')
        if base_mode == 'print':
            print('stdout:', (output[0] or b'').decode(errors='replace')[:2000])
            print('stderr:', (output[1] or b'').decode(errors='replace')[:2000])
        else:
            print('terminal_state:', sent, 'prompt_visible:', b'Say hello.' in transcript,
                  'api_key_prompt:', b'ANTHROPIC_API_KEY' in transcript,
                  'not_logged_in:', b'Not logged in' in transcript)
            key_index = transcript.find(b'ANTHROPIC_API_KEY')
            if key_index >= 0:
                print('key_prompt:', transcript[max(0, key_index - 700):key_index + 1800].decode(errors='replace'))
            print('terminal_tail:', transcript.decode(errors='replace')[-500:])
        raise SystemExit(1)
    path, body, headers = selected
    text = json.dumps(body, indent=2, ensure_ascii=False) + '\n'
    if 'offline-capture-dummy' in text or str(Path.home()) in text:
        raise SystemExit('refusing to save request containing a credential or real home path')
    out = root / 'wire' / f'{mode}-request.json'
    out.parent.mkdir(exist_ok=True)
    if out.exists():
        raise SystemExit(f'refusing to overwrite {out}')
    out.write_text(text, encoding='utf-8')
    if mode.endswith('-beta'):
        header_path = root / 'wire' / f'{mode}-headers.json'
        if header_path.exists():
            raise SystemExit(f'refusing to overwrite {header_path}')
        header_path.write_text(json.dumps(headers, indent=2) + '\n', encoding='utf-8')
        print('beta:', headers.get('anthropic-beta'))
    print(f'CAPTURED mode={mode} path={path} tools={len(body["tools"])} system_blocks={len(body.get("system", []))} exit={process.returncode}')
    print('tool_names:', ', '.join(item.get('name', item.get('type', '?')) for item in body['tools']))
    print('file:', out)
