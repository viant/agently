import { randomUUID } from 'node:crypto';
import { mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { AgUiSession } from '../../../../agently-core-ag-ui/sdk/ts/src/aguiSession';

// Official upstream Render deployment; only synthetic data reaches this public demo.
const origin = 'https://ag-ui-dojo-langgraph-fastapi.onrender.com';
const endpoint = `${origin}/agent/agentic_chat`;
const sourceRevision = '97f789cc1c48eaaf6949c9e9a88abb5cf6e7c5e3';
const output = resolve(dirname(fileURLToPath(import.meta.url)), 'evidence/public-langgraph-session.json');
const result: Record<string, unknown> = {
    checkedAt: new Date().toISOString(), backend: 'Official AG-UI LangGraph Dojo FastAPI',
    mode: 'public-hosted', endpoint, sourceReferenceRevision: sourceRevision, deploymentRevision: null,
    sourceReference: `https://github.com/ag-ui-protocol/ag-ui/tree/${sourceRevision}/integrations/langgraph/python/examples`,
    deploymentConfig: `https://github.com/ag-ui-protocol/ag-ui/blob/${sourceRevision}/render.yaml`,
    client: { implementation: 'AgUiSession', profile: 'standard', sdk: '@ag-ui/client@1.0.1', authentication: 'none', credentials: 'omit' },
    productionUI: { tested: false, reason: 'This portable session harness is preparation; same production UI browser proof is a separate gate.' },
};

async function publicFetch(url: string, init: RequestInit = {}) {
    const target = new URL(url);
    if (target.origin !== origin) throw new Error('Interop harness restricts requests to the reviewed public demo origin');
    // Explicit allowlist: no ambient BFF cookies, tokens or user headers may cross this boundary.
    const headers = new Headers();
    const supplied = new Headers(init.headers);
    for (const key of ['accept', 'content-type']) { const value = supplied.get(key); if (value) headers.set(key, value); }
    return fetch(url, { ...init, headers, credentials: 'omit', redirect: 'error', signal: AbortSignal.any([AbortSignal.timeout(45000), ...(init.signal ? [init.signal] : [])]) });
}
const requests: { url: string; method: string; inputKeys: string[]; hasAgentlyExtension: boolean; credentials: string }[] = [];
const events: Record<string, number> = {};
const phases: string[] = [];

try {
    const health = await publicFetch(`${endpoint}/health`);
    if (!health.ok) throw new Error(`Public demo health HTTP ${health.status}`);
    result.health = await health.json();
    const openapi = await publicFetch(`${origin}/openapi.json`);
    if (!openapi.ok) throw new Error(`Public demo OpenAPI HTTP ${openapi.status}`);
    const specification = await openapi.json() as any;
    if (!specification.paths?.['/agent/agentic_chat']?.post) throw new Error('Reviewed public chat endpoint is absent from OpenAPI');
    result.openapi = { title: specification.info?.title, version: specification.info?.version, protocolVersionProperty: specification.components?.schemas?.RunAgentInput?.properties?.protocolVersion };
    const session = new AgUiSession({
        url: endpoint, profile: 'standard', connectionId: 'public-agui-langgraph-dojo', threadId: `agently-web-interop-${randomUUID()}`,
        fetch: async (url, init) => {
            const input = JSON.parse(String(init.body));
            if (input.forwardedProps?.agently) throw new Error('Generic interoperability run must not send Agently extension properties');
            requests.push({ url, method: init.method || 'GET', inputKeys: Object.keys(input).sort(), hasAgentlyExtension: false, credentials: 'omit' });
            return publicFetch(url, init);
        },
    });
    const unsubscribe = session.subscribe(() => { const phase = session.getSnapshot().phase; if (phases.at(-1) !== phase) phases.push(phase); });
    const protocol = session.subscribeProtocol({ onEvent: ({ event }) => { events[event.type] = (events[event.type] || 0) + 1; } });
    const assertions: { name: string; passed: boolean; detail?: string }[] = [];
    const token = 'AGUI_WEB_INTEROP_OK';
    await session.send({ id: randomUUID(), role: 'user', content: `Protocol interoperability smoke check. Reply with exactly ${token}. Do not call tools.` }, { runId: randomUUID() });
    const first = session.getSnapshot();
    const firstReply = first.messages.filter(message => message.role === 'assistant').map(message => typeof message.content === 'string' ? message.content : '').join('\n');
    assertions.push({ name: 'standard streamed chat completed', passed: first.phase === 'completed' && firstReply.includes(token) });
    await session.send({ id: randomUUID(), role: 'user', content: 'What exact token did you just reply with? Return only that token. Do not call tools.' }, { runId: randomUUID() });
    const second = session.getSnapshot();
    const lastReply = [...second.messages].reverse().find(message => message.role === 'assistant');
    assertions.push({ name: 'same thread follow-up preserves conversation', passed: second.phase === 'completed' && typeof lastReply?.content === 'string' && lastReply.content.includes(token) });
    const toolToken = 'AGUI_WEB_INTEROP_TOOL_OK';
    const tool = { name: 'interop_confirm_marker', description: 'A harmless frontend interoperability check: returns the supplied synthetic marker.', parameters: { type: 'object', properties: { marker: { type: 'string' } }, required: ['marker'], additionalProperties: false } };
    await session.send({ id: randomUUID(), role: 'user', content: `Call interop_confirm_marker with marker ${toolToken} exactly once. Wait for the tool result before your final response.` }, { runId: randomUUID(), tools: [tool] });
    const pending = session.getSnapshot();
    assertions.push({ name: 'standard frontend tool handoff', passed: pending.phase === 'completed' && pending.pendingToolCallIds.length === 1, detail: `phase=${pending.phase}; pending=${pending.pendingToolCallIds.length}` });
    let calls = 0;
    if (pending.phase === 'completed' && pending.pendingToolCallIds.length === 1) {
        await session.client.executeClientTools([{ tool, execute: args => {
            const input = args as { marker?: string };
            if (input.marker !== toolToken) throw new Error('Frontend tool received unexpected marker');
            calls += 1;
            return { content: input.marker };
        } }]);
        await session.continue({ runId: randomUUID(), tools: [tool] });
    }
    const final = session.getSnapshot();
    const toolReply = [...final.messages].reverse().find(message => message.role === 'assistant' && typeof message.content === 'string' && message.content.length);
    assertions.push({ name: 'frontend tool result continues through same session', passed: calls === 1 && final.phase === 'completed' && typeof toolReply?.content === 'string' && toolReply.content.includes(toolToken) });
    assertions.push({ name: 'official reducer observes protocol text and state', passed: (events.RUN_STARTED || 0) >= 2 && (events.RUN_FINISHED || 0) >= 2 && (events.TEXT_MESSAGE_CONTENT || 0) > 0 && (events.STATE_SNAPSHOT || 0) > 0 });
    assertions.push({ name: 'no Agently extension discovery or envelope', passed: requests.length === 4 && requests.every(request => !request.hasAgentlyExtension) });
    // The checkpoint is generic session state; durable replay is correctly disabled for this public target.
    let replayRejected = false;
    try { await session.reconnect(); } catch (error) { replayRejected = String(error).includes('not advertised durable replay'); }
    assertions.push({ name: 'does not assume unsupported durable replay', passed: replayRejected });
    result.assertions = assertions;
    result.status = assertions.every(assertion => assertion.passed) ? 'passed' : 'failed';
    result.syntheticAssistantReplies = final.messages.filter(message => message.role === 'assistant').map(message => message.content);
    result.final = { phase: final.phase, messageCount: final.messages.length, stateKeys: Object.keys(final.state || {}), pendingInterrupts: final.interrupts.length, frontendToolExecutions: calls };
    unsubscribe(); protocol.unsubscribe();
    if (result.status !== 'passed') process.exitCode = 1;
} catch (error) {
    result.status = 'failed';
    result.error = error instanceof Error ? { name: error.name, message: error.message } : String(error);
    process.exitCode = 1;
} finally {
    result.eventCounts = events; result.phases = phases; result.requests = requests;
    await mkdir(dirname(output), { recursive: true });
    await writeFile(output, `${JSON.stringify(result, null, 2)}\n`);
    console.log(JSON.stringify({ status: result.status, evidence: output, error: result.error, eventCounts: events }, null, 2));
}
