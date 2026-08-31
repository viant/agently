const pendingDrafts = new Map();

function key(feedId = '', conversationId = '') {
  const feed = String(feedId || '').trim();
  const conversation = String(conversationId || '').trim();
  return feed && conversation ? `${conversation}::${feed}` : '';
}

export function savePendingFeedDraft(feedId, conversationId, snapshot = {}) {
  const draftKey = key(feedId, conversationId);
  if (!draftKey) return false;
  pendingDrafts.set(draftKey, snapshot && typeof snapshot === 'object' ? snapshot : {});
  return true;
}

export function clearPendingFeedDraft(feedId, conversationId) {
  const draftKey = key(feedId, conversationId);
  if (!draftKey) return false;
  return pendingDrafts.delete(draftKey);
}

export function restorePendingFeedDraft(feedId, conversationId, sourceSignature = '', context = null) {
  const draftKey = key(feedId, conversationId);
  const pending = draftKey ? pendingDrafts.get(draftKey) : null;
  if (!pending) return 'none';
  if (String(pending.sourceSignature || '') !== String(sourceSignature || '')) {
    pendingDrafts.delete(draftKey);
    return 'cleared';
  }

  const formRef = String(pending.formDataSourceRef || '').trim();
  if (formRef) {
    const target = context?.Context?.(formRef);
    target?.handlers?.dataSource?.setFormData?.({ values: pending.formData || {} });
    if (target?.signals?.formStatus) target.signals.formStatus.value = { dirty: true };
  }
  for (const [ref, state] of Object.entries(pending.collections || {})) {
    const target = context?.Context?.(ref);
    target?.handlers?.dataSource?.setCollection?.(Array.isArray(state?.rows) ? state.rows : []);
    target?.handlers?.dataSource?.setSelection?.({ selected: Array.isArray(state?.selectedRows) ? state.selectedRows : [] });
    if (target?.signals?.formStatus) target.signals.formStatus.value = { dirty: true };
  }
  return 'restored';
}

export function resetPendingFeedDrafts() {
  pendingDrafts.clear();
}

export function markFeedDataSourcesDirty(context = null, refs = []) {
  const normalized = (Array.isArray(refs) ? refs : [])
    .map((value) => String(value || '').trim())
    .filter((value, index, values) => value && values.indexOf(value) === index);
  for (const ref of normalized) {
    const status = context?.Context?.(ref)?.signals?.formStatus;
    if (status) status.value = { ...(status.peek?.() || status.value || {}), dirty: true };
  }
  return normalized.length;
}
