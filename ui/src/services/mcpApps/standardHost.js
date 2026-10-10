import { AppBridge } from '@modelcontextprotocol/ext-apps/app-bridge';
import { JSONRPCMessageSchema } from '@modelcontextprotocol/sdk/types.js';

const guestMethods = new Set(['tools/call', 'resources/read', 'ping', 'ui/notifications/size-changed',
  'ui/notifications/request-teardown', 'notifications/cancelled', 'ui/request-display-mode']);
const idKey = id => `${typeof id}:${id}`;

// Official AppBridge handles schemas/version negotiation. This transport adds
// the lifecycle, exact window/origin, quota and nonreplay boundary the SDK does
// not enforce (its pre-initialized assertion is only a warning).
export class AppHostTransport {
  constructor(target, origin, eventWindow = globalThis.window) {
    this.target = target; this.origin = origin; this.eventWindow = eventWindow;
    this.phase = 'sandbox'; this.ids = new Set(); this.outstanding = new Set();
  }
  async start() {
    this.listener = event => {
      if (event.source !== this.target || event.origin !== this.origin || this.phase === 'closed') return;
      let message;
      try {
        if (new TextEncoder().encode(JSON.stringify(event.data)).length > 256 * 1024) return;
        message = JSONRPCMessageSchema.parse(event.data);
      } catch { return; }
      if (!message.method) {
        if (!this.outstanding.delete(idKey(message.id))) return;
        this.onmessage?.(message); return;
      }
      let accepted = false;
      if (this.phase === 'sandbox' && message.method === 'ui/notifications/sandbox-proxy-ready' && message.id === undefined) {
        this.phase = 'loading'; accepted = true;
      } else if (this.phase === 'loading' && message.method === 'ui/initialize' && message.id !== undefined) {
        this.phase = 'initializing'; this.initializeId = message.id; accepted = true;
      } else if (this.phase === 'initializing' && this.initializeSucceeded && message.method === 'ui/notifications/initialized' && message.id === undefined) {
        this.phase = 'ready'; accepted = true;
      } else if (this.phase === 'ready' && guestMethods.has(message.method)) accepted = true;
      if (message.id !== undefined) {
        const key = idKey(message.id);
        if (this.ids.has(key) || this.ids.size >= 256) return; // uncertain duplicate never invokes a handler
        this.ids.add(key);
        if (!accepted) {
          this.target.postMessage({ jsonrpc: '2.0', id: message.id, error: { code: -32601, message: 'App method unavailable in this lifecycle' } }, this.origin);
          return;
        }
      }
      if (accepted) this.onmessage?.(message);
    };
    this.eventWindow.addEventListener('message', this.listener);
  }
  async send(message) {
    if (this.phase === 'closed') throw new Error('App transport closed');
    if (message.method) {
      const sandboxLoad = message.method === 'ui/notifications/sandbox-resource-ready' && this.phase === 'loading';
      const teardown = message.method === 'ui/resource-teardown' && this.phase === 'teardown';
      if (this.phase !== 'ready' && !sandboxLoad && !teardown) throw new Error('App transport is not initialized');
    }
    if (!message.method && message.id === this.initializeId) this.initializeSucceeded = !!message.result?.protocolVersion && !message.error;
    if (message.method && message.id !== undefined) this.outstanding.add(idKey(message.id));
    this.target.postMessage(message, this.origin);
  }
  beginTeardown() { this.phase = 'teardown'; }
  async close() {
    if (this.phase === 'closed') return;
    this.phase = 'closed'; this.eventWindow.removeEventListener('message', this.listener);
    this.outstanding.clear(); this.onclose?.();
  }
}

export function createStandardAppHost({ proxy, target, origin, eventWindow, resource, onReady = () => {}, onSize = () => {}, onError = () => {}, onClosed = () => {} }) {
  const transport = new AppHostTransport(target, origin, eventWindow);
  const bridge = new AppBridge(null, { name: 'Agently', version: '1' }, {
    ...(proxy.serverTools === false ? {} : { serverTools: {} }), serverResources: {}, sandbox: { csp: resource.csp },
  }, { hostContext: { displayMode: 'inline', availableDisplayModes: ['inline'] } });
  let closed = false, ready = false, closing, queued = 0, completeInput = false, delivery = Promise.resolve();
  let initialize, abortInitialization;
  const initialized = new Promise((resolve, reject) => { initialize = resolve; abortInitialization = reject; });
  // Closing an app that never queued any data must not cause an unhandled rejection.
  void initialized.catch(() => {});
  const deliver = send => {
    if (closed) return Promise.reject(new Error('App host closed'));
    if (queued >= 32) return Promise.reject(new Error('App delivery queue exhausted'));
    queued++;
    const result = delivery.then(() => initialized).then(() => {
      if (closed) throw new Error('App host closed');
      return send();
    }).finally(() => { queued--; });
    delivery = result.catch(() => {});
    return result;
  };
  if (proxy.serverTools !== false) bridge.oncalltool = params => proxy.call('tools/call', params);
  bridge.onreadresource = params => proxy.call('resources/read', params);
  bridge.onsizechange = onSize;
  bridge.onrequestdisplaymode = async () => ({ mode: 'inline' });
  bridge.onsandboxready = () => { void bridge.sendSandboxResourceReady({ html: resource.html, csp: resource.csp, sandbox: 'allow-scripts' }).catch(onError); };
  bridge.oninitialized = () => { if (!closed) { ready = true; initialize(); onReady(); } };
  const close = () => {
    if (closing) return closing;
    closed = true; proxy.dispose();
    abortInitialization(new Error('App closed before initialization'));
    transport.beginTeardown();
    closing = (async () => {
      try { if (ready) await bridge.teardownResource({}, { timeout: 1000 }); }
      catch { /* A disappeared/unresponsive view loses authority on timeout. */ }
      finally { await bridge.close(); onClosed(); }
    })();
    return closing;
  };
  bridge.onrequestteardown = () => { void close(); };
  return {
    connect: () => bridge.connect(transport), close,
    sendInput: argumentsValue => deliver(() => { completeInput = true; return bridge.sendToolInput({ arguments: argumentsValue }); }),
    sendPartialInput: argumentsValue => deliver(() => {
      if (completeInput) throw new Error('Complete app tool input already sent');
      return bridge.sendToolInputPartial({ arguments: argumentsValue });
    }),
    sendResult: result => deliver(() => bridge.sendToolResult(result)),
    sendCancelled: reason => deliver(() => bridge.sendToolCancelled(reason ? { reason } : {})),
  };
}
