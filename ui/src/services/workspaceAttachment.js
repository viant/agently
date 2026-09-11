// Ownership comes from structured descriptors or the legacy transcript adapter.
// Originless objects use the global workspace switcher, never an inferred turn.
export function resolveWorkspaceAttachmentOwnerIndex(rows = [], workspaceWindow = null) {
  if (!workspaceWindow || !Array.isArray(rows)) return -1;
  const sourceTurnId = String(workspaceWindow?.workspaceObject?.origin?.turnId || workspaceWindow?.origin?.turnId || workspaceWindow?.sourceTurnId || workspaceWindow?.turnId || '').trim();
  if (!sourceTurnId) return -1;
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    if (String(row?.turnId || '').trim() !== sourceTurnId) continue;
    if (row?.kind === 'assistant' || row?.kind === 'iteration') return index;
  }
  return -1;
}
