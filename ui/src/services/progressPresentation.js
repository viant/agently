// Presentation only: execution state and developer diagnostics retain raw errors.
export function endUserProgressMessage(message = '') {
  const text = String(message || '');
  if (/could not stop|cannot cancel|could not cancel/i.test(text)) return 'Couldn’t confirm that the request stopped. You can try again.';
  if (/unauthorized|authentication|sign.?in|authorization required/i.test(text)) return 'A connection needs your attention before this can continue.';
  if (/timeout|timed out|network|unavailable|connection/i.test(text)) return 'The connection was interrupted. Please try again.';
  return 'This step couldn’t be completed. You can try again.';
}

export function progressStatusPresentation(stage = {}, developerMode = false, backendUnavailable = false) {
  const phase = backendUnavailable ? 'offline' : String(stage.phase || 'ready');
  const text = backendUnavailable ? 'Service temporarily unavailable. Reconnecting…' : String(stage.text || 'Ready');
  if (developerMode || !['error', 'failed', 'offline'].includes(phase)) return {phase, text};
  return {phase: 'attention', text: backendUnavailable ? text : endUserProgressMessage(text)};
}
