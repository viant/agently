/**
 * Singleton ElicitationTracker from the SDK.
 * chatRuntime stores pending elicitations here via SSE events.
 * ElicitationOverlay subscribes and renders the modal Dialog.
 */
import { ElicitationTracker } from 'agently-core-ui-sdk';

export const elicitationTracker = new ElicitationTracker();

// Convenience wrappers for backward compat
export function setPendingElicitation(elicitation) {
  elicitationTracker.setPending(elicitation);
}

export function getPendingElicitation() {
  return elicitationTracker.pending;
}

export function clearPendingElicitation() {
  elicitationTracker.clear();
}

export function onElicitationChange(fn) {
  return elicitationTracker.onChange(fn);
}

export function normalizeElicitationDialogState(source = {}, fallbackConversationId = '') {
  const direct = source?.elicitation && typeof source.elicitation === 'object'
    ? { ...source.elicitation, ...source }
    : source;
  const elicitationId = String(direct?.elicitationId || direct?.ElicitationId || '').trim();
  const requestedSchema = direct?.requestedSchema || direct?.schema || null;
  if (!elicitationId || !requestedSchema || typeof requestedSchema !== 'object') {
    return null;
  }
  const callbackURL = String(direct?.callbackURL || direct?.callbackUrl || '').trim();
  const derivedConversationId = (() => {
    const explicit = String(direct?.conversationId || direct?.ConversationId || fallbackConversationId || '').trim();
    if (explicit) return explicit;
    const match = callbackURL.match(/\/v1\/(?:api\/)?conversations\/([^/]+)\/elicitation\//i);
    return match ? String(match[1] || '').trim() : '';
  })();
  return {
    elicitationId,
    conversationId: derivedConversationId,
    turnId: String(direct?.turnId || direct?.TurnId || '').trim(),
    message: String(direct?.message || direct?.content || '').trim(),
    requestedSchema,
    callbackURL,
    url: String(direct?.url || direct?.Url || '').trim(),
    mode: String(direct?.mode || direct?.Mode || '').trim(),
    status: String(direct?.status || '').trim()
  };
}

export function openElicitationDialog(source = {}, fallbackConversationId = '') {
  const normalized = normalizeElicitationDialogState(source, fallbackConversationId);
  if (!normalized) return false;
  setPendingElicitation(normalized);
  return true;
}
