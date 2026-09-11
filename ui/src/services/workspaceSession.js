// One conversation-scoped presentation record. Resource authorization stays with
// the server/renderer; this cache never grants access or executes restored actions.
const key = (id) => `agently.workspaceSession:${id}`;
const parse = (storage, name, fallback) => {
  try { return JSON.parse(storage?.getItem(name) || 'null') ?? fallback; } catch { return fallback; }
};
export function readWorkspaceSession(storage, conversationId) {
  const saved = parse(storage, key(conversationId), null);
  if (saved?.version === 1 && saved.conversationId === conversationId) return saved;
  const legacy = parse(storage, `agently.workspaceState:${conversationId}`, null);
  const windows = legacy ? (Array.isArray(legacy.windows) ? legacy.windows : [legacy]) : [];
  return {
    version: 1, conversationId, windows,
    activeWindowId: storage?.getItem(`agently.selectedWorkspaceWindowId:${conversationId}`) || '',
    activeSurface: storage?.getItem(`agently.activeSurface:${conversationId}`) === 'workspace' ? 'workspace' : 'conversation',
    workspaceMode: storage?.getItem(`agently.workspacePresentationMode:${conversationId}`) === 'full' ? 'focus' : 'split',
    closedWindowIds: parse(storage, `agently.dismissedWorkspaceWindowIds:${conversationId}`, []),
  };
}
export function updateWorkspaceSession(storage, conversationId, change) {
  const previous = readWorkspaceSession(storage, conversationId);
  const next = change(previous);
  try { storage?.setItem(key(conversationId), JSON.stringify(next)); } catch { /* Session storage may be unavailable. */ }
  return next;
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
    if (Number(before?.revision || 0) > Number(after?.revision || 0)) continue;
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
