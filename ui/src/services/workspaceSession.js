// Workspace descriptors remain in memory and are restored from the server.
// Persist only presentation preferences; never trust browser-stored content.
const preferenceKey = (id) => `agently.workspacePreferences:${id}`;
const reportSections = value => Object.fromEntries(Object.entries(value && typeof value === 'object' ? value : {})
  .filter(([key, section]) => key.length <= 2048 && typeof section === 'string' && section.length <= 512).slice(-100));
function readPreferences(storage, id) {
  try {
    const value = JSON.parse(storage?.getItem(preferenceKey(id)) || 'null');
    if (!value || value.version !== 1) return {};
    const ids = list => Array.isArray(list) ? [...new Set(list.filter(id => typeof id === 'string' && id.length <= 512))].slice(0, 100) : undefined;
    return {
      closedWindowIds: ids(value.closedWindowIds) || [],
      ...(ids(value.openWindowIds) ? {openWindowIds: ids(value.openWindowIds)} : {}),
      ...(typeof value.activeWindowId === 'string' ? {activeWindowId: value.activeWindowId} : {}),
      workspaceMode: value.workspaceMode === 'split' ? 'split' : 'focus',
      reportSections: reportSections(value.reportSections),
      ...(['conversation', 'workspace'].includes(value.activeSurface)
        ? {activeSurface: value.activeSurface, hasSurfaceSelection: true} : {}),
    };
  } catch (_) { return {}; }
}
const sessionsByClient = new WeakMap();
const fallbackClient = {};
function sessions(client) {
  const owner = client && typeof client === 'object' ? client : fallbackClient;
  if (!sessionsByClient.has(owner)) sessionsByClient.set(owner, new Map());
  return sessionsByClient.get(owner);
}
export function readWorkspaceSession(storage, conversationId) {
  const saved = sessions(storage).get(conversationId);
  if (saved) return saved;
  return {
    version: 1, conversationId, windows: [], activeWindowId: '',
    activeSurface: 'conversation', workspaceMode: 'focus', closedWindowIds: [],
    hasSurfaceSelection: false, ...readPreferences(storage, conversationId),
  };
}
export function updateWorkspaceSession(storage, conversationId, change) {
  const previous = readWorkspaceSession(storage, conversationId);
  const next = change(previous);
  sessions(storage).set(conversationId, next);
  try {
    storage?.setItem(preferenceKey(conversationId), JSON.stringify({
      version: 1, activeWindowId: next.activeWindowId, workspaceMode: next.workspaceMode,
      closedWindowIds: next.closedWindowIds, openWindowIds: next.openWindowIds,
      reportSections: reportSections(next.reportSections),
      ...(next.hasSurfaceSelection ? {activeSurface: next.activeSurface} : {}),
    }));
  } catch (_) { /* Storage may be unavailable; retain the in-memory preference. */ }
  return next;
}

export function readReportSection(storage, conversationId, reportKey) {
  if (!conversationId || !reportKey) return '';
  return readWorkspaceSession(storage, conversationId).reportSections?.[reportKey] || '';
}

export function rememberReportSection(storage, conversationId, reportKey, sectionId) {
  if (!conversationId || !reportKey) return;
  updateWorkspaceSession(storage, conversationId, state => ({...state,
    reportSections: reportSections({...state.reportSections, [reportKey]: sectionId}),
  }));
}
function mergeViewState(previous, incoming) {
  if (incoming === undefined) return previous;
  if (!previous || !incoming || typeof previous !== 'object' || typeof incoming !== 'object' || Array.isArray(previous) || Array.isArray(incoming)) return incoming;
  const result = { ...previous };
  for (const [name, value] of Object.entries(incoming)) result[name] = mergeViewState(previous[name], value);
  return result;
}
export function mergeWorkspaceSessionWindows(state, incoming = []) {
  const windows = [...state.windows];
  for (const candidate of incoming) {
    if (!candidate?.windowId || (candidate.conversationId && candidate.conversationId !== state.conversationId)) continue;
    const index = windows.findIndex((entry) => entry.windowId === candidate.windowId);
    if (index < 0) { windows.push(candidate); continue; }
    const previous = windows[index];
    const before = previous.workspaceObject;
    const after = candidate.workspaceObject;
    const beforeReset = Number(before?.content?.resetVersion || 0), afterReset = Number(after?.content?.resetVersion || 0);
    if (beforeReset > afterReset || (beforeReset === afterReset && Number(before?.revision || 0) > Number(after?.revision || 0))) continue;
    windows[index] = { ...previous, ...candidate,
      windowForm: mergeViewState(previous.windowForm, candidate.windowForm),
      dataSourceState: mergeViewState(previous.dataSourceState, candidate.dataSourceState),
      viewState: mergeViewState(previous.viewState, candidate.viewState),
      parameters: mergeViewState(previous.parameters, candidate.parameters),
      workspaceObject: after ? { ...after, origin: before?.origin?.turnId ? before.origin : after.origin } : before,
    };
  }
  return { ...state, windows };
}
export function workspaceHistory(state) {
  return state.windows.map((entry) => state.closedWindowIds.includes(entry.windowId) ? {
    ...entry, workspaceObject: { ...entry.workspaceObject, lifecycle: { ...entry.workspaceObject?.lifecycle, state: 'closed' } },
  } : entry);
}
