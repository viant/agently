export const MCPUI_APPROVAL_REQUEST_EVENT = 'agently:mcpui-approval-request';
export const MCPUI_APPROVAL_OUTCOME_EVENT = 'agently:mcpui-approval-outcome';

export function normalizeMCPUIApprovalRequest(detail = null) {
  const payload = detail && typeof detail === 'object' ? detail : {};
  const approvalId = String(payload.approvalId || '').trim();
  if (!approvalId) return null;
  return {
    approvalId,
    resourceUri: String(payload.resourceUri || '').trim(),
    toolName: String(payload.toolName || '').trim(),
    title: String(payload.title || '').trim(),
  };
}

export function dispatchMCPUIApprovalRequest(detail = null) {
  const normalized = normalizeMCPUIApprovalRequest(detail);
  if (!normalized || typeof window === 'undefined' || typeof window.dispatchEvent !== 'function') {
    return false;
  }
  window.dispatchEvent(new CustomEvent(MCPUI_APPROVAL_REQUEST_EVENT, { detail: normalized }));
  return true;
}

export function subscribeMCPUIApprovalRequests(handler) {
  if (typeof window === 'undefined' || typeof window.addEventListener !== 'function' || typeof handler !== 'function') {
    return () => {};
  }
  const listener = (event) => {
    const normalized = normalizeMCPUIApprovalRequest(event?.detail);
    if (!normalized) return;
    handler(normalized, event);
  };
  window.addEventListener(MCPUI_APPROVAL_REQUEST_EVENT, listener);
  return () => window.removeEventListener(MCPUI_APPROVAL_REQUEST_EVENT, listener);
}

export function normalizeMCPUIApprovalOutcome(detail = null) {
  const payload = detail && typeof detail === 'object' ? detail : {};
  const approvalId = String(payload.approvalId || '').trim();
  if (!approvalId) return null;
  return {
    approvalId,
    action: String(payload.action || '').trim(),
    status: String(payload.status || '').trim(),
    decision: String(payload.decision || '').trim(),
    conversationId: String(payload.conversationId || '').trim(),
    turnId: String(payload.turnId || '').trim(),
    messageId: String(payload.messageId || '').trim(),
    toolName: String(payload.toolName || '').trim(),
    result: String(payload.result || '').trim(),
    errorMessage: String(payload.errorMessage || '').trim(),
  };
}

export function dispatchMCPUIApprovalOutcome(detail = null) {
  const normalized = normalizeMCPUIApprovalOutcome(detail);
  if (!normalized || typeof window === 'undefined' || typeof window.dispatchEvent !== 'function') {
    return false;
  }
  window.dispatchEvent(new CustomEvent(MCPUI_APPROVAL_OUTCOME_EVENT, { detail: normalized }));
  return true;
}

export function subscribeMCPUIApprovalOutcomes(handler) {
  if (typeof window === 'undefined' || typeof window.addEventListener !== 'function' || typeof handler !== 'function') {
    return () => {};
  }
  const listener = (event) => {
    const normalized = normalizeMCPUIApprovalOutcome(event?.detail);
    if (!normalized) return;
    handler(normalized, event);
  };
  window.addEventListener(MCPUI_APPROVAL_OUTCOME_EVENT, listener);
  return () => window.removeEventListener(MCPUI_APPROVAL_OUTCOME_EVENT, listener);
}
