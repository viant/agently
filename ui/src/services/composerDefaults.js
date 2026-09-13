const key = metadata => `agently.composerDefaults.v1:${metadata?.workspaceId || metadata?.workspaceRoot || 'default'}`;
export function readComposerDefaults(metadata) {
  try { const value = JSON.parse(localStorage.getItem(key(metadata)) || '{}'); return {agent: typeof value.agent === 'string' ? value.agent : '', model: typeof value.model === 'string' ? value.model : ''}; } catch { return {}; }
}
export function saveComposerDefaults(metadata, value) {
  localStorage.setItem(key(metadata), JSON.stringify({agent: value.agent || '', model: value.model || ''}));
}
