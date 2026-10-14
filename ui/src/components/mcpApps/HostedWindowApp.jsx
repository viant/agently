import React from 'react';
import { registerContainerRenderer } from 'forge/core';
import HostedAppRenderer from './HostedAppRenderer.jsx';

// The backend resource reader validates the installed URI, owning window,
// selected target, actor, renderer origin and sandbox on every read.
export function HostedWindowApp({ container, context, isActive = true }) {
  const state = context?.windowState || {};
  const metadata = context?.metadata || state.metadata || state;
  const resource = metadata.resource;
  const target = metadata.resourceTarget;
  const uri = container?.mcpApp?.resourceURI;
  if (!resource?.uri || !resource?.providerIdentity || !target?.selectionToken ||
      typeof uri !== 'string' || !uri.startsWith('ui://')) {
    return <div role="alert">Hosted app resource authority is unavailable.</div>;
  }
  return <div style={{ display: isActive ? 'flex' : 'none', flex: 1, minHeight: 0, flexDirection: 'column' }} aria-hidden={!isActive}><HostedAppRenderer uri={uri} title={container.title || state.title || 'Hosted app'}
    resourceContext={{ resource, resourceTarget: target }} /></div>;
}

registerContainerRenderer('mcpApp', HostedWindowApp);
