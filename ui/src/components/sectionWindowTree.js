// Follow parent links rather than resource names so local and remote windows
// share the same section ownership, including nested and floating children.
export function sectionWindowIds(windows, rootId) {
  const entries = Array.isArray(windows) ? windows : [];
  const byId = new Map(entries.map(entry => [entry.windowId, entry]));
  if (!rootId || !byId.has(rootId)) return new Set();
  return new Set(entries.filter(entry => {
    const visited = new Set();
    let current = entry;
    while (current && !visited.has(current.windowId)) {
      if (current.windowId === rootId) return true;
      visited.add(current.windowId);
      current = byId.get(current.parentKey);
    }
    return false;
  }).map(entry => entry.windowId));
}

export function belongsToAppSection(windows, entry) {
  const byId = new Map((Array.isArray(windows) ? windows : []).map(win => [win.windowId, win]));
  const visited = new Set();
  for (let current = entry; current && !visited.has(current.windowId); current = byId.get(current.parentKey)) {
    if (current.region === 'app.section') return true;
    visited.add(current.windowId);
  }
  return false;
}
