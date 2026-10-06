// Real upstream HttpAgent -> assembled Agently -> local OpenAI fixture.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { randomUUID } from 'node:crypto';
const require = createRequire(new URL('../../../agently-core-ag-ui/sdk/ts/package.json', import.meta.url));
const { HttpAgent } = require('@ag-ui/client');
const { EventSchema } = require('@ag-ui/core/schemas');
// Validate a cloned response independently while HttpAgent consumes the live stream.
const wireValidations = [];
const validatedFetch = async (input, init) => {
  const response = await fetch(input, init);
  if (!response.ok) return response;
  wireValidations.push(response.clone().text().then(wire => {
    let count = 0;
    for (const frame of wire.split(/\r?\n\r?\n/)) {
      const data = frame.split(/\r?\n/).filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n');
      if (data) { EventSchema.parse(JSON.parse(data)); count++; }
    }
    return { count };
  }).catch(error => ({ error })));
  return response;
};
const url = process.env.AGENTLY_AGUI_URL ?? 'http://127.0.0.1:18081/v1/ag-ui/run';
for (let attempt = 0; attempt < 30; attempt++) {
  try { if ((await fetch(new URL('/healthz', url))).ok) break; }
  catch { /* Assembly startup may still be loading the isolated workspace. */ }
  if (attempt === 29) throw new Error('Assembly backend did not become ready');
  await new Promise(resolve => setTimeout(resolve, 500));
}
let capabilities;
const discovery = new HttpAgent({ url, fetch: validatedFetch, threadId: randomUUID() });
await discovery.runAgent({ forwardedProps: { agently: { version: '1', operation: 'capabilities' } } }, {
  onCustomEvent: ({ event }) => { if (event.name === 'agently.capabilities') capabilities = event.value; },
});
assert.equal(capabilities?.version, '1');
assert.equal(capabilities.capabilities.custom.agently.execution.agentId, true);
for (const [agentId, prompt, tool] of [['standard', 'Hello standard agent', false], ['simple', 'Hello fixture', false], ['tool_fixture', 'fixture-tool please', true]]) {
  const agent = new HttpAgent({ url, fetch: validatedFetch, threadId: randomUUID(), initialMessages: [{ id: randomUUID(), role: 'user', content: prompt }] });
  const events = [];
  let runError;
  const parameters = agentId === 'standard' ? {} : { forwardedProps: { agently: { version: '1', operation: 'chat', payload: { agentId, model: 'local_mock' } } } };
  await agent.runAgent(parameters, {
    onEvent: ({ event }) => { events.push(event.type); },
    onRunErrorEvent: ({ event }) => { runError = event.message; },
  });
  assert.equal(runError, undefined);
  assert.equal(events.at(-1), 'RUN_FINISHED');
  assert.ok(agent.messages.some(message => message.content === 'Hello from the local Agently AG-UI assembly fixture.'));
  if (tool) {
    assert.ok(events.includes('TOOL_CALL_ARGS'));
    assert.ok(events.includes('TOOL_CALL_RESULT'));
    const call = agent.messages.flatMap(message => message.toolCalls ?? []).find(call => /^agui\.tool\/[^/]+\/fixture-tool-call$/.test(call.id));
    assert.ok(call, 'tool identity must be scoped to the native logical turn');
    assert.deepEqual(JSON.parse(call.function.arguments), { names: ['AGENTLY_AGUI_FIXTURE_VALUE'] });
    const result = agent.messages.find(message => message.role === 'tool');
    assert.equal(result?.toolCallId, call.id);
    assert.ok(result?.content.includes('fixture-value'));
  }
  console.log(`${agentId}: verified ${events.length} AG-UI events, ${agent.messages.length} protocol messages`);
}
const forge = new HttpAgent({ url, fetch: validatedFetch, threadId: randomUUID(), initialMessages: [{ id: randomUUID(), role: 'user', content: 'fixture-forge please' }] });
const activities = [];
const visibleDeltas = [];
let forgeError;
await forge.runAgent({ forwardedProps: { agently: { version: '1', operation: 'chat', payload: { agentId: 'simple', model: 'local_mock' } } } }, {
  onEvent: ({ event }) => {
    if (event.type === 'ACTIVITY_SNAPSHOT') activities.push(event);
    if (event.type === 'TEXT_MESSAGE_CONTENT') visibleDeltas.push(event.delta);
  },
  onRunErrorEvent: ({ event }) => { forgeError = event.message; },
});
assert.equal(forgeError, undefined);
assert.ok(activities.some(event => event.activityType === 'agently.rendered-content' && event.content.version === '1' && event.content.renderedContent), 'rich content must remain a structured activity');
const visible = visibleDeltas.join('');
assert.ok(visible.includes('Report:') && visible.includes('Done.') && visible.includes('[Interactive content]'));
for (const text of [visible, ...forge.messages.filter(message => message.role === 'assistant').map(message => message.content ?? '')]) {
  assert.ok(!text.includes('private_authoring_key') && !text.includes('forge-ui') && !text.includes('forge-data'), 'authoring payload must never appear in standard chat');
}
console.log(`forge: verified ${activities.length} structured activity snapshots and safe fragmented chat`);
const wireResults = await Promise.all(wireValidations);
for (const result of wireResults) { if (result.error) throw result.error; }
console.log(`Assembly AG-UI smoke passed; ${wireResults.reduce((count, result) => count + result.count, 0)} raw wire events validated against official 1.0 schema`);
