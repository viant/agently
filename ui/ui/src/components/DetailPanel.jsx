import React, { useEffect, useMemo, useState } from 'react';
import { Button, Dialog } from '@blueprintjs/core';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
import { openLinkedConversationWindow } from '../services/conversationWindow';
import { displayStepIcon, displayStepTitle, isAgentRunTool } from '../services/toolPresentation';
import { resolvePayload } from '../services/chatRuntime';
import { client } from '../services/agentlyClient';

const payloadCache = new Map();

function parseJSONIfPossible(value) {
  if (typeof value !== 'string') return value;
  const text = value.trim();
  if (!text) return value;
  if (!(text.startsWith('{') || text.startsWith('['))) return value;
  try {
    return JSON.parse(text);
  } catch (_) {
    return value;
  }
}

function extractCompressedPayloadId(payload = null) {
  const source = parseJSONIfPossible(payload);
  if (!source || typeof source !== 'object' || Array.isArray(source)) return '';
  const id = String(
    source.id
    || source.Id
    || source.payloadId
    || source.payloadID
    || source.ID
    || ''
  ).trim();
  if (!id) return '';
  const compression = String(source.compression || source.Compression || '').toLowerCase();
  const note = String(source.note || source.Note || '').toLowerCase();
  const hasInlineBody = typeof source.inlineBody === 'string' || typeof source.InlineBody === 'string';
  if (compression && compression !== 'none') return id;
  if (hasInlineBody && compression) return id;
  if (note.includes('compressed')) return id;
  return '';
}

async function fetchPayloadById(payloadId = '') {
  const id = String(payloadId || '').trim();
  if (!id) return null;
  if (payloadCache.has(id)) return payloadCache.get(id);
  try {
    const res = await client.getPayload(id, { raw: true });
    const contentType = String(res?.contentType || '').toLowerCase();
    const decoder = new TextDecoder();
    const text = decoder.decode(res.data);
    if (!text || !text.trim()) {
      const empty = { payloadId: id, note: 'payload is empty' };
      payloadCache.set(id, empty);
      return empty;
    }
    let normalized = text;
    if (contentType.includes('application/json') || (text.trim().startsWith('{') || text.trim().startsWith('['))) {
      try {
        normalized = JSON.parse(text);
      } catch (_) {
        normalized = text;
      }
    }
    payloadCache.set(id, normalized);
    return normalized;
  } catch (err) {
    throw err;
  }
}

function detailKind(toolCall = {}) {
  const raw = String(toolCall?.reason || '').toLowerCase();
  if (raw.includes('think')) return 'thinking';
  if (raw.includes('link')) return 'link';
  if (raw.includes('tool')) return 'tool_call';
  return toolCall?.kind === 'model' ? 'thinking' : 'tool_call';
}

function findLinkedConversationId(toolCall = {}) {
  const explicit = String(toolCall?.linkedConversationId || '').trim();
  if (explicit) return explicit;
  const scan = (node) => {
    if (!node) return '';
    if (Array.isArray(node)) {
      for (const item of node) {
        const found = scan(item);
        if (found) return found;
      }
      return '';
    }
    if (typeof node === 'object') {
      const direct = String(
        node.linkedConversationId
        || node.LinkedConversationId
        || node.conversationId
        || node.ConversationId
        || ''
      ).trim();
      if (direct && /^[0-9a-f-]{24,}$/i.test(direct)) return direct;
      for (const key of Object.keys(node)) {
        const found = scan(node[key]);
        if (found) return found;
      }
    }
    return '';
  };
  return scan(toolCall?.responsePayload) || '';
}

function openLinkedConversation(id = '', onClose) {
  const conversationID = String(id || '').trim();
  if (!conversationID) return;
  openLinkedConversationWindow(conversationID);
  if (typeof onClose === 'function') onClose();
}

function stringifyPayload(payload) {
  if (payload == null) return '';
  if (typeof payload === 'string') return payload;
  try {
    return JSON.stringify(payload, null, 2);
  } catch (_) {
    return String(payload);
  }
}

function toNumber(value) {
  const n = Number(value);
  return Number.isFinite(n) && n >= 0 ? n : null;
}

