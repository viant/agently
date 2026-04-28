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
  return normalized === 'llm/agents-run'
    || normalized === 'llm/agents/run'
    || normalized === 'llm/agents-start'
    || normalized === 'llm/agents/start';
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

function normalizeModelToken(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  return text
    .replace(/^openai_/i, '')
    .replace(/^vertexai_/i, '')
    .replace(/^bedrock_/i, '')
    .replace(/^xai_/i, '')
    .replace(/_/g, '.')
    .replace(/\.mini$/i, ' mini');
}

function resolveModelIdentity(step = {}) {
  const directProvider = String(step?.provider || '').trim();
  const directModel = String(step?.model || '').trim();
  if (directProvider || directModel) {
    return {
      provider: directProvider,
      model: normalizeModelToken(directModel),
    };
  }
  const payloads = [
    extractPayloadObject(step?.requestPayload || null),
    extractPayloadObject(step?.providerRequestPayload || null),
  ];
  for (const payload of payloads) {
    if (!payload || typeof payload !== 'object') continue;
    const provider = String(
      payload?.provider
      || payload?.input?.provider
      || ''
    ).trim();
    const model = String(
      payload?.model
      || payload?.input?.model
      || ''
    ).trim();
    if (provider || model) {
      return {
        provider,
        model: normalizeModelToken(model),
      };
    }
  }
  const combined = String(step?.toolName || step?.name || '').trim();
  if (combined.includes('/')) {
    const [provider, model] = combined.split('/', 2);
    return {
      provider: String(provider || '').trim(),
      model: normalizeModelToken(model),
    };
  }
  return {
    provider: '',
    model: normalizeModelToken(combined),
  };
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

export function executionRoleLabel(step = {}) {
  const explicitRole = String(step?.executionRole || '').trim();
  if (!explicitRole) return '';
  switch (explicitRole.toLowerCase()) {
    case 'react': return '⌬';
    case 'bootstrap': return '⇢';
    case 'intake': return '⇢';
    case 'narrator': return '✍';
    case 'router': return '🧭';
    case 'summary': return '≡';
    case 'worker': return '⚙';
    default: return '';
  }
}

export function displayStepTitle(step = {}) {
  const kind = String(step?.kind || '').toLowerCase();
  if (kind === 'model') {
    const role = String(step?.executionRole || '').trim().toLowerCase();
    if (role === 'bootstrap') return 'Bootstrap';
    if (role === 'narrator') return 'Narrator';
    if (role === 'intake') return 'Intake';
    if (role === 'summary') return 'Summary';
    if (role === 'router') return 'Router';
    const { provider, model } = resolveModelIdentity(step);
    if (provider && model) return `${provider}/${model}`;
    if (model) return model;
    if (provider) return provider;
    return 'assistant model';
  }
  if (kind === 'turn') {
    const reason = String(step?.reason || step?.toolName || '').trim().toLowerCase();
    switch (reason) {
      case 'turn_started':
        return 'Turn started';
      case 'turn_completed':
        return 'Turn completed';
      case 'turn_failed':
        return 'Turn failed';
      case 'turn_canceled':
      case 'turn_cancelled':
        return 'Turn canceled';
      default:
        return 'Turn event';
    }
  }
  const delegated = delegatedAgentLabel(step);
  if (delegated) return delegated;
  return String(step?.toolName || step?.ToolName || 'tool');
}

export function displayStepIcon(step = {}) {
  const kind = String(step?.kind || '').toLowerCase();
  if (kind === 'model') return '🧠';
  if (kind === 'turn') return '⏺';
  if (isAgentRunTool(step)) return '🧭';
  return '🛠';
}
