import {getActiveUIBridgeTransportContext, waitForUIBridgeReady} from 'forge/core';

export function isNativePublicPresentation(window) {
  const resource = window?.resource;
  return resource?.providerIdentity === 'internal' && resource?.authorityBinding === 'public-local-definition'
    && String(resource.uri || '').startsWith('window://workspace/');
}

// The first GET supplies the authored frame; the native store then assigns its
// local instance id. The second GET keeps that exact selector while the host
// binds the completed OPEN to the existing negotiated UI transport.
export async function admitNativePresentation(window, instanceId, loadWindow, parameters, targetContext, routing = {}) {
  if (!isNativePublicPresentation(window)) return window;
  if (!await waitForUIBridgeReady(2500)) throw new Error('Workspace UI transport is unavailable');
  const transport = getActiveUIBridgeTransportContext();
  if (!transport?.sessionId || !transport?.clientId || !instanceId) throw new Error('Workspace UI transport is unavailable');
  return loadWindow(window.windowKey, parameters, targetContext, {
    ...routing, clientId: transport.clientId, windowId: instanceId,
    resolvedResource: structuredClone(window.resource),
  });
}
