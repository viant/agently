export const MCPUI_VERSION = '1.0.0';

export const MCPUI_METHODS = {
  TOOLS_CALL: 'mcpui:tools-call',
  MESSAGE: 'mcpui:message',
  OPEN_LINK: 'mcpui:open-link',
  HOST_READY: 'mcpui:host-ready',
  TOOL_INPUT: 'mcpui:tool-input',
  TOOL_INPUT_PARTIAL: 'mcpui:tool-input-partial',
  TOOL_RESULT: 'mcpui:tool-result',
  TEARDOWN: 'mcpui:teardown',
  SIZE_CHANGED: 'mcpui:size-changed',
};

export function buildEnvelope(method, params = {}) {
  return { version: MCPUI_VERSION, method, params };
}

export function isEnvelopeCandidate(value) {
  return !!value && typeof value === 'object' && typeof value.method === 'string';
}

export function validateEnvelope(value = {}) {
  if (!isEnvelopeCandidate(value)) {
    return { ok: false, error: 'invalid envelope' };
  }
  const version = String(value.version || '').trim();
  if (version !== MCPUI_VERSION) {
    return { ok: false, error: `unsupported envelope version: ${version || 'missing'}` };
  }
  const method = String(value.method || '').trim();
  if (!Object.values(MCPUI_METHODS).includes(method)) {
    return { ok: false, error: `unsupported method: ${method}` };
  }
  return { ok: true, envelope: { version, method, params: value.params || {} } };
}
