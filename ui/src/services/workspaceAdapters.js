// Renderer adapters share identity, origin and lifecycle with hosted Forge views.
export function mcpWorkspaceDescriptor({uri, conversationId, title, turnId = '', toolCallId = '', historical = true}) {
  const windowId = `mcpui:${encodeURIComponent(conversationId)}:${encodeURIComponent(uri)}`;
  return {
    windowId, hostOpenState: historical ? 'historical_replay' : 'fresh',
    workspaceObject: {
      version: 1, objectId: `workspace:${windowId}`, conversationId, kind: 'mcpApp',
      origin: {turnId, toolCallId}, content: {renderer: 'mcpApp', windowId, resourceUri: uri},
      navigation: {label: title || 'Interactive app', icon: 'application'},
      placement: {preferred: 'workspace', allowed: ['inline', 'workspace'], initialWorkspaceMode: 'focus'},
      lifecycle: {state: 'opening', restorePolicy: 'conversation', refreshPolicy: 'explicit'},
      capabilities: {close: true, focus: true, split: true},
    },
  };
}

export function toolFeedWorkspaceDescriptor(feed) {
  const windowId = `toolfeed:${encodeURIComponent(feed.conversationId)}:${encodeURIComponent(feed.feedId)}`;
  return {windowId, workspaceObject: {
    version: 1, objectId: `workspace:${windowId}`, conversationId: feed.conversationId, kind: 'toolOutput',
    origin: {turnId: feed.turnId || ''}, content: {renderer: 'toolFeed', windowId, feedId: feed.feedId},
    navigation: {label: feed.title || 'Tool output', icon: 'document'},
    placement: {preferred: 'workspace', allowed: ['inline', 'workspace', 'overlay'], initialWorkspaceMode: 'focus'},
    lifecycle: {state: 'opening', restorePolicy: 'conversation', refreshPolicy: 'explicit'},
    capabilities: {close: true, focus: true, split: true},
  }};
}