function parseMaybeJSON(value) {
  if (value == null) return null;
  if (typeof value === 'object') return value;
  if (typeof value !== 'string') return null;
  const text = value.trim();
  if (!text || !(text.startsWith('{') || text.startsWith('['))) return null;
  try {
    return JSON.parse(text);
  } catch (_) {
    return null;
  }
}

function readTokenUsage(toolCall = {}) {
  const candidates = [
    toolCall?.usage,
    toolCall?.tokenUsage,
    toolCall?.responsePayload?.usage,
    toolCall?.providerResponsePayload?.usage,
    parseMaybeJSON(toolCall?.responsePayload)?.usage,
    parseMaybeJSON(toolCall?.providerResponsePayload)?.usage
  ].filter(Boolean);

  for (const usage of candidates) {
    const prompt = toNumber(
      usage?.prompt_tokens
      ?? usage?.promptTokens
      ?? usage?.input_tokens
      ?? usage?.inputTokens
      ?? usage?.PromptTokens
      ?? usage?.InputTokens
    );
    const completion = toNumber(
      usage?.completion_tokens
      ?? usage?.completionTokens
      ?? usage?.output_tokens
      ?? usage?.outputTokens
      ?? usage?.CompletionTokens
      ?? usage?.OutputTokens
    );
    const total = toNumber(
      usage?.total_tokens
      ?? usage?.totalTokens
      ?? usage?.TotalTokens
      ?? ((prompt || 0) + (completion || 0))
    );
    if (prompt != null || completion != null || total != null) {
      return { prompt, completion, total };
    }
  }
  return null;
}

function mergeHydratedToolCall(base = {}, incoming = {}) {
  if (!incoming || typeof incoming !== 'object') return base || {};
  return {
    ...base,
    ...incoming,
    kind: incoming?.kind || base?.kind || '',
    reason: incoming?.reason || base?.reason || '',
    toolName: incoming?.toolName || base?.toolName || '',
    provider: incoming?.provider || base?.provider || '',
    model: incoming?.model || base?.model || '',
    status: incoming?.status || base?.status || '',
    linkedConversationId: incoming?.linkedConversationId || incoming?.LinkedConversationId || base?.linkedConversationId || base?.LinkedConversationId || '',
    requestPayloadId: incoming?.requestPayloadId || incoming?.RequestPayloadId || base?.requestPayloadId || '',
    responsePayloadId: incoming?.responsePayloadId || incoming?.ResponsePayloadId || base?.responsePayloadId || '',
    providerRequestPayloadId: incoming?.providerRequestPayloadId || incoming?.ProviderRequestPayloadId || base?.providerRequestPayloadId || '',
    providerResponsePayloadId: incoming?.providerResponsePayloadId || incoming?.ProviderResponsePayloadId || base?.providerResponsePayloadId || '',
    streamPayloadId: incoming?.streamPayloadId || incoming?.StreamPayloadId || base?.streamPayloadId || '',
    requestPayload: incoming?.requestPayload ?? incoming?.RequestPayload ?? base?.requestPayload ?? null,
    responsePayload: incoming?.responsePayload ?? incoming?.ResponsePayload ?? base?.responsePayload ?? null,
    providerRequestPayload: incoming?.providerRequestPayload ?? incoming?.ProviderRequestPayload ?? base?.providerRequestPayload ?? null,
    providerResponsePayload: incoming?.providerResponsePayload ?? incoming?.ProviderResponsePayload ?? base?.providerResponsePayload ?? null,
    streamPayload: incoming?.streamPayload ?? incoming?.StreamPayload ?? base?.streamPayload ?? null,
    latencyMs: incoming?.latencyMs ?? incoming?.LatencyMs ?? base?.latencyMs ?? null
  };
}

function toolCallHasPayloads(toolCall = {}) {
  return Boolean(
    toolCall?.requestPayloadId
    || toolCall?.responsePayloadId
    || toolCall?.providerRequestPayloadId
    || toolCall?.providerResponsePayloadId
    || toolCall?.streamPayloadId
    || toolCall?.requestPayload
    || toolCall?.responsePayload
    || toolCall?.providerRequestPayload
    || toolCall?.providerResponsePayload
    || toolCall?.streamPayload
  );
}

