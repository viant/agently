import React from 'react';
import {
  MCPUI_GUEST_BRIDGE_READY_EVENT,
  MCPUI_HOST_READY_EVENT,
} from '../../services/mcpApps/forgeGuestBridge.js';
import {
  probeDirectBridge,
  probeDirectOpenLink,
  probeDirectToolCall,
  probeUnsafeOpenLink,
  readVerifierDiagnosticsSnapshot,
} from '../../services/mcpApps/mcpuiVerifierRouteDiagnostics.js';

export default function MCPUIVerifierRouteDebug() {
  const [snapshot, setSnapshot] = React.useState(() => readVerifierDiagnosticsSnapshot(typeof window === 'undefined' ? null : window));
  const [probe, setProbe] = React.useState({ attempts: 0, ok: null, reason: '', timestamp: '' });
  const [toolProbe, setToolProbe] = React.useState({ attempts: 0, ok: null, reason: '', timestamp: '' });
  const [linkProbe, setLinkProbe] = React.useState({ attempts: 0, ok: null, reason: '', timestamp: '' });
  const [unsafeLinkProbe, setUnsafeLinkProbe] = React.useState({
    attempts: 0,
    ok: null,
    reason: '',
    timestamp: '',
    url: '',
    hostLinkAffordance: null,
    hostPolicyReason: '',
  });

  React.useEffect(() => {
    if (typeof window === 'undefined') return () => {};
    const refresh = () => setSnapshot(readVerifierDiagnosticsSnapshot(window));
    refresh();
    window.addEventListener(MCPUI_HOST_READY_EVENT, refresh);
    window.addEventListener(MCPUI_GUEST_BRIDGE_READY_EVENT, refresh);
    return () => {
      window.removeEventListener(MCPUI_HOST_READY_EVENT, refresh);
      window.removeEventListener(MCPUI_GUEST_BRIDGE_READY_EVENT, refresh);
    };
  }, []);

  const triggerProbe = React.useCallback(() => {
    const result = probeDirectBridge(typeof window === 'undefined' ? null : window);
    setProbe((prev) => ({
      attempts: prev.attempts + 1,
      ok: result.ok,
      reason: result.reason,
      timestamp: result.timestamp,
    }));
  }, []);

  const triggerToolProbe = React.useCallback(() => {
    const result = probeDirectToolCall(typeof window === 'undefined' ? null : window);
    setToolProbe((prev) => ({
      attempts: prev.attempts + 1,
      ok: result.ok,
      reason: result.reason,
      timestamp: result.timestamp,
    }));
  }, []);

  const triggerLinkProbe = React.useCallback(() => {
    const result = probeDirectOpenLink(typeof window === 'undefined' ? null : window);
    setLinkProbe((prev) => ({
      attempts: prev.attempts + 1,
      ok: result.ok,
      reason: result.reason,
      timestamp: result.timestamp,
    }));
  }, []);

  const triggerUnsafeLinkProbe = React.useCallback(() => {
    const result = probeUnsafeOpenLink(typeof window === 'undefined' ? null : window);
    const hostPolicy = result.hostPolicy || {};
    setUnsafeLinkProbe((prev) => ({
      attempts: prev.attempts + 1,
      ok: result.ok,
      reason: result.reason,
      timestamp: result.timestamp,
      url: result.url || '',
      hostLinkAffordance: typeof hostPolicy.hostLinkAffordance === 'boolean' ? hostPolicy.hostLinkAffordance : null,
      hostPolicyReason: String(hostPolicy.reason || ''),
    }));
  }, []);

  return (
    <aside
      data-testid="mcpui-verifier-route-debug"
      style={{
        padding: '10px 14px',
        borderBottom: '1px solid #d8dee8',
        background: '#eef4ff',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
        fontSize: 12,
        color: '#1d2939',
        display: 'grid',
        gap: 6,
      }}
    >
      <div style={{ fontWeight: 600, fontSize: 13 }}>MCP UI verifier route diagnostics</div>
      <div data-testid="diag-bridge">
        <strong>Guest bridge installed:</strong>{' '}
        <span data-value={String(snapshot.guestBridgeInstalled)}>
          {snapshot.guestBridgeInstalled ? 'yes' : 'no'}
        </span>
      </div>
      <div data-testid="diag-host-ready">
        <strong>Host-ready received:</strong>{' '}
        <span data-value={String(snapshot.hostReady)}>
          {snapshot.hostReady ? 'yes' : 'no'}
        </span>
      </div>
      <div data-testid="diag-window-id">
        <strong>windowId:</strong>{' '}
        <span>{snapshot.windowId || '(empty)'}</span>
      </div>
      <div data-testid="diag-resource-uri">
        <strong>resourceUri:</strong>{' '}
        <span>{snapshot.resourceUri || '(empty)'}</span>
      </div>
      <div data-testid="diag-allowed-tools">
        <strong>allowedTools:</strong>{' '}
        <span>{snapshot.allowedTools.length === 0 ? '(none)' : snapshot.allowedTools.join(', ')}</span>
      </div>
      <div data-testid="diag-allowed-tool-bundles">
        <strong>allowedToolBundles:</strong>{' '}
        <span>{snapshot.allowedToolBundles.length === 0 ? '(none)' : snapshot.allowedToolBundles.join(', ')}</span>
      </div>
      <div>
        <button type="button" data-testid="diag-probe-direct-bridge" onClick={triggerProbe}>
          Probe direct bridge.message
        </button>
        {probe.attempts > 0 ? (
          <span data-testid="diag-probe-result" style={{ marginLeft: 8 }} data-ok={String(Boolean(probe.ok))}>
            attempt #{probe.attempts}: {probe.ok ? 'ok' : 'fail'} — {probe.reason} @ {probe.timestamp}
          </span>
        ) : (
          <span data-testid="diag-probe-result-empty" style={{ marginLeft: 8 }}>
            no probe attempts yet
          </span>
        )}
      </div>
      <div>
        <button type="button" data-testid="diag-probe-direct-toolcall" onClick={triggerToolProbe}>
          Probe direct bridge.toolCall
        </button>
        {toolProbe.attempts > 0 ? (
          <span data-testid="diag-tool-probe-result" style={{ marginLeft: 8 }} data-ok={String(Boolean(toolProbe.ok))}>
            attempt #{toolProbe.attempts}: {toolProbe.ok ? 'ok' : 'fail'} — {toolProbe.reason} @ {toolProbe.timestamp}
          </span>
        ) : null}
      </div>
      <div>
        <button type="button" data-testid="diag-probe-direct-openlink" onClick={triggerLinkProbe}>
          Probe direct bridge.openLink
        </button>
        {linkProbe.attempts > 0 ? (
          <span data-testid="diag-link-probe-result" style={{ marginLeft: 8 }} data-ok={String(Boolean(linkProbe.ok))}>
            attempt #{linkProbe.attempts}: {linkProbe.ok ? 'ok' : 'fail'} — {linkProbe.reason} @ {linkProbe.timestamp}
          </span>
        ) : null}
      </div>
      <div>
        <button type="button" data-testid="diag-probe-unsafe-openlink" onClick={triggerUnsafeLinkProbe}>
          Probe unsafe bridge.openLink (javascript:)
        </button>
        {unsafeLinkProbe.attempts > 0 ? (
          <span
            data-testid="diag-unsafe-link-probe-result"
            style={{ marginLeft: 8 }}
            data-ok={String(Boolean(unsafeLinkProbe.ok))}
            data-host-link-affordance={String(unsafeLinkProbe.hostLinkAffordance)}
          >
            attempt #{unsafeLinkProbe.attempts}: {unsafeLinkProbe.ok ? 'ok' : 'fail'} — {unsafeLinkProbe.reason} @ {unsafeLinkProbe.timestamp} — url={unsafeLinkProbe.url} — hostLinkAffordance={String(unsafeLinkProbe.hostLinkAffordance)}
            {unsafeLinkProbe.hostPolicyReason ? ` — hostPolicy: ${unsafeLinkProbe.hostPolicyReason}` : ''}
          </span>
        ) : null}
      </div>
    </aside>
  );
}
