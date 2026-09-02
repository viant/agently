import {client} from './agentlyClient';

// applyPermission is intentionally separate from both metadata and datasource
// fetches. Forge calls it only after complete metadata and a concrete resource
// context are available.
export async function applyPermission({
  windowKey = '',
  resource = {},
  windowParams = {},
  conversationId = '',
  targetContext = null,
} = {}) {
  return client.applyPermission(windowKey, {
    conversationId: String(conversationId || '').trim(),
    resource: resource && typeof resource === 'object' && !Array.isArray(resource) ? resource : {},
    windowParams: windowParams && typeof windowParams === 'object' && !Array.isArray(windowParams) ? windowParams : {},
    targetContext: targetContext || undefined,
  });
}
