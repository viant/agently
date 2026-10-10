import { buildStandardMcpDocument, MCP_APP_MIME, MAX_APP_BYTES } from './standardDocument.js';

// This entry point is deployed on a dedicated origin with no host cookies or
// application routes. Guest HTML only executes in the nested opaque frame.
export function startSandboxProxy({ window: win, document: doc }) {
  const hostOrigin = new URLSearchParams(win.location.hash.slice(1)).get('hostOrigin');
  if (!hostOrigin || new URL(hostOrigin).origin !== hostOrigin || hostOrigin === win.location.origin) throw new Error('Invalid sandbox host origin');
  let view = null;
  const onMessage = event => {
    const message = event.data;
    if (!message || message.jsonrpc !== '2.0') return;
    try { if (new TextEncoder().encode(JSON.stringify(message)).length > MAX_APP_BYTES * 6 + 4096) return; } catch { return; }
    if (event.source === win.parent && event.origin === hostOrigin) {
      if (message.method === 'ui/notifications/sandbox-resource-ready') {
        if (view || message.id !== undefined) return; // one resource per proxy
        const params = message.params || {};
        const document = buildStandardMcpDocument({ uri: 'ui://sandbox/view', mimeType: MCP_APP_MIME, text: params.html, _meta: { ui: { csp: params.csp } } });
        view = doc.createElement('iframe');
        view.setAttribute('sandbox', 'allow-scripts');
        view.setAttribute('title', 'MCP App view');
        view.style.cssText = 'display:block;width:100%;height:100%;border:0';
        let loaded = false;
        view.addEventListener('load', () => {
          if (!loaded) { loaded = true; return; }
          view?.remove(); view = null;
          win.parent.postMessage({ jsonrpc: '2.0', method: 'ui/notifications/request-teardown', params: {} }, hostOrigin);
        });
        view.srcdoc = document;
        doc.body.appendChild(view);
        return;
      }
      if (message.method?.startsWith('ui/notifications/sandbox-')) return;
      view?.contentWindow?.postMessage(message, '*');
    } else if (view && event.source === view.contentWindow && event.origin === 'null') {
      if (message.method?.startsWith('ui/notifications/sandbox-')) return;
      win.parent.postMessage(message, hostOrigin);
    }
  };
  win.addEventListener('message', onMessage);
  win.parent.postMessage({ jsonrpc: '2.0', method: 'ui/notifications/sandbox-proxy-ready', params: {} }, hostOrigin);
  return () => { win.removeEventListener('message', onMessage); view?.remove(); };
}
if (typeof window !== 'undefined' && window.parent !== window) startSandboxProxy({ window, document });