function normalizeToolStepFromTranscript(message = {}) {
  const toolCall = message?.ToolCall || message?.toolCall || {};
  const requestPayload = resolvePayload(
    toolCall?.RequestPayload
    || toolCall?.requestPayload
    || message?.RequestPayload
    || message?.requestPayload
  );
  const responsePayload = resolvePayload(
    toolCall?.ResponsePayload
    || toolCall?.responsePayload
    || message?.ResponsePayload
    || message?.responsePayload
  );
  const providerRequestPayload = resolvePayload(
    toolCall?.ProviderRequestPayload
    || toolCall?.providerRequestPayload
    || message?.ProviderRequestPayload
    || message?.providerRequestPayload
  );
  const providerResponsePayload = resolvePayload(
    toolCall?.ProviderResponsePayload
    || toolCall?.providerResponsePayload
    || message?.ProviderResponsePayload
    || message?.providerResponsePayload
  );
  const streamPayload = resolvePayload(
    toolCall?.StreamPayload
    || toolCall?.streamPayload
    || message?.StreamPayload
    || message?.streamPayload
  );
  return {
    id: message?.Id || message?.id || toolCall?.MessageId || toolCall?.messageId || '',
    kind: 'tool',
    reason: 'tool_call',
    toolName: message?.ToolName || message?.toolName || toolCall?.ToolName || toolCall?.toolName || '',
    status: message?.Status || message?.status || toolCall?.Status || toolCall?.status || '',
    latencyMs: toolCall?.LatencyMs || toolCall?.latencyMs || message?.LatencyMs || message?.latencyMs || null,
    linkedConversationId: message?.LinkedConversationId || message?.linkedConversationId || toolCall?.LinkedConversationId || toolCall?.linkedConversationId || '',
    requestPayloadId: toolCall?.RequestPayloadId || toolCall?.requestPayloadId || message?.RequestPayloadId || message?.requestPayloadId || '',
    responsePayloadId: toolCall?.ResponsePayloadId || toolCall?.responsePayloadId || message?.ResponsePayloadId || message?.responsePayloadId || '',
    providerRequestPayloadId: toolCall?.ProviderRequestPayloadId || toolCall?.providerRequestPayloadId || message?.ProviderRequestPayloadId || message?.providerRequestPayloadId || '',
    providerResponsePayloadId: toolCall?.ProviderResponsePayloadId || toolCall?.providerResponsePayloadId || message?.ProviderResponsePayloadId || message?.providerResponsePayloadId || '',
    streamPayloadId: toolCall?.StreamPayloadId || toolCall?.streamPayloadId || message?.StreamPayloadId || message?.streamPayloadId || '',
    requestPayload,
    responsePayload,
    providerRequestPayload,
    providerResponsePayload,
    streamPayload
  };
}

function normalizeModelStepFromTranscript(message = {}) {
  const modelCall = message?.ModelCall || message?.modelCall || {};
  return {
    id: message?.Id || message?.id || modelCall?.MessageId || modelCall?.messageId || '',
    kind: 'model',
    reason: Number(message?.Interim || message?.interim || 0) === 1 ? 'thinking' : 'final_response',
    toolName: `${String(modelCall?.Provider || modelCall?.provider || '').trim() ? `${String(modelCall?.Provider || modelCall?.provider || '').trim()}/` : ''}${String(modelCall?.Model || modelCall?.model || '').trim()}` || 'model',
    provider: modelCall?.Provider || modelCall?.provider || '',
    model: modelCall?.Model || modelCall?.model || '',
    status: modelCall?.Status || modelCall?.status || message?.Status || message?.status || '',
    latencyMs: modelCall?.LatencyMs || modelCall?.latencyMs || null,
    requestPayloadId: modelCall?.RequestPayloadId || modelCall?.requestPayloadId || '',
    responsePayloadId: modelCall?.ResponsePayloadId || modelCall?.responsePayloadId || '',
    providerRequestPayloadId: modelCall?.ProviderRequestPayloadId || modelCall?.providerRequestPayloadId || '',
    providerResponsePayloadId: modelCall?.ProviderResponsePayloadId || modelCall?.providerResponsePayloadId || '',
    streamPayloadId: modelCall?.StreamPayloadId || modelCall?.streamPayloadId || '',
    requestPayload: resolvePayload(modelCall?.ModelCallRequestPayload || modelCall?.modelCallRequestPayload || modelCall?.RequestPayload || modelCall?.requestPayload),
    responsePayload: resolvePayload(modelCall?.ModelCallResponsePayload || modelCall?.modelCallResponsePayload || modelCall?.ResponsePayload || modelCall?.responsePayload),
    providerRequestPayload: resolvePayload(modelCall?.ModelCallProviderRequestPayload || modelCall?.modelCallProviderRequestPayload),
    providerResponsePayload: resolvePayload(modelCall?.ModelCallProviderResponsePayload || modelCall?.modelCallProviderResponsePayload),
    streamPayload: resolvePayload(modelCall?.ModelCallStreamPayload || modelCall?.modelCallStreamPayload)
  };
}

