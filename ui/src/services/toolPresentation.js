function parseMaybeJSON(value) {
  if (!value) return null;
  if (typeof value === 'object') return value;
  if (typeof value !== 'string') return null;
  const text = value.trim();
  if (!text || (!(text.startsWith('{')) && !(text.startsWith('[')))) return null;
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

export function humanizeAgentId(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  return text
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (ch) => ch.toUpperCase());
}

export function normalizeToolNameForDisplay(value = '') {
  return String(value || '').trim().toLowerCase().replace(/[:_]/g, '/');
}

export function isAgentRunTool(stepOrName = {}) {
  const toolName = typeof stepOrName === 'string'
    ? stepOrName
    : (stepOrName?.toolName || stepOrName?.ToolName || '');
  const normalized = normalizeToolNameForDisplay(toolName);
  return normalized === 'llm/agents-run' || normalized === 'llm/agents/run';
}

export function extractPayloadObject(payload = null) {
  const direct = parseMaybeJSON(payload);
  if (direct && typeof direct === 'object' && !Array.isArray(direct)) {
    if (direct.agentId || direct.agentID || direct.AgentID) return direct;
    if (typeof direct.inlineBody === 'string') {
      const parsed = parseMaybeJSON(direct.inlineBody);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed;
    }
    if (typeof direct.InlineBody === 'string') {
      const parsed = parseMaybeJSON(direct.InlineBody);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return parsed;
    }
  }
  return direct;
}

export function delegatedAgentId(step = {}) {
  if (!isAgentRunTool(step)) return '';
  const payload = extractPayloadObject(step?.requestPayload || step?.RequestPayload || null);
  if (!payload || typeof payload !== 'object') return '';
  return String(
    payload?.agentId
    || payload?.agentID
    || payload?.AgentID
    || payload?.input?.agentId
    || payload?.input?.agentID
    || payload?.input?.AgentID
    || ''
  ).trim();
}

export function delegatedAgentLabel(step = {}) {
  return delegatedAgentId(step);
}

export function displayStepTitle(step = {}) {
  const kind = String(step?.kind || '').toLowerCase();
  if (kind === 'model') {
    const provider = String(step?.provider || '').trim();
    const model = String(step?.model || '').trim();
    return model ? `${provider ? `${provider}/` : ''}${model}` : 'model';
  }
  const delegated = delegatedAgentLabel(step);
  if (delegated) return delegated;
  return String(step?.toolName || step?.ToolName || 'tool');
}

export function displayStepIcon(step = {}) {
  const kind = String(step?.kind || '').toLowerCase();
  if (kind === 'model') return '🧠';
  if (isAgentRunTool(step)) return '🧭';
  return '🛠';
}
