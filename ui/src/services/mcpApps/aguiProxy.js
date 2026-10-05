// Host RPCs have isolated protocol histories and capture the trusted alias.
// Guest parameters can never replace the backend binding.
import { AgUiCommands } from 'agently-core-ui-sdk';

export class AgUiMcpProxy {
  constructor(client, content, onPending = () => {}) {
    this.client = client;
    this.binding = Object.freeze({ serverId: content.serverId, serverHash: content.serverHash });
    this.onPending = onPending;
    this.requests = new Map();
    this.disposed = false;
    this.nativeConversationId = content._agentlyApp?.threadId || '';
  }
  call(method, params = {}) {
    if (this.disposed) return Promise.reject(new Error('App host was disposed'));
    if (!['resources/read', 'tools/call', 'ping'].includes(method)) return Promise.reject(new Error('Unsupported app method'));
    const id = crypto.randomUUID();
    const session = this.client.createAgUiSession({ connectionId: 'agently', profile: 'agently', durableReplay: true, threadId: id });
    return new Promise((resolve, reject) => {
      const request = { id, session, resolve, reject };
      this.requests.set(id, request);
      void this.execute(request, () => session.continue({ runId: id, forwardedProps: {
        __proxiedMCPRequest: { ...this.binding, method, params: structuredClone(params) },
      } }));
    });
  }
  async execute(request, action) {
    try {
      const result = await action();
      if (this.disposed || !this.requests.has(request.id)) return;
      const snapshot = request.session.getSnapshot();
      if (snapshot.phase === 'interrupted') {
        request.pending = { id: request.id, threadId: snapshot.threadId, runId: snapshot.runId, interrupts: snapshot.interrupts };
        request.attaching = false;
        this.onPending(request.pending);
        this.observeRecovery();
        return; // JSON-RPC remains pending; never return a queued fake result.
      }
      if (snapshot.phase !== 'completed') throw new Error(`App request ${snapshot.phase}`);
      this.requests.delete(request.id);
      if (!this.requests.size) { this.observer?.close(); this.observer = null; }
      this.onPending({ id: request.id, completed: true });
      request.resolve(result.result);
    } catch (error) {
      if (!this.requests.delete(request.id)) return;
      if (!this.requests.size) { this.observer?.close(); this.observer = null; }
      this.onPending({ id: request.id, completed: true });
      request.reject(error);
    }
  }
  resume(id, responses) {
    const request = this.requests.get(id);
    if (!request) throw new Error('App request is no longer pending');
    if (request.attaching) throw new Error('App continuation is already being observed');
    return this.execute(request, () => request.session.resume(responses, { runId: crypto.randomUUID() }));
  }
  observeOutcome(outcome) {
    const protocol = outcome?.protocol;
    if (this.disposed || protocol?.version !== '1' || protocol.kind !== 'mcp-app' || !protocol.continuationRunId) return false;
    for (const request of this.requests.values()) {
      const pending = request.pending;
      if (!pending || request.attaching || pending.threadId !== protocol.threadId || pending.runId !== protocol.originalRunId
        || !pending.interrupts.some(interrupt => interrupt.id === outcome.approvalId)) continue;
      this.attachContinuation(request, protocol.continuationRunId);
      return true;
    }
    return false;
  }
  attachContinuation(request, runId) {
    if (this.disposed || request.attaching || !runId || runId === request.pending?.runId) return;
    request.attaching = true;
    this.onPending({ ...request.pending, resuming: true });
    // The inbox already submitted the resume. Observe its journal with a fresh
    // reducer: the interrupted client correctly rejects a run without answers.
    request.session.detach();
    request.session = this.client.createAgUiSession({ connectionId: 'agently', profile: 'agently', durableReplay: true,
      threadId: request.pending.threadId });
    void this.execute(request, () => request.session.attach(runId, crypto.randomUUID()));
  }
  observeRecovery() {
    if (this.observer || !this.nativeConversationId || !this.client.observeNativeEvents) return;
    this.observer = this.client.observeNativeEvents(this.nativeConversationId, {
      onOpen: () => { void this.refreshPending(); },
      onEvent: event => { if (event.type === 'conversation_meta_updated' && event.patch?.aguiUpdated === true) void this.refreshPending(); },
    });
    void this.refreshPending();
  }
  refreshPending() {
    if (this.disposed || !this.client.agUiTransport) return Promise.resolve();
    if (this.refreshing) { this.refreshAgain = true; return this.refreshing; }
    this.refreshAgain = false;
    this.refreshing = (async () => {
      for (const request of this.requests.values()) {
        if (!request.pending || request.attaching) continue;
        const pending = request.pending;
        try {
          const commands = new AgUiCommands(this.client.agUiTransport());
          const runId = crypto.randomUUID();
          const result = await commands.execute('run.get', { runId: pending.runId }, { threadId: pending.threadId, runId, requestId: runId });
          if (!this.disposed && this.requests.has(request.id) && request.pending === pending && result?.resumedByRunId) this.attachContinuation(request, result.resumedByRunId);
        } catch (error) {
          if (!this.disposed && this.requests.has(request.id)) this.onPending({ ...pending, statusError: String(error?.message || error) });
        }
      }
    })().finally(() => {
      this.refreshing = null;
      if (this.refreshAgain && !this.disposed) void this.refreshPending();
    });
    return this.refreshing;
  }
  dispose() {
    this.disposed = true;
    this.observer?.close();
    for (const request of this.requests.values()) {
      request.session.detach();
      request.reject(new DOMException('App host was disposed', 'AbortError'));
    }
    this.requests.clear();
  }
}