function currentConversationId() {
  if (typeof window === 'undefined') return '';
  const match = String(window.location?.pathname || '').match(/\/conversation\/([^/?#]+)/);
  if (match) return decodeURIComponent(match[1]);
  return String(window.localStorage?.getItem('agently.selectedConversationId') || '').trim();
}

async function hydrateToolCallFromTranscript(toolCall = {}) {
  if (toolCallHasPayloads(toolCall)) return toolCall;
  if (typeof window === 'undefined') return toolCall;
  const conversationId = currentConversationId();
  if (!conversationId) return toolCall;
  let turns;
  try {
    const transcript = await client.getTranscript({ conversationId, includeModelCalls: true, includeToolCalls: true });
    turns = Array.isArray(transcript?.turns) ? transcript.turns : [];
  } catch (_) {
    return toolCall;
  }
  const targetId = String(toolCall?.id || '').trim();
  const targetName = String(toolCall?.toolName || '').trim().toLowerCase().replace(/[:\-_]/g, '');
  const targetKind = String(
    toolCall?.kind
    || (toolCall?.model || toolCall?.provider ? 'model' : '')
    || (toolCall?.toolName ? 'tool' : '')
    || ''
  ).trim().toLowerCase();
  for (const turn of turns) {
    const messages = Array.isArray(turn?.Message) ? turn.Message : [];
    for (const message of messages) {
      const messageId = String(message?.Id || message?.id || '').trim();
      if (targetKind === 'model' && (message?.ModelCall || message?.modelCall)) {
        const candidate = normalizeModelStepFromTranscript(message);
        const idMatch = targetId && candidate.id === targetId;
        const modelMatch = toolCall?.model && candidate.model && String(candidate.model).toLowerCase() === String(toolCall.model).toLowerCase();
        const providerMatch = toolCall?.provider && candidate.provider && String(candidate.provider).toLowerCase() === String(toolCall.provider).toLowerCase();
        if (idMatch || ((modelMatch || providerMatch) && toolCallHasPayloads(candidate))) {
          return mergeHydratedToolCall(toolCall, candidate);
        }
      }
      // Also check if a tool-kind step is actually this model's foreground agent run
      if (targetKind === 'model' && !targetId) {
        const directName = String(message?.ToolName || message?.toolName || '').trim().toLowerCase().replace(/[:\-_]/g, '');
        if (directName && targetName && directName === targetName) {
          const attached = normalizeToolStepFromTranscript(message);
          if (toolCallHasPayloads(attached)) {
            return mergeHydratedToolCall(toolCall, attached);
          }
        }
      }
      if (targetKind === 'tool' || targetKind === '') {
        const directName = String(message?.ToolName || message?.toolName || '').trim().toLowerCase().replace(/[:\-_]/g, '');
        if ((targetId && messageId === targetId) || (targetName && directName === targetName)) {
          const attached = normalizeToolStepFromTranscript(message);
          // Accept even without payloads if ID matches — tool may still be running.
          if (toolCallHasPayloads(attached) || (targetId && messageId === targetId)) {
            return mergeHydratedToolCall(toolCall, attached);
          }
        }
        const toolMessages = Array.isArray(message?.ToolMessage) ? message.ToolMessage : [];
        for (const item of toolMessages) {
          const candidate = normalizeToolStepFromTranscript(item);
          const candidateId = String(candidate.id || '').trim();
          const candidateName = String(candidate.toolName || '').trim().toLowerCase().replace(/[:\-_]/g, '');
          if ((targetId && candidateId === targetId) || (targetName && candidateName === targetName)) {
            if (toolCallHasPayloads(candidate) || (targetId && candidateId === targetId)) {
              return mergeHydratedToolCall(toolCall, candidate);
            }
          }
        }
      }
    }
  }
  return toolCall;
}

export default function DetailPanel({ toolCall, onClose }) {
  const [payloadDialog, setPayloadDialog] = useState(null); // {title, loading, payload, error}
  const [resolvedToolCall, setResolvedToolCall] = useState(toolCall || null);
  useEffect(() => {
    let cancelled = false;
    setResolvedToolCall(toolCall || null);
    if (!toolCall) return () => {};
    hydrateToolCallFromTranscript(toolCall).then((next) => {
      if (!cancelled) setResolvedToolCall(next || toolCall);
    }).catch(() => {
      if (!cancelled) setResolvedToolCall(toolCall);
    });
    return () => {
      cancelled = true;
    };
  }, [toolCall]);
  const effectiveToolCall = resolvedToolCall || toolCall || null;
  const kind = useMemo(() => detailKind(effectiveToolCall), [effectiveToolCall]);
  const tokenUsage = useMemo(() => readTokenUsage(effectiveToolCall || {}), [effectiveToolCall]);
  const linkedConversationId = useMemo(() => findLinkedConversationId(effectiveToolCall || {}), [effectiveToolCall]);
  const canOpenLinkedConversation = Boolean(linkedConversationId)
    && (isAgentRunTool(effectiveToolCall || {}) || kind === 'link');
  const payloadCapable = kind === 'tool_call' || kind === 'thinking';

  if (!effectiveToolCall) return null;

  const openPayload = async (part) => {
    const hydratedToolCall = await hydrateToolCallFromTranscript(effectiveToolCall || {});
    const map = {
      request: {
        title: 'Request',
        payload: hydratedToolCall?.requestPayload || null,
        payloadId: hydratedToolCall?.requestPayloadId || ''
      },
      response: {
        title: 'Response',
        payload: hydratedToolCall?.responsePayload || null,
        payloadId: hydratedToolCall?.responsePayloadId || ''
      },
      providerRequest: {
        title: 'Provider Request',
        payload: hydratedToolCall?.providerRequestPayload || null,
        payloadId: hydratedToolCall?.providerRequestPayloadId || ''
      },
      providerResponse: {
        title: 'Provider Response',
        payload: hydratedToolCall?.providerResponsePayload || null,
        payloadId: hydratedToolCall?.providerResponsePayloadId || ''
      },
      stream: {
        title: 'Stream',
        payload: hydratedToolCall?.streamPayload || null,
        payloadId: hydratedToolCall?.streamPayloadId || ''
      }
    };
    const selected = map[part];
    if (!selected) return;
    setPayloadDialog({ title: selected.title, loading: true, payload: null, error: '' });
    try {
      let loaded = selected.payload;
      const compressedPayloadId = extractCompressedPayloadId(loaded);
      const payloadId = String(selected.payloadId || compressedPayloadId || '').trim();
      if ((loaded == null || loaded === '' || compressedPayloadId) && payloadId) {
        loaded = await fetchPayloadById(payloadId);
      }
      setPayloadDialog({ title: selected.title, loading: false, payload: loaded, error: '' });
    } catch (err) {
      setPayloadDialog({
        title: selected.title,
        loading: false,
        payload: null,
        error: String(err?.message || err || 'failed to load payload')
      });
    }
  };

  const copyPayload = async () => {
    if (!payloadDialog || payloadDialog.payload == null) return;
    try {
      await navigator.clipboard.writeText(stringifyPayload(payloadDialog.payload));
    } catch (_) {}
  };

  const isModel = String(effectiveToolCall?.kind || '').toLowerCase() === 'model';
  // Always enable payload buttons — openPayload() re-hydrates on demand,
  // so even if payload IDs aren't known yet they can be fetched on click.
  const hasStream = !!(effectiveToolCall?.streamPayloadId || effectiveToolCall?.streamPayload);

  return (
    <aside className="app-detail-panel app-detail-panel-dialog">
      <div className="app-detail-head app-detail-head-compact">
        <div className="app-detail-tool-row">
          <span className={`app-detail-kind kind-${isModel ? 'model' : 'tool'}`}>
            {displayStepIcon(effectiveToolCall || {})}
          </span>
          <span className="app-detail-tool-name">{displayStepTitle(effectiveToolCall || {})}</span>
          <span className="app-detail-status-chip">{String(effectiveToolCall?.status || '').toLowerCase() || 'unknown'}</span>
        </div>
        <Button minimal small icon="cross" onClick={onClose} />
      </div>

      <div className="app-detail-section app-detail-section-summary">
        {isModel && effectiveToolCall?.provider ? <div><strong>Provider:</strong> {effectiveToolCall.provider}</div> : null}
        {isModel && effectiveToolCall?.model ? <div><strong>Model:</strong> {effectiveToolCall.model}</div> : null}
        {effectiveToolCall?.latencyMs ? <div><strong>Duration:</strong> {Math.round(effectiveToolCall.latencyMs)}ms</div> : null}
        {isModel && tokenUsage ? (
          <div className="app-detail-token-usage">
            <span className="app-detail-token-chip">Prompt {tokenUsage.prompt ?? 'n/a'}</span>
            <span className="app-detail-token-chip">Completion {tokenUsage.completion ?? 'n/a'}</span>
            <span className="app-detail-token-chip app-detail-token-chip-total">Total {tokenUsage.total ?? 'n/a'}</span>
          </div>
        ) : null}
        {linkedConversationId ? (
          <div className="app-detail-thread-link">
            <strong>Linked conversation:</strong>
            <Button small className="app-detail-pill app-detail-thread-open" onClick={() => openLinkedConversation(linkedConversationId, onClose)}>Open Thread →</Button>
          </div>
        ) : null}
      </div>

      {isModel ? (
        <>
          <div className="app-detail-section-label">General</div>
          <div className="app-detail-action-bar">
            <Button small className="app-detail-pill" onClick={() => openPayload('request')}>Request</Button>
            <Button small className="app-detail-pill" onClick={() => openPayload('response')}>Response</Button>
            {hasStream ? <Button small className="app-detail-pill" onClick={() => openPayload('stream')}>Stream</Button> : null}
          </div>
          <div className="app-detail-section-label">Provider</div>
          <div className="app-detail-action-bar">
            <Button small className="app-detail-pill" onClick={() => openPayload('providerRequest')}>Provider Request</Button>
            <Button small className="app-detail-pill" onClick={() => openPayload('providerResponse')}>Provider Response</Button>
          </div>
        </>
      ) : (
        <div className="app-detail-action-bar">
          <Button small className="app-detail-pill" onClick={() => openPayload('request')}>Request</Button>
          <Button small className="app-detail-pill" onClick={() => openPayload('response')}>Response</Button>
        </div>
      )}

      <Dialog
        isOpen={!!payloadDialog}
        onClose={() => setPayloadDialog(null)}
        title={payloadDialog?.title || 'Payload'}
        style={{ width: '90vw', maxWidth: '1400px', minWidth: '900px' }}
      >
        <div className="app-payload-dialog-body">
          <div className="app-payload-dialog-actions">
            <Button small onClick={copyPayload} disabled={payloadDialog?.payload == null}>Copy</Button>
          </div>
          {payloadDialog?.loading ? <div>Loading…</div> : null}
          {payloadDialog?.error ? <div className="app-detail-error">{payloadDialog.error}</div> : null}
          {!payloadDialog?.loading && !payloadDialog?.error && payloadDialog?.payload != null ? (
            <CodeMirror
              value={stringifyPayload(payloadDialog?.payload || '')}
              extensions={[json()]}
              editable={false}
              basicSetup={{ lineNumbers: true, foldGutter: true }}
              height="72vh"
              theme="dark"
            />
          ) : null}
          {!payloadDialog?.loading && !payloadDialog?.error && payloadDialog?.payload == null ? (
            <div className="app-detail-empty">Payload is not available for this step yet.</div>
          ) : null}
        </div>
      </Dialog>
    </aside>
  );
}
