// Temporary render state only. Durable workspace descriptors are restored from
// the server conversation transcript; browser storage is never authoritative.
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
    activeSurface: 'conversation', workspaceMode: 'split', closedWindowIds: [],
  };
}
export function updateWorkspaceSession(storage, conversationId, change) {
  const previous = readWorkspaceSession(storage, conversationId);
  const next = change(previous);
  sessions(storage).set(conversationId, next);
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
