import React from 'react';
import { WindowContent } from 'forge/components';
import { installForgeGuestBridge } from '../../services/mcpApps/forgeGuestBridge.js';
import MCPUIVerifierRouteDebug from './MCPUIVerifierRouteDebug.jsx';
import { MCPUI_VERIFIER_ROUTE_WINDOW_KEY } from '../../services/mcpApps/mcpuiVerifierRouteDiagnostics.js';

function parseJSONParam(raw = '') {
  const text = String(raw || '').trim();
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch (_) {
    return {};
  }
}

function buildWindowPayload(windowKey, payload, parameters) {
  const data = payload?.data && typeof payload.data === 'object' ? payload.data : {};
  return {
    ...data,
    windowId: `mcpui:${windowKey}`,
    windowKey,
    windowTitle: String(data?.namespace || windowKey || 'Workspace').trim() || 'Workspace',
    presentation: String(data?.presentation || 'hosted').trim() || 'hosted',
    region: String(data?.region || 'mcpui.bubble').trim() || 'mcpui.bubble',
    parameters: parameters && typeof parameters === 'object' ? parameters : {},
    isInTab: true,
  };
}

export default function MCPUIForgeWindowPage() {
  const params = React.useMemo(() => new URLSearchParams(window.location.search), []);
  const windowKey = String(params.get('windowKey') || '').trim();
  const windowParams = React.useMemo(() => parseJSONParam(params.get('windowParams')), [params]);
  const [state, setState] = React.useState({
    loading: true,
    error: '',
    window: null,
  });

  React.useEffect(() => installForgeGuestBridge(window), []);

  React.useEffect(() => {
    let active = true;
    if (!windowKey) {
      setState({ loading: false, error: 'windowKey is required', window: null });
      return () => {};
    }
    fetch(`/v1/api/agently/forge/window/${encodeURIComponent(windowKey)}`, {
      method: 'GET',
      credentials: 'include',
      headers: { Accept: 'application/json' },
    })
      .then(async (response) => {
        if (!response.ok) {
          const text = await response.text().catch(() => '');
          throw new Error(text || `window fetch failed (${response.status})`);
        }
        return response.json();
      })
      .then((payload) => {
        if (!active) return;
        setState({
          loading: false,
          error: '',
          window: buildWindowPayload(windowKey, payload, windowParams),
        });
      })
      .catch((err) => {
        if (!active) return;
        setState({
          loading: false,
          error: err?.message || 'Failed to load workspace window',
          window: null,
        });
      });
    return () => {
      active = false;
    };
  }, [windowKey, windowParams]);

  if (state.loading) {
    return <div style={{ padding: 20, color: '#475467', fontFamily: 'ui-sans-serif, system-ui, sans-serif' }}>Loading workspace window...</div>;
  }
  if (state.error) {
    return <div style={{ padding: 20, color: '#b42318', fontFamily: 'ui-sans-serif, system-ui, sans-serif' }}>{state.error}</div>;
  }
  return (
    <div style={{ minHeight: '100vh', background: '#f8fafc' }}>
      {windowKey === MCPUI_VERIFIER_ROUTE_WINDOW_KEY ? <MCPUIVerifierRouteDebug /> : null}
      <WindowContent window={state.window} isInTab />
    </div>
  );
}
