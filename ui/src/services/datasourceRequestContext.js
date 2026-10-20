import {findMetadataSignal, getActiveUIBridgeTransportContext} from 'forge/core';

const DATASOURCE_FETCH_ROUTE = /\/v1\/api\/datasources\/[^/]+\/fetch$/;
const WINDOW_METADATA_ROUTE = /\/v1\/api\/agently\/forge\/window\/[^/]+$/;
const isObject = value => !!value && typeof value === 'object' && !Array.isArray(value);
const detached = value => JSON.parse(JSON.stringify(value));

export function prepareAgentlyDataConnectorRequest({
  url = '',
  queryParams = null,
  body = null,
  windowState = null,
  headers = null,
} = {}) {
  // Native workspace data uses the authenticated UI session. The chat ID is
  // presentation state and can refer to an unsaved or deleted conversation.
  const workspaceData = windowState?.presentation === 'hosted' && windowState?.region === 'chat.top'
    && !windowState?.workspaceObject?.origin?.turnId && !windowState?.mcpUI
    && windowState?.workspaceObject?.content?.renderer !== 'toolFeed';
  const convID = workspaceData ? '' : String(windowState?.conversationId || '').trim();
  const requestURL = String(url || '');
  const metadataRequest = WINDOW_METADATA_ROUTE.test(requestURL);
  if (!metadataRequest && !DATASOURCE_FETCH_ROUTE.test(requestURL)) return;
  const transport = getActiveUIBridgeTransportContext();
  if (transport?.sessionId && headers) {
    try {
      const pageURL = globalThis.window?.location?.href || globalThis.location?.href || transport.url;
      const bridgeURL = new URL(transport.url, pageURL);
      const destination = new URL(requestURL, pageURL);
      if (destination.origin === new URL(pageURL).origin && bridgeURL.origin === destination.origin) {
        if (windowState?.windowId && queryParams && !queryParams.has('windowId')) queryParams.append('windowId',String(windowState.windowId));
        if (metadataRequest && windowState?.windowId && queryParams && !queryParams.has('clientId')) queryParams.append('clientId', transport.clientId);
        if (typeof headers.set === 'function') headers.set(transport.sessionHeader, transport.sessionId);
        else headers[transport.sessionHeader] = transport.sessionId;
      }
    } catch (_) { /* Invalid/foreign destinations receive no UI session. */ }
  }
  // Bind the definition already selected by the server to the browser instance.
  // This is transport routing only, without another authorization/OPEN request.
  if (metadataRequest) return;
  if (workspaceData) {
    queryParams?.delete?.('conversationId');
    if (isObject(body)) delete body.conversationId;
  }
  if (convID && queryParams && typeof queryParams.append === 'function' && !queryParams.has('conversationId')) {
    queryParams.append('conversationId', convID);
  }
  if (body && typeof body === 'object' && !Array.isArray(body)) {
    // A server-issued binding is provenance, never a UI grant. Keep its
    // complete original admission frame together; the backend verifies it.
    const currentMetadata = (windowState?.windowId ? findMetadataSignal(windowState.windowId)?.peek?.() : null) || windowState?.metadata;
    const admitted = [windowState?.inlineMetadata, windowState, currentMetadata]
      .find(candidate => isObject(candidate?.selectionBinding));
    if (admitted) {
      body.windowParameters = detached(windowState?.openedWindowParameters || windowState?.parameters || {});
      body.selectionBinding = detached(admitted.selectionBinding);
      if (isObject(admitted.resource)) body.resource = detached(admitted.resource);
      else delete body.resource;
      if (isObject(admitted.resourceTarget)) body.target = detached(admitted.resourceTarget);
      else delete body.target;
      if (convID && !body.conversationId) body.conversationId = convID;
      return;
    }
    if (windowState?.parameters && typeof windowState.parameters === 'object') body.windowParameters = {...windowState.parameters};
    if (convID && !body.conversationId) body.conversationId = convID;
    const metadata = currentMetadata;
    const resource = metadata?.resource || windowState?.resource;
    if (resource && typeof resource === 'object' && !Array.isArray(resource)) {
      // The backend independently verifies the transported pin.
      body.resource = {...resource};
      const target = metadata?.resourceTarget || windowState?.resourceTarget;
      if (target && typeof target === 'object' && !Array.isArray(target)) body.target = {...target};
    }
  }
}
