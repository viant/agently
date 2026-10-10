import {findMetadataSignal} from 'forge/core';

const DATASOURCE_FETCH_ROUTE = /\/v1\/api\/datasources\/[^/]+\/fetch$/;
const WINDOW_METADATA_ROUTE = /\/v1\/api\/agently\/forge\/window\/[^/]+$/;

export function prepareAgentlyDataConnectorRequest({
  url = '',
  queryParams = null,
  body = null,
  windowState = null,
} = {}) {
  const convID = String(windowState?.conversationId || '').trim();
  const requestURL = String(url || '');
  if (WINDOW_METADATA_ROUTE.test(requestURL)) {
    return;
  }
  if (!DATASOURCE_FETCH_ROUTE.test(requestURL)) return;
  if (convID && queryParams && typeof queryParams.append === 'function' && !queryParams.has('conversationId')) {
    queryParams.append('conversationId', convID);
  }
  if (body && typeof body === 'object' && !Array.isArray(body)) {
    if (windowState?.parameters && typeof windowState.parameters === 'object') body.windowParameters = {...windowState.parameters};
    if (convID && !body.conversationId) body.conversationId = convID;
    const metadata = (windowState?.windowId ? findMetadataSignal(windowState.windowId)?.peek?.() : null) || windowState?.metadata;
    const resource = metadata?.resource || windowState?.resource;
    if (resource && typeof resource === 'object' && !Array.isArray(resource)) {
      // The backend independently verifies the transported pin.
      body.resource = {...resource};
      const target = metadata?.resourceTarget || windowState?.resourceTarget;
      if (target && typeof target === 'object' && !Array.isArray(target)) body.target = {...target};
    }
  }
}
