import React from 'react';
import StandaloneForgeWindowSurface from './StandaloneForgeWindowSurface.jsx';
import { useSetting } from 'forge/core';
import { installForgeGuestBridge } from '../../services/mcpApps/forgeGuestBridge.js';
import MCPUIVerifierRouteDebug from './MCPUIVerifierRouteDebug.jsx';
import { MCPUI_VERIFIER_ROUTE_WINDOW_KEY } from '../../services/mcpApps/mcpuiVerifierRouteDiagnostics.js';

export function parseJSONParam(raw = '') {
  const text = String(raw || '').trim();
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch (_) {
    return {};
  }
}

export function buildWindowPayload(windowKey, payload, parameters) {
  const data = payload?.data && typeof payload.data === 'object' ? payload.data : {};
  const normalizedWindowKey = String(windowKey || '').trim();
  const normalizedWindowId = `mcpui:${normalizedWindowKey}`;
  return {
    ...data,
    // A signed OPEN supplies one complete admitted frame. Keep its definition
    // and original parameters together; dynamic frames need no second GET.
    ...(data?.selectionBinding && data?.view ? {
      inlineMetadata: JSON.parse(JSON.stringify(data)),
      openedWindowParameters: JSON.parse(JSON.stringify(parameters && typeof parameters === 'object' ? parameters : {})),
    } : {}),
    windowId: normalizedWindowId,
    windowKey: normalizedWindowKey,
    // Forge report/export flows derive draft artifact identity from container
    // ids. Hosted MCPUI windows historically only carried windowId/windowKey,
    // which left reportBuilder draft export inactive even when runtime rows
    // were visible. Mirror the hosted window identity onto Forge's conventional
    // id/stateKey fields so draft export can build a stable artifactRef.
    id: String(data?.id || normalizedWindowKey || normalizedWindowId).trim() || normalizedWindowId,
    stateKey: String(data?.stateKey || data?.id || normalizedWindowKey || normalizedWindowId).trim() || normalizedWindowId,
    windowTitle: String(data?.namespace || normalizedWindowKey || 'Workspace').trim() || 'Workspace',
    presentation: String(data?.presentation || 'hosted').trim() || 'hosted',
    region: String(data?.region || 'mcpui.bubble').trim() || 'mcpui.bubble',
    parameters: parameters && typeof parameters === 'object' ? parameters : {},
    isInTab: true,
  };
}

export function appendTargetContext(params, targetContext = {}) {
  const platform = String(targetContext.platform || '').trim();
  const formFactor = String(targetContext.formFactor || '').trim();
  const surface = String(targetContext.surface || '').trim();
  const capabilities = Array.isArray(targetContext.capabilities)
    ? targetContext.capabilities.map((item) => String(item || '').trim()).filter(Boolean)
    : [];
  if (platform) params.set('platform', platform);
  if (formFactor) params.set('formFactor', formFactor);
  if (surface) params.set('surface', surface);
  capabilities.forEach((capability) => params.append('capabilities', capability));
}

export function loadSelectedWindow(windowKey, parameters, targetContext = {}) {
    const requestParams = new URLSearchParams();
    appendTargetContext(requestParams, targetContext);
    // Selection parameters must reach server admission before rendering the
    // window. Client parameters describe the selection; the server resolves
    // the protected resource and decides its capabilities.
    requestParams.set('applyPermission', 'true');
    requestParams.set('windowParams', JSON.stringify(parameters));
    const query = requestParams.toString();
    const requestURL = `/v1/api/agently/forge/window/${encodeURIComponent(windowKey)}${query ? `?${query}` : ''}`;
    return fetch(requestURL, {
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
      .then(payload => buildWindowPayload(windowKey, payload, parameters));
}

export default function MCPUIForgeWindowPage() {
  const params = React.useMemo(() => new URLSearchParams(window.location.search), []);
  const windowKey = String(params.get('windowKey') || '').trim();
  const windowParams = React.useMemo(() => parseJSONParam(params.get('windowParams')), [params]);
  const { targetContext = {} } = useSetting();
  const targetKey = React.useMemo(() => JSON.stringify({
    platform: String(targetContext.platform || '').trim(),
    formFactor: String(targetContext.formFactor || '').trim(),
    surface: String(targetContext.surface || '').trim(),
    capabilities: Array.isArray(targetContext.capabilities)
      ? targetContext.capabilities.map((item) => String(item || '').trim()).filter(Boolean).sort()
      : [],
  }), [targetContext]);
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
    loadSelectedWindow(windowKey, windowParams, targetContext)
      .then((openedWindow) => {
        if (!active) return;
        setState({
          loading: false,
          error: '',
          window: openedWindow,
          targetKey,
        });
      })
      .catch((err) => {
        if (!active) return;
        setState({
          loading: false,
          error: err?.message || 'Failed to load workspace window',
          window: null,
          targetKey,
        });
      });
    return () => {
      active = false;
    };
  }, [windowKey, windowParams, targetContext, targetKey]);

  if (state.loading || !state.error && state.targetKey !== targetKey) {
    return <div style={{ padding: 20, color: '#475467', fontFamily: 'var(--app-font-family, ui-sans-serif, system-ui, sans-serif)' }}>Loading workspace window...</div>;
  }
  if (state.error) {
    return <div style={{ padding: 20, color: '#b42318', fontFamily: 'var(--app-font-family, ui-sans-serif, system-ui, sans-serif)' }}>{state.error}</div>;
  }
  return (
    <div style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', background: '#f8fafc' }}>
      {windowKey === MCPUI_VERIFIER_ROUTE_WINDOW_KEY ? <MCPUIVerifierRouteDebug /> : null}
      <StandaloneForgeWindowSurface selectedWindow={state.window} loadWindow={loadSelectedWindow} targetContext={targetContext} />
    </div>
  );
}
