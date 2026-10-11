import {findMetadataSignal} from 'forge/core';

// Only current server-provided metadata supplies an owning primitive. Local
// window keys, selected agents and datasource IDs never imply a private owner.
export function lookupOwnerFromContext(context, {reactive = false} = {}) {
  const windowID = context?.identity?.windowId;
  const signal = windowID ? findMetadataSignal(windowID) : null;
  const current = signal ? (reactive ? signal.value : signal.peek?.()) : context?.metadata;
  const resource = current?.resource;
  if (!resource || typeof resource !== 'object' || !/^(window|template|intent):\/\//.test(resource.uri || '')) return null;
  return {owner: {uri: resource.uri, revision: resource.kind === 'working' ? 'working' : resource.revision || ''}, providerIdentity: resource.providerIdentity || '', authorityBinding: resource.authorityBinding || ''};
}

