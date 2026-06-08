const DATASOURCE_FETCH_ROUTE = /\/v1\/api\/datasources\/[^/]+\/fetch$/;

export function prepareAgentlyDataConnectorRequest({
  url = '',
  queryParams = null,
  body = null,
  windowState = null,
} = {}) {
  const convID = String(windowState?.conversationId || '').trim();
  if (!convID) return;
  if (!DATASOURCE_FETCH_ROUTE.test(String(url || ''))) return;
  if (queryParams && typeof queryParams.append === 'function' && !queryParams.has('conversationId')) {
    queryParams.append('conversationId', convID);
  }
  if (body && typeof body === 'object' && !Array.isArray(body) && !body.conversationId) {
    body.conversationId = convID;
  }
}
