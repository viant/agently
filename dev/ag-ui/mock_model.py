#!/usr/bin/env python3
"""Loopback-only deterministic OpenAI Chat Completions fixture; no credentials."""
import argparse
import json
import shlex
import time
from presentation_fixture import report_text, feed_call
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        if self.path != '/v1/chat/completions':
            self.send_error(404)
            return
        if self.server.queue_input_log:
            latest = next((message for message in reversed(request.get('messages', [])) if message.get('role') == 'user'), {})
            with open(self.server.queue_input_log, 'a') as log:
                log.write(json.dumps({'at': time.time(), 'latestUserContent': latest.get('content', '')}) + '\n')
        text = 'Hello from the local Agently AG-UI assembly fixture.'
        forge = any('fixture-forge' in str(message.get('content', '')) for message in request.get('messages', []))
        if forge:
            text = 'Report:\n```forge-data\n{"id":"rows","data":[{"private_authoring_key":1}]}\n```\n```forge-ui\n{"version":1,"blocks":[]}\n```\nDone.'
        if any('fixture-report' in str(message.get('content', '')) for message in request.get('messages', [])):
            text = report_text()
            forge = True
        tools = request.get('tools') or []
        client_tool = any('fixture-client-tool' in str(message.get('content', '')) for message in request.get('messages', []))
        needs_tool = tools and (client_tool or any('fixture-tool' in str(message.get('content', '')) for message in request.get('messages', []))) and not any(message.get('role') == 'tool' for message in request.get('messages', []))
        tool_call = {'index': 0, 'id': 'fixture-tool-call', 'type': 'function', 'function': {'name': tools[0]['function']['name'] if tools else 'unused', 'arguments': json.dumps({'names': ['AGENTLY_AGUI_FIXTURE_VALUE']})}}
        if client_tool:
            frontend = next((tool for tool in tools if 'ui_lookup' in tool['function']['name']), tools[0] if tools else None)
            tool_call = {'index': 0, 'id': 'fixture-client-call', 'type': 'function', 'function': {'name': frontend['function']['name'] if frontend else 'unused', 'arguments': json.dumps({'value': 'client fixture'})}}
        if any('fixture-mcp-app' in str(message.get('content', '')) for message in request.get('messages', [])):
            selected = next((tool for tool in tools if 'fixture_view' in tool['function']['name']), None)
            needs_tool = selected is not None and not any(message.get('role') == 'tool' for message in request.get('messages', []))
            tool_call = {'index': 0, 'id': 'fixture-app-call', 'type': 'function', 'function': {'name': selected['function']['name'] if selected else 'unused', 'arguments': json.dumps({'instance': 'initial'})}}
        if any('fixture-tool-approval-effect' in str(message.get('content', '')) for message in request.get('messages', [])):
            if not self.server.approval_effect_log:
                self.send_error(400, 'approval effect fixture requires --approval-effect-log')
                return
            code = f"from pathlib import Path; p=Path({self.server.approval_effect_log!r}); p.open('a').write('executed\\n'); print('fixture-effect-recorded')"
            tool_call['function']['arguments'] = json.dumps({'commands': ['python3 -c ' + shlex.quote(code)]})
        if any('fixture-goal-accounting' in str(message.get('content', '')) for message in request.get('messages', [])):
            time.sleep(1.2)  # Actual completed-turn elapsed accounting, no fake RequestTime.
        presentation_call = feed_call(request.get('messages', []), tools)
        if presentation_call is not None:
            needs_tool = True
            tool_call = presentation_call
        delay_messages = request.get('messages', [])
        if self.server.cancel_latest_user_only:
            delay_messages = [next((message for message in reversed(delay_messages) if message.get('role') == 'user'), {})]
        if any('fixture-cancel-window' in str(message.get('content', '')) for message in delay_messages):
            time.sleep(self.server.cancel_window_seconds)  # Owned browser fixture: allow the real stop control to cancel admission/execution.
        base = {'id': 'mock-completion', 'object': 'chat.completion', 'created': 1, 'model': request.get('model', 'mock')}
        self.send_response(200)
        if request.get('stream'):
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            deltas = [{'role': 'assistant'}, {'tool_calls': [tool_call]}] if needs_tool else [{'role': 'assistant'}, {'content': text}]
            if forge and not needs_tool:
                # Split fence names and JSON across chunks to exercise safe
                # streaming projection rather than only terminal extraction.
                deltas = [{'role': 'assistant'}] + [{'content': text[i:i+7]} for i in range(0, len(text), 7)]
            for delta in deltas:
                self.wfile.write(('data: ' + json.dumps({**base, 'object': 'chat.completion.chunk', 'choices': [{'index': 0, 'delta': delta, 'finish_reason': None}]}) + '\n\n').encode())
                self.wfile.flush()
            self.wfile.write(('data: ' + json.dumps({**base, 'object': 'chat.completion.chunk', 'choices': [{'index': 0, 'delta': {}, 'finish_reason': 'tool_calls' if needs_tool else 'stop'}]}) + '\n\n').encode())
            if any('fixture-goal-accounting' in str(message.get('content', '')) for message in request.get('messages', [])):
                self.wfile.write(('data: ' + json.dumps({**base, 'object': 'chat.completion.chunk', 'choices': [], 'usage': {'prompt_tokens': 5, 'completion_tokens': 10, 'total_tokens': 15}}) + '\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
        else:
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({**base, 'choices': [{'index': 0, 'message': {'role': 'assistant', 'content': text}, 'finish_reason': 'stop'}], 'usage': {'prompt_tokens': 5, 'completion_tokens': 10, 'total_tokens': 15}}).encode())

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--port', type=int, default=18082)
    parser.add_argument('--cancel-window-seconds', type=int, default=30, help='Owned deterministic overlap window; default keeps existing cancellation proof')
    parser.add_argument('--cancel-latest-user-only', action='store_true', help='Owned Queue proof: ignore historical delay markers; default preserved')
    parser.add_argument('--queue-input-log', help='Owned synthetic Queue proof latest-user model input evidence')
    parser.add_argument('--approval-effect-log', help='Owned temporary file used only by explicit approval effect fixture')
    args = parser.parse_args()
    server = ThreadingHTTPServer(('127.0.0.1', args.port), Handler)
    server.approval_effect_log = args.approval_effect_log
    server.cancel_window_seconds = args.cancel_window_seconds
    server.cancel_latest_user_only = args.cancel_latest_user_only
    server.queue_input_log = args.queue_input_log
    print(f'Mock model listening on http://127.0.0.1:{server.server_port}/v1', flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        server.server_close()
