#!/usr/bin/env python3
"""Create an isolated workspace. Refuses an existing nonempty destination."""
import argparse
from pathlib import Path

parser = argparse.ArgumentParser()
parser.add_argument('workspace', type=Path)
parser.add_argument('--model-port', type=int, default=18082)
parser.add_argument('--mcp-app-port', type=int, help='Enable the owned MCP Apps fixture on this loopback port')
parser.add_argument('--presentation-fixture-port', type=int, help='Enable owned synthetic report/feed MCP fixture')
args = parser.parse_args()
root = args.workspace.resolve()
if root.exists() and any(root.iterdir()):
    parser.error('workspace must be new or empty')
(root / 'agents/simple').mkdir(parents=True, exist_ok=True)
(root / 'models').mkdir()
(root / 'config.yaml').write_text(f'''agents:
  url: agents
models:
  url: models
auth:
  enabled: false
default:
  agent: simple
  model: local_mock
  runtimeRoot: {root / 'runtime'}
  statePath: {root / 'runtime/state'}
  resources:
    indexPath: {root / 'runtime/index'}
    snapshotPath: {root / 'runtime/snapshots'}
internalMCP:
  services: []
''')
(root / 'models/local_mock.yaml').write_text(f'''id: local_mock
name: Local deterministic fixture
options:
  provider: openai
  model: gpt-4o-mini
  url: http://127.0.0.1:{args.model_port}/v1
  envKey: AGENTLY_AGUI_MOCK_KEY
  disableStreaming: false
  contextContinuation: false
''')
(root / 'agents/simple/simple.yaml').write_text('''id: simple
name: Local fixture agent
modelRef: local_mock
temperature: 0
persona:
  role: assistant
  actor: Fixture
prompt:
  uri: user.tmpl
systemPrompt:
  uri: system.tmpl
tool: {}
''')
(root / 'agents/simple/user.tmpl').write_text('{{.Task.Prompt}}\n')
(root / 'agents/simple/system.tmpl').write_text('Answer concisely.\n')
(root / 'agents/tool_fixture').mkdir()
(root / 'agents/tool_fixture/tool_fixture.yaml').write_text('''id: tool_fixture
name: Local tool fixture agent
modelRef: local_mock
temperature: 0
prompt:
  uri: user.tmpl
systemPrompt:
  uri: system.tmpl
tool:
  callExposure: conversation
  items:
    - pattern: "system/os:getEnv"
      type: function
''')
(root / 'agents/tool_fixture/user.tmpl').write_text('{{.Task.Prompt}}\n')
(root / 'agents/tool_fixture/system.tmpl').write_text('Use the provided tool for fixture-tool requests.\n')
(root / 'tools/bundles').mkdir(parents=True)
(root / 'tools/bundles/approval_fixture.yaml').write_text('''id: approval_fixture
match:
  - name: "system/os:getEnv"
    approval:
      mode: queue
      queueBehavior: wait
''')
(root / 'agents/approval_fixture').mkdir()
(root / 'agents/approval_fixture/approval_fixture.yaml').write_text('''id: approval_fixture
name: Local approval fixture agent
modelRef: local_mock
temperature: 0
prompt:
  uri: user.tmpl
systemPrompt:
  uri: system.tmpl
tool:
  callExposure: conversation
  bundles: [approval_fixture]
''')
(root / 'agents/approval_fixture/user.tmpl').write_text('{{.Task.Prompt}}\n')
(root / 'agents/approval_fixture/system.tmpl').write_text('Use the provided tool for fixture-tool requests.\n')
(root / 'tools/bundles/approval_effect_fixture.yaml').write_text('''id: approval_effect_fixture
match:
  - name: "system/exec:execute"
    approval:
      mode: queue
      queueBehavior: wait
''')
(root / 'agents/approval_effect_fixture').mkdir()
(root / 'agents/approval_effect_fixture/approval_effect_fixture.yaml').write_text(
    (root / 'agents/approval_fixture/approval_fixture.yaml').read_text().replace('approval_fixture', 'approval_effect_fixture'))
(root / 'agents/approval_effect_fixture/user.tmpl').write_text('{{.Task.Prompt}}\n')
(root / 'agents/approval_effect_fixture/system.tmpl').write_text('Use the provided tool for fixture-tool requests.\n')
if args.mcp_app_port is not None:
    (root / 'mcp').mkdir(exist_ok=True)
    (root / 'mcp/mcp_app_fixture.yaml').write_text(f'''name: mcp_app_fixture
transport:
  type: streamable
  url: http://127.0.0.1:{args.mcp_app_port}/mcp
toolsListVisibility: public
''')
    (root / 'tools/bundles/mcp_app_fixture.yaml').write_text('''id: mcp_app_fixture
match:
  - name: "mcp_app_fixture:fixture_view"
  - name: "mcp_app_fixture:fixture_approved"
    approval:
      mode: queue
      queueBehavior: wait
''')
    (root / 'agents/mcp_app_fixture').mkdir()
    (root / 'agents/mcp_app_fixture/mcp_app_fixture.yaml').write_text('''id: mcp_app_fixture
name: Local MCP Apps fixture agent
modelRef: local_mock
temperature: 0
prompt:
  uri: user.tmpl
systemPrompt:
  uri: system.tmpl
tool:
  callExposure: conversation
  bundles: [mcp_app_fixture]
''')
    (root / 'agents/mcp_app_fixture/user.tmpl').write_text('{{.Task.Prompt}}\n')
    (root / 'agents/mcp_app_fixture/system.tmpl').write_text('Use fixture_view for fixture-mcp-app requests.\n')
if args.presentation_fixture_port is not None:
    (root / 'mcp').mkdir(exist_ok=True)
    (root / 'mcp/presentation.yaml').write_text(f'''name: presentation
transport:
  type: streamable
  url: http://127.0.0.1:{args.presentation_fixture_port}/mcp
toolsListVisibility: public
''')
    (root / 'agents/presentation_fixture').mkdir()
    (root / 'agents/presentation_fixture/presentation_fixture.yaml').write_text('''id: presentation_fixture
name: Local presentation fixture agent
modelRef: local_mock
temperature: 0
prompt:
  uri: user.tmpl
systemPrompt:
  uri: system.tmpl
tool:
  callExposure: conversation
  items:
    - pattern: "presentation:fixture_feed"
      type: function
''')
    (root / 'agents/presentation_fixture/user.tmpl').write_text('{{.Task.Prompt}}\n')
    (root / 'agents/presentation_fixture/system.tmpl').write_text('Use fixture_feed only for explicit fixture-feed prompts.\n')
    (root / 'feeds').mkdir()
    (root / 'feeds/queue.yaml').write_text((Path(__file__).resolve().parents[2] / 'bootstrap/defaults/feeds/queue.yaml').read_text())
    (root / 'feeds/presentation.yaml').write_text('''id: presentation-fixture
title: Synthetic presentation feed
developerOnly: false
presentation:
  icon: table
  accent: '#3857d6'
  target: inline
match:
  service: presentation
  method: fixture_feed
activation:
  kind: history
  scope: turn
dataSource:
  output:
    source: output
  rows:
    dataSourceRef: output
    selectors: {data: rows}
    uniqueKey: [{field: id}]
ui:
  title: Synthetic presentation feed
  renderMode: forge
  containers:
    - id: fixture-feed-table
      type: table
      dataSourceRef: rows
      table:
        columns:
          - {id: label, name: Label}
          - {id: value, name: Value}
''')
print(root)
