export function dispatchForgeUIAction(detail = {}) {
  if (typeof window === 'undefined') return;
  const payload = {
    ...detail,
    callback: detail?.callback || null,
  };
  window.lastForgeUIAction = payload;
  window.dispatchEvent(new CustomEvent('forge-ui-action', { detail: payload }));
}

export function subscribeForgeUIAction(callback) {
  if (typeof window === 'undefined' || typeof callback !== 'function') {
    return () => {};
  }
  const handler = (event) => callback(event?.detail || null, event);
  window.addEventListener('forge-ui-action', handler);
  return () => window.removeEventListener('forge-ui-action', handler);
}

function summarizeForgeUIAction(detail = {}) {
  const eventName = String(detail?.eventName || detail?.callback?.eventName || 'forge_ui_submit').trim();
  const tableId = String(detail?.tableId || '').trim();
  const selectedCount = Array.isArray(detail?.selectedRows) ? detail.selectedRows.length : 0;
  const unselectedCount = Array.isArray(detail?.unselectedRows) ? detail.unselectedRows.length : 0;
  const changedCount = Array.isArray(detail?.changedRows) ? detail.changedRows.length : 0;
  return [
    `Forge UI callback: ${eventName}`,
    tableId ? `tableId=${tableId}` : '',
    `selected=${selectedCount}`,
    `unselected=${unselectedCount}`,
    `changed=${changedCount}`,
    '',
    JSON.stringify(detail, null, 2),
  ].filter(Boolean).join('\n');
}

export function connectForgeUIActionsToChat(submitMessage, contextProvider) {
  if (typeof submitMessage !== 'function') return () => {};
  return subscribeForgeUIAction(async (detail) => {
    const context = typeof contextProvider === 'function' ? contextProvider() : contextProvider;
    if (!context) return;
    const message = summarizeForgeUIAction(detail);
    try {
      await submitMessage({ context, message });
    } catch (err) {
      console.error('forge-ui-action submit failed', err);
    }
  });
}
