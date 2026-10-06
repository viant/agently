import assert from 'node:assert/strict';
import {readFile, writeFile} from 'node:fs/promises';
import {createRequire} from 'node:module';
import {randomUUID} from 'node:crypto';
const require = createRequire(new URL('../../../agently-core-ag-ui/examples/ag-ui-shell/package.json', import.meta.url));
const {HttpAgent} = require('@ag-ui/client');
const {EventSchema} = require('@ag-ui/core/schemas');
const [phase, path] = process.argv.slice(2);
assert.ok(['prepare', 'resume'].includes(phase) && path, 'prepare|resume and owned temporary file required');
const url = process.env.AGENTLY_AGUI_URL ?? 'http://127.0.0.1:20421/v1/ag-ui/run';
let cookie = '', accepted, wire;
async function run(threadId, runId, forwardedProps, messages = [], resume) {
  const agent = new HttpAgent({url, threadId, initialMessages: messages, fetch: async (target, init) => {
    accepted = JSON.parse(init.body);
    const response = await fetch(target, {...init, headers: {...init.headers, ...(cookie ? {cookie} : {})}});
    if (response.headers.get('set-cookie')) cookie = response.headers.get('set-cookie').split(';')[0];
    assert.equal(response.status, 200, await response.clone().text());
    wire = response.clone().text();
    return response;
  }});
  const events = [];
  const result = await agent.runAgent({runId, forwardedProps, ...(resume ? {resume} : {})}, {
    onEvent: ({event}) => {EventSchema.parse(event); events.push(event);},
  });
  return {events, result, input: accepted, wire: await wire};
}
async function count(instance) {
  const log = await readFile(process.env.MCP_APP_LOG, 'utf8');
  return log.split('\n').filter(Boolean).map(JSON.parse).filter(r => r.method === 'tools/call' && r.params?.arguments?.instance === instance).length;
}
async function replay(input, expected) {
  const response = await fetch(url, {method: 'POST', headers: {'content-type': 'application/json', cookie}, body: JSON.stringify(input)});
  assert.equal(response.status, 200);
  assert.equal(await response.text(), expected);
}
if (phase === 'prepare') {
  const chat = await run(randomUUID(), randomUUID(), {agently: {version: '1', operation: 'chat', payload: {agentId: 'mcp_app_fixture', model: 'local_mock'}}}, [{id: randomUUID(), role: 'user', content: 'fixture-mcp-app restart'}]);
  const content = chat.events.find(e => e.type === 'ACTIVITY_SNAPSHOT' && e.activityType === 'mcp-apps')?.content;
  assert.ok(content);
  const scope = {serverId: content.serverId, serverHash: content.serverHash};
  const readID = randomUUID();
  const read = await run(readID, readID, {__proxiedMCPRequest: {...scope, method: 'resources/read', params: {uri: content.resourceUri}}});
  const isolated = randomUUID(), instance = randomUUID();
  const props = {__proxiedMCPRequest: {...scope, method: 'tools/call', params: {name: 'fixture_approved', arguments: {instance}}}};
  const pending = await run(isolated, isolated, props);
  assert.equal(pending.events.at(-1).outcome?.type, 'interrupt');
  assert.equal(await count(instance), 0);
  await writeFile(path, JSON.stringify({cookie, read, pending, props, isolated, instance}), {mode: 0o600});
  console.log('Prepared durable app alias, completed resource receipt, and approval handoff; zero remote effects.');
} else {
  const saved = JSON.parse(await readFile(path, 'utf8'));
  cookie = saved.cookie;
  await replay(saved.read.input, saved.read.wire);
  await replay(saved.pending.input, saved.pending.wire);
  assert.equal(await count(saved.instance), 0);
  const interrupt = saved.pending.events.at(-1).outcome.interrupts[0];
  const resumed = await run(saved.isolated, randomUUID(), saved.props, [], [{interruptId: interrupt.id, status: 'resolved', payload: {action: 'approve'}}]);
  assert.equal(resumed.events.at(-1).outcome?.type, 'success', JSON.stringify(resumed.events));
  assert.equal(resumed.result.result._meta['fixture/private'].instance, saved.instance);
  assert.equal(resumed.result.result.structuredContent.instance, saved.instance);
  assert.equal(await count(saved.instance), 1);
  await replay(resumed.input, resumed.wire);
  assert.equal(await count(saved.instance), 1);
  console.log('Actual backend restart: aliases and receipts resolve; original approval resumes once; exact replay adds no effect.');
}
