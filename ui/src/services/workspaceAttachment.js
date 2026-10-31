import {inlineReportWorkspaceDescriptor, canonicalReportSources} from './workspaceAdapters.js';
// Ownership comes from structured descriptors or the legacy transcript adapter.
// Originless objects use the global workspace switcher, never an inferred turn.
export function resolveWorkspaceAttachmentOwnerIndex(rows = [], workspaceWindow = null) {
  if (!workspaceWindow || !Array.isArray(rows)) return -1;
  const sourceTurnId = String(workspaceWindow?.workspaceObject?.origin?.turnId || workspaceWindow?.workspaceObject?.lastActivatedBy?.turnId || workspaceWindow?.origin?.turnId || workspaceWindow?.sourceTurnId || workspaceWindow?.turnId || '').trim();
  if (!sourceTurnId) return -1;
  for (let index = rows.length - 1; index >= 0; index -= 1) {
    const row = rows[index];
    if (String(row?.turnId || '').trim() !== sourceTurnId) continue;
    if (row?.kind === 'assistant' || row?.kind === 'iteration') return index;
  }
  return -1;
}


export function workspaceEntryFromAttachment(attachment, conversationId = '') {
  const object = attachment?.workspaceObject;
  const content = object?.content;
  if (attachment?.kind !== 'workspaceObject' || !object?.objectId || object.objectId !== attachment.objectId || !content?.windowId || !content?.windowKey) return null;
  if (object.conversationId && conversationId && object.conversationId !== conversationId) return null;
  return {windowId: content.windowId, windowKey: content.windowKey, conversationId: object.conversationId || conversationId,
    presentation: 'hosted', region: 'chat.top', parameters: content.parameters || {}, workspaceObject: object,
    navigation: {...object.navigation, label: attachment.label || object.navigation?.label, icon: attachment.icon || object.navigation?.icon}};
}

export function projectedWorkspaceEntries(rows = [], conversationId = '', {includeInlineReports = false, includePendingReports = false} = {}) {
  const entries = new Map();
  for (const row of rows) {
    if (row.hostEffectsAllowed === false || row.connectionProfile === 'standard') continue;
    for (const attachment of row.attachments || []) {
      const entry = workspaceEntryFromAttachment(attachment, conversationId);
      if (!entry) continue;
      const previous = entries.get(entry.workspaceObject.objectId);
      if (!previous || Number(entry.workspaceObject.revision || 0) >= Number(previous.workspaceObject.revision || 0)) entries.set(entry.workspaceObject.objectId, entry);
    }
  }
  if (includeInlineReports) for (const {row, messageId, renderedContent} of canonicalReportSources(rows)) {
    for (const assembly of renderedContent?.reports || []) {
      if (assembly.status === 'rendering' && !includePendingReports) continue;
      if ((renderedContent.diagnostics || []).some(item => item.reportId === assembly.id)) continue;
      const entry = inlineReportWorkspaceDescriptor({assembly, conversationId, messageId, turnId: row.turnId});
      if (!entry) continue;
      const previous = entries.get(entry.workspaceObject.objectId);
      if (!previous || Number(entry.workspaceObject.content.resetVersion) > Number(previous.workspaceObject.content.resetVersion || 0)
        || (entry.workspaceObject.content.resetVersion === (previous.workspaceObject.content.resetVersion || 0)
          && Number(entry.workspaceObject.revision) >= Number(previous.workspaceObject.revision))) entries.set(entry.workspaceObject.objectId, entry);
    }
  }
  return [...entries.values()];
}
