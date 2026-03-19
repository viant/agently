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
