import { sdkBaseURL } from '../../endpoint';
import { buildEnvelope, MCPUI_METHODS, validateEnvelope } from './appproto.js';

const MCP_UI_TOOL_CALL_PATH = `${sdkBaseURL}/api/mcp-ui/tools/call`;

export function isAllowedTool(allowedTools = [], toolName = '') {
  const wanted = String(toolName || '').trim();
  return wanted ? (Array.isArray(allowedTools) ? allowedTools : []).includes(wanted) : false;
}

// classifyOpenLinkURL is the pure, host-side policy decision for mcpui:open-link
// URLs in MVP: only absolute https: URLs are accepted. Everything else (empty,
// relative, malformed, javascript:, data:, and all other non-https: schemes)
// is rejected with a structured reason. This is the single source of truth for
// the policy documented in mcp-ui.md "ui/open-link".
export function classifyOpenLinkURL(url = '') {
  const raw = String(url || '').trim();
  if (!raw) {
    return { ok: false, reason: 'open-link url is required' };
  }
  let parsed;
  try {
    parsed = new URL(raw);
  } catch (_) {
    return { ok: false, reason: `open-link url is invalid: ${raw}` };
  }
  if (parsed.protocol !== 'https:') {
    return { ok: false, reason: `open-link url is not allowed: ${parsed.protocol || raw}` };
  }
  return { ok: true, url: parsed.toString() };
}

export function normalizeSafeOpenLink(url = '') {
  const verdict = classifyOpenLinkURL(url);
  if (!verdict.ok) {
    throw new Error(verdict.reason);
  }
  return verdict.url;
}

export async function executeGuestToolCall(params = {}, fetchImpl = globalThis.fetch) {
  if (typeof fetchImpl !== 'function') {
    throw new Error('fetch implementation is required');
  }
  const name = String(params?.name || '').trim();
  if (!name) {
    throw new Error('tool name is required');
  }
  const response = await fetchImpl(MCP_UI_TOOL_CALL_PATH, {
    method: 'POST',
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      conversationId: String(params?.conversationId || '').trim(),
      toolName: name,
      arguments: params?.arguments || {},
      assistantText: String(params?.assistantText || '').trim(),
      toolBundles: Array.isArray(params?.toolBundles) ? params.toolBundles : [],
    }),
  });
  if (!response.ok) {
    const text = await response.text().catch(() => '');
    throw new Error(text || `tool execute failed (${response.status})`);
  }
  return response.json();
}

export async function handleGuestEnvelope(envelope, context = {}) {
  const parsed = validateEnvelope(envelope);
  if (!parsed.ok) {
    throw new Error(parsed.error);
  }
  const { params, method } = parsed.envelope;
  if (method === MCPUI_METHODS.MESSAGE) {
    return { type: 'message', payload: params };
  }
  if (method === MCPUI_METHODS.OPEN_LINK) {
    return { type: 'open-link', payload: { ...params, url: normalizeSafeOpenLink(params?.url) } };
  }
  if (method === MCPUI_METHODS.TOOLS_CALL) {
    const allowed = Array.isArray(context.allowedTools) ? context.allowedTools : [];
    if (!isAllowedTool(allowed, params?.name)) {
      throw new Error(`tool not allowed: ${params?.name || ''}`);
    }
    const toolResult = await executeGuestToolCall({
      ...params,
      conversationId: context.conversationId,
      toolBundles: context.allowedToolBundles,
    }, context.fetchImpl);
    return {
      type: 'tool-result',
      payload: buildEnvelope(MCPUI_METHODS.TOOL_RESULT, {
        windowId: params?.windowId || '',
        resourceUri: params?.resourceUri || '',
        toolName: params?.name || '',
        content: [{ type: 'text', text: String(toolResult?.result || '') }],
        structuredContent: toolResult || {},
        _meta: {},
        protocolVersion: context.protocolVersion || parsed.envelope.version,
      }),
    };
  }
  throw new Error(`unsupported guest method: ${method}`);
}
