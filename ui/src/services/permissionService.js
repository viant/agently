import {ensureUIBridgeClientId, getActiveUIBridgeTransportContext} from 'forge/core';
import {client} from './agentlyClient';

// applyPermission is intentionally separate from both metadata and datasource
// fetches. Forge calls it only after complete metadata and a concrete resource
// context are available.
export async function applyPermission({
  windowId = '',
  windowKey = '',
  completeMetadata = null,
  resource = {},
  windowParams = {},
  conversationId = '',
  targetContext = null,
} = {}) {
  const transport = getActiveUIBridgeTransportContext();
  const instance = String(windowId || (windowKey === 'chat/new' ? 'chat/new' : '')).trim();
  return client.applyPermission(windowKey, {
    conversationId: String(conversationId || '').trim(),
    ...(instance && transport?.sessionId ? {windowId: instance, clientId: transport.clientId, bridgeSessionId: transport.sessionId}
      : windowKey === 'chat/new' ? {windowId: 'chat/new', clientId: ensureUIBridgeClientId()} : {}),
    ...(completeMetadata?.resource ? {resolvedResource: completeMetadata.resource} : {}),
    resource: resource && typeof resource === 'object' && !Array.isArray(resource) ? resource : {},
    windowParams: windowParams && typeof windowParams === 'object' && !Array.isArray(windowParams) ? windowParams : {},
    targetContext: targetContext || undefined,
  });
}
