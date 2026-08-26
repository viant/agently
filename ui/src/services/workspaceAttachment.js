function normalizeToolName(value = '') {
  return String(value || '').trim().toLowerCase().replace(/:/g, '/');
}

function iterationOpenedWorkspace(row = null) {
  if (row?.kind !== 'iteration') return false;
  const workspaceTools = new Set(['ui/view/open', 'ui/window/open', 'ui/window/show']);
  return (Array.isArray(row?.rounds) ? row.rounds : []).some((round) => (
    (Array.isArray(round?.toolCalls) ? round.toolCalls : []).some((tool) => {
      const status = String(tool?.status || '').trim().toLowerCase();
      return (!status || ['completed', 'succeeded', 'success', 'done'].includes(status))
        && workspaceTools.has(normalizeToolName(tool?.toolName));
    })
  ));
}

export function resolveWorkspaceAttachmentOwnerIndex(rows = [], workspaceWindow = null) {
  if (!workspaceWindow || !Array.isArray(rows)) return -1;
  const explicitTurnId = String(workspaceWindow?.sourceTurnId || workspaceWindow?.turnId || '').trim();
  let sourceTurnId = explicitTurnId;
  if (!sourceTurnId) {
    for (let index = rows.length - 1; index >= 0; index -= 1) {
      if (iterationOpenedWorkspace(rows[index])) {
        sourceTurnId = String(rows[index]?.turnId || '').trim();
        if (sourceTurnId) break;
      }
    }
  }
  if (!sourceTurnId) return -1;
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    if (String(row?.turnId || '').trim() !== sourceTurnId) continue;
    if (row?.kind === 'assistant' || row?.kind === 'iteration') return index;
  }
  return -1;
}
