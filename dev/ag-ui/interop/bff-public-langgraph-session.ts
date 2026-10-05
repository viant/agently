import { randomUUID } from 'node:crypto';
import { readFile, writeFile } from 'node:fs/promises';
import { AgentlyClient } from '../../../../agently-core-ag-ui/sdk/ts/src/client';

// The browser-equivalent client connects only to the owned local BFF. The BFF
// registry, not this client, selects the reviewed anonymous upstream endpoint.
const origin = process.env.AGUI_BFF_ORIGIN ?? 'http://127.0.0.1:20431';
if (!['127.0.0.1', 'localhost'].includes(new URL(origin).hostname)) throw new Error('Live credential fixture requires an owned loopback BFF');
const privateTokenPath = process.env.AGUI_BFF_TOKEN_FILE;
if (!privateTokenPath) throw new Error('AGUI_BFF_TOKEN_FILE must name a private test token bundle');
let cookie = '';
const requests: { path: string; status: number }[] = [];
const counts: Record<string, number> = {};
const evidence: Record<string, unknown> = {
    checkedAt: new Date().toISOString(), mode: 'authenticated-bff-to-public-hosted',
    backend: 'Official AG-UI LangGraph Dojo FastAPI',
    productionUI: { tested: false, reason: 'This verifies the real BFF and SDK transport; rendered production UI is a separate gate.' },
};
try {
    const token = JSON.parse(await readFile(privateTokenPath, 'utf8'));
    const client = new AgentlyClient({ baseURL: `${origin}/v1`, useCookies: true, fetchImpl: async (url, init) => {
        const target = new URL(String(url));
        if (target.origin !== origin) throw new Error('BFF credentials must never reach another origin');
        const headers = new Headers(init?.headers);
        if (cookie) headers.set('Cookie', cookie);
        const response = await fetch(target, { ...init, headers, redirect: 'error', signal: AbortSignal.any([AbortSignal.timeout(60000), ...(init?.signal ? [init.signal] : [])]) });
        const cookies = response.headers.getSetCookie();
        if (cookies.length) cookie = cookies.map(value => value.split(';', 1)[0]).join('; ');
        requests.push({ path: target.pathname, status: response.status });
        return response;
    } });
    await client.createAuthSession({ accessToken: token.AccessToken, idToken: token.IDToken, refreshToken: token.RefreshToken, expiresAt: token.ExpiresAt });
    if (!cookie || !await client.getAuthMe()) throw new Error('Existing BFF session was not established');
    const backend = (await client.listAgUiBackends()).find(item => item.id === 'langgraph');
    if (!backend || backend.profile !== 'standard' || backend.durableReplay || !backend.publicDemo) throw new Error('Expected public-demo capability profile');
    const remote = await client.createAgUiBackendThread(backend.id);
    const session = client.createAgUiSession({ threadId: remote.threadId, connectionId: backend.id, profile: backend.profile, durableReplay: backend.durableReplay });
    const subscription = session.subscribeProtocol({ onEvent: ({ event }) => { counts[event.type] = (counts[event.type] ?? 0) + 1; } });
    const marker = 'AGUI_BFF_INTEROP_OK';
    await session.send({ id: randomUUID(), role: 'user', content: `Synthetic interoperability check. Reply with exactly ${marker}. Do not use tools.` }, { runId: randomUUID() });
    await session.send({ id: randomUUID(), role: 'user', content: 'Return the exact marker from your previous response, and nothing else. Do not use tools.' }, { runId: randomUUID() });
    subscription.unsubscribe();
    const last = [...session.getSnapshot().messages].reverse().find(message => message.role === 'assistant');
    if (session.getSnapshot().phase !== 'completed' || typeof last?.content !== 'string' || !last.content.includes(marker)) throw new Error('Standard follow-up failed');
    if (!await client.getAuthMe()) throw new Error('BFF session changed after streaming');
    evidence.status = 'passed';
    evidence.assertions = ['existing BFF session established', 'only authorized backend listed', 'server-issued scoped remote thread', 'two standard streamed runs and conversation continuity', 'all client requests use the BFF origin', 'legacy BFF identity remains valid'];
    evidence.eventCounts = counts;
    evidence.requests = requests;
    evidence.reply = marker;
    await writeFile(new URL('./evidence/bff-public-langgraph-session.json', import.meta.url), JSON.stringify(evidence, null, 2) + '\n');
    console.log('Authenticated BFF → official public LangGraph: PASS (two streamed runs; credentials confined to BFF)');
} catch (error) {
    // Never print server auth bodies, session cookies or the private token bundle.
    evidence.status = 'failed';
    evidence.failure = { type: error instanceof Error ? error.name : 'Error', status: (error as { status?: number })?.status };
    evidence.requests = requests;
    await writeFile(new URL('./evidence/bff-public-langgraph-session.json', import.meta.url), JSON.stringify(evidence, null, 2) + '\n');
    console.error('Authenticated BFF public-demo proof failed; sanitized evidence saved');
    process.exitCode = 1;
}
