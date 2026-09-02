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
    if (convID && !body.conversationId) body.conversationId = convID;
  }
}
