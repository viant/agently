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

export function normalizeCanonicalReportAssembly(assembly = {}) {
  let source = assembly.source;
  if (typeof source === 'string') {
    try { source = JSON.parse(source); } catch (_) { source = null; }
  }
  return {...assembly, authoritative: true,
    source: source && typeof source === 'object' && !Array.isArray(source) ? source : {},
    dataSources: Object.fromEntries(Object.entries(assembly.dataSources || {}).map(([id, value]) =>
      [id, {...value, id, data: value?.data ?? value?.payload}]))};
}

// Conversation, turn (or message), scope and report ID distinguish outputs from revisions.
export function inlineReportWorkspaceDescriptor({assembly, conversationId, messageId, turnId = ''}) {
  if (!conversationId || !messageId || !assembly?.id || assembly.status !== 'committed') return null;
  const normalized = normalizeCanonicalReportAssembly(assembly);
  if (!Object.keys(normalized.source).length || !['dashboard-v1', 'report-document-v1'].includes(assembly.grammar || 'dashboard-v1')) return null;
  const scope = assembly.scope || 'message';
  const windowId = `report:${[conversationId, turnId || messageId, scope, assembly.id].map(encodeURIComponent).join(':')}`;
  return {windowId, windowKey: 'report/workspace', windowTitle: normalized.source.title || 'Report',
    conversationId, presentation: 'hosted', region: 'chat.top', hostOpenState: 'historical_replay',
    workspaceObject: {version: 1, objectId: `workspace:${windowId}`, conversationId, kind: 'report',
      revision: Number(assembly.sequence || 0), origin: {turnId, messageId},
      content: {renderer: 'inlineReport', windowId, windowKey: 'report/workspace', messageId, turnId, reportId: assembly.id, reportScope: scope, resetVersion: Number(assembly.resetVersion || 0)},
      navigation: {label: normalized.source.title || 'Report', icon: 'document'},
      placement: {preferred: 'workspace', allowed: ['workspace'], initialWorkspaceMode: 'split'},
      lifecycle: {state: 'ready', restorePolicy: 'conversation', refreshPolicy: 'explicit'},
      capabilities: {close: true, focus: true, split: true, reopen: true}}};
}

export function canonicalReportSources(rows = []) {
  return rows.flatMap(row => {
    if (row.hostEffectsAllowed === false || row.connectionProfile === 'standard') return [];
    if (row.kind === 'assistant') return [{row, messageId: row.messageId || row.renderKey, renderedContent: row.renderedContent}];
    if (row.kind !== 'iteration') return [];
    const rounds = (row.rounds || []).filter(round => round.hostEffectsAllowed !== false && round.connectionProfile !== 'standard')
      .map(round => ({row, messageId: round.modelSteps?.[0]?.assistantMessageId || round.pageId || round.renderKey, renderedContent: round.renderedContent}));
    return [...rounds, ...(row.assistantFinal ? [{row, messageId: row.assistantFinal.messageId || row.assistantFinal.renderKey, renderedContent: row.assistantFinal.renderedContent}] : [])];
  });
}
