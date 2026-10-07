import React, { useEffect, useRef, useState } from 'react';
import { connectorConfig } from '../../connector.js';
import { createStandardAppHost } from '../../services/mcpApps/standardHost.js';
import { client } from '../../services/agentlyClient.js';
import { AgUiMcpProxy } from '../../services/mcpApps/aguiProxy.js';
import { readStandardAppResource, resolveSandboxUrl } from '../../services/mcpApps/standardDocument.js';
import AppFrame from './AppFrame.jsx';
import { dispatchMCPUIApprovalRequest, subscribeMCPUIApprovalOutcomes } from '../../services/mcpApps/approvalEvents.js';

export default function StandardAppRenderer({ activity, sandboxUrl = connectorConfig.mcpUI.sandboxUrl }) {
  const frame = useRef(null);
  const current = useRef(activity);
  current.current = activity;
  const bridgeRef = useRef(null);
  const proxyLoads = useRef(0);
  const initialized = useRef(false);
  const resultSent = useRef('');
  const flushResultRef = useRef(null);
  const proxyRef = useRef(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState([]);
  useEffect(() => {
    let alive = true;
    initialized.current = false;
    resultSent.current = '';
    proxyLoads.current = 0;
    setError(''); setLoading(true); setPending([]);
    let proxy;
    try { proxy = activity.content._browserMCP ? client.browserMCPAppProxy(activity) : new AgUiMcpProxy(client, activity.content, update => {
      if (!alive) return;
      setPending(items => update.completed ? items.filter(item => item.id !== update.id)
        : [...items.filter(item => item.id !== update.id), update]);
    }); } catch (cause) { setError(String(cause?.message || cause)); setLoading(false); return () => { alive = false; }; }
    proxyRef.current = proxy;
    const stopOutcomes = subscribeMCPUIApprovalOutcomes(outcome => proxy.observeOutcome(outcome));
    let bridge;
    let delivery = Promise.resolve();
    const flushResult = () => {
      delivery = delivery.then(async () => {
        if (!alive || !initialized.current) return;
        const result = current.current.content.result;
        const signature = JSON.stringify(result);
        if (!result || signature === resultSent.current) return;
        await bridge.sendResult(result);
        if (alive) resultSent.current = signature;
      }).catch(cause => { if (alive) setError(String(cause?.message || cause)); });
      return delivery;
    };
    flushResultRef.current = flushResult;
    const onReady = async () => {
      if (!alive || initialized.current) return;
      try {
        const partial = current.current.content.toolInputPartial;
        if (partial) await bridge.sendPartialInput(partial);
        await bridge.sendInput(current.current.content.toolInput || {});
        if (!alive) return;
        initialized.current = true;
        await flushResult();
        if (alive) setLoading(false);
      } catch (cause) { if (alive) setError(String(cause?.message || cause)); }
    };
    void (async () => {
      try {
        const result = await proxy.call('resources/read', { uri: activity.content.resourceUri });
        if (!alive) return;
        const resource = readStandardAppResource(result, activity.content.resourceUri);
        const url = resolveSandboxUrl(sandboxUrl);
        const target = frame.current?.contentWindow;
        if (!target) throw new Error('App frame is unavailable');
        bridge = createStandardAppHost({ proxy, target, origin: new URL(url).origin, resource, onReady,
          onError: cause => { if (alive) setError(String(cause?.message || cause)); },
          onClosed: () => { if (alive && frame.current) frame.current.src = 'about:blank'; },
          onSize: ({ height }) => { if (alive && frame.current && Number.isFinite(height)) frame.current.style.height = `${Math.max(240, Math.min(1200, height))}px`; },
        });
        bridgeRef.current = bridge;
        await bridge.connect();
        if (alive && frame.current) frame.current.src = url;
      } catch (cause) { if (alive) { setError(String(cause?.message || cause)); setLoading(false); } }
    })();
    return () => {
      alive = false;
      initialized.current = false;
      bridgeRef.current = null;
      flushResultRef.current = null;
      proxy.dispose();
      proxyRef.current = null;
      stopOutcomes();
      void bridge?.close();
    };
  }, [activity.mountKey, sandboxUrl]);
  useEffect(() => {
    void flushResultRef.current?.();
  }, [activity]);
  return <div className="app-bubble-row app-bubble-row-assistant app-mcpui-bubble-row" data-testid="standard-mcp-app" data-app-instance={activity.content._agentlyApp.appInstanceId}>
    <div className="app-bubble app-bubble-assistant app-mcpui-bubble">
      <div className="app-bubble-content app-mcpui-bubble-content">
        <div className="app-mcpui-bubble-label">Interactive app</div>
        {loading && !error ? <div role="status">Loading app…</div> : null}
        {error ? <div role="alert">{error}</div> : null}
        {pending.length ? <div role="status">{pending.every(request => request.resuming) ? 'Finishing app request…' : 'Waiting for approval…'}
          {pending.flatMap(request => request.resuming ? [] : (request.interrupts || [])).filter(interrupt => interrupt.reason === 'approval').map(interrupt => (
            <button key={interrupt.id} type="button" onClick={() => dispatchMCPUIApprovalRequest({ approvalId: interrupt.id,
              resourceUri: activity.content.resourceUri, title: interrupt.message || 'App tool approval' })}>Review approval</button>
          ))}
          <button type="button" onClick={() => { void proxyRef.current?.refreshPending(); }}>Check status</button>
        </div> : null}
        <AppFrame ref={frame} onLoad={() => { if (bridgeRef.current && ++proxyLoads.current > 1) void bridgeRef.current.close(); }} sandbox="allow-scripts allow-same-origin" title="MCP App" />
      </div>
    </div>
  </div>;
}
