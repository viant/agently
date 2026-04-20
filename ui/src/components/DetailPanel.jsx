import React, { useEffect, useMemo, useState } from 'react';
import { Button, Dialog } from '@blueprintjs/core';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
import { openLinkedConversationWindow } from '../services/conversationWindow';
import { displayStepIcon, displayStepTitle, isAgentRunTool } from '../services/toolPresentation';
import { resolvePayload } from '../services/payloads';
import { flattenCanonicalTranscriptSteps, transcriptConversationTurns } from '../services/canonicalTranscript';
import { client } from '../services/agentlyClient';

const payloadCache = new Map();
const MODEL_PRICING_USD_PER_MILLION = {
  'openai:gpt-5.4': { input: 2.5, output: 15.0 },
  'openai:gpt-5.4-mini': { input: 0.75, output: 4.5 },
  'openai:gpt-5.4-nano': { input: 0.2, output: 1.25 }
};

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

function normalizePricingKey(provider = '', model = '') {
  const normalizedProvider = String(provider || '').trim().toLowerCase().replace(/[_\s]+/g, '-');
  const normalizedModel = String(model || '')
    .trim()
    .toLowerCase()
    .replace(/[_\s]+/g, '-')
    .replace(/--+/g, '-');
  if (!normalizedProvider || !normalizedModel) return '';
  return `${normalizedProvider}:${normalizedModel}`;
}

export function estimateTokenUsageCost(toolCall = {}) {
  const usage = readTokenUsage(toolCall);
  if (!usage) return null;
  const pricing = MODEL_PRICING_USD_PER_MILLION[normalizePricingKey(toolCall?.provider, toolCall?.model)];
  if (!pricing) return null;
  const promptTokens = Number(usage?.prompt || 0) || 0;
  const completionTokens = Number(usage?.completion || 0) || 0;
  const promptCost = (promptTokens / 1_000_000) * pricing.input;
  const completionCost = (completionTokens / 1_000_000) * pricing.output;
  const total = promptCost + completionCost;
  return {
    promptCost,
    completionCost,
    total,
    currency: 'USD'
  };
}

export function formatUsdEstimate(value) {
  const amount = Number(value);
  if (!Number.isFinite(amount) || amount < 0) return '';
  if (amount === 0) return '$0.00';
  if (amount < 0.01) return `$${amount.toFixed(4)}`;
  return `$${amount.toFixed(2)}`;
}

function mergeHydratedToolCall(base = {}, incoming = {}) {
  if (!incoming || typeof incoming !== 'object') return base || {};
  return {
    ...base,
    ...incoming,
    kind: incoming?.kind || base?.kind || '',
    phase: incoming?.phase || base?.phase || '',
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

function phaseLabel(step = {}) {
  const explicitRole = String(step?.executionRole || '').trim().toLowerCase();
  switch (explicitRole) {
    case 'react': return '⌬';
    case 'intake': return '⇢';
    case 'narrator': return '✍';
    case 'router': return '🧭';
    case 'summary': return '≡';
    case 'worker': return '⚙';
    default: return '';
  }
}

function toolCallHasResolvedPayloadContent(toolCall = {}) {
  return Boolean(
    toolCall?.requestPayload
    || toolCall?.responsePayload
    || toolCall?.providerRequestPayload
    || toolCall?.providerResponsePayload
    || toolCall?.streamPayload
  );
}

function currentConversationId() {
  if (typeof window === 'undefined') return '';
  const match = String(window.location?.pathname || '').match(/\/conversation\/([^/?#]+)/);
  if (match) return decodeURIComponent(match[1]);
  return String(window.localStorage?.getItem('agently.selectedConversationId') || '').trim();
}

export async function hydrateToolCallFromTranscript(toolCall = {}) {
  if (typeof window === 'undefined') return toolCall;
  const conversationId = currentConversationId();
  if (!conversationId) return toolCall;
  let turns;
  try {
    const transcript = await client.getTranscript({ conversationId, includeModelCalls: true, includeToolCalls: true });
    turns = transcriptConversationTurns(transcript);
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
  for (const candidate of flattenCanonicalTranscriptSteps(turns)) {
    const candidateId = String(candidate.id || '').trim();
    const candidateName = String(candidate.toolName || '').trim().toLowerCase().replace(/[:\-_]/g, '');
    if (targetKind === 'model' && String(candidate.kind || '').toLowerCase() === 'model') {
      const idMatch = targetId && candidateId === targetId;
      const modelMatch = toolCall?.model && candidate.model && String(candidate.model).toLowerCase() === String(toolCall.model).toLowerCase();
      const providerMatch = toolCall?.provider && candidate.provider && String(candidate.provider).toLowerCase() === String(toolCall.provider).toLowerCase();
      if (idMatch || ((modelMatch || providerMatch) && toolCallHasResolvedPayloadContent(candidate))) {
        return mergeHydratedToolCall(toolCall, candidate);
      }
    }
    if ((targetKind === 'tool' || targetKind === '') && String(candidate.kind || '').toLowerCase() === 'tool') {
      if ((targetId && candidateId === targetId) || (targetName && candidateName === targetName)) {
        if (toolCallHasResolvedPayloadContent(candidate) || (targetId && candidateId === targetId)) {
          return mergeHydratedToolCall(toolCall, candidate);
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
  const tokenCost = useMemo(() => estimateTokenUsageCost(effectiveToolCall || {}), [effectiveToolCall]);
  const linkedConversationId = useMemo(() => findLinkedConversationId(effectiveToolCall || {}), [effectiveToolCall]);
  const errorText = String(
    effectiveToolCall?.errorMessage
    || effectiveToolCall?.ErrorMessage
    || ''
  ).trim();
  const canOpenLinkedConversation = Boolean(linkedConversationId)
    && (isAgentRunTool(effectiveToolCall || {}) || kind === 'link');
  const payloadCapable = kind === 'tool_call' || kind === 'thinking';
  const hasRequestPayload = !!String(
    effectiveToolCall?.requestPayloadId
    || effectiveToolCall?.requestPayload
    || effectiveToolCall?.providerRequestPayloadId
    || effectiveToolCall?.providerRequestPayload
    || ''
  ).trim();
  const hasResponsePayload = !!String(
    effectiveToolCall?.responsePayloadId
    || effectiveToolCall?.responsePayload
    || effectiveToolCall?.providerResponsePayloadId
    || effectiveToolCall?.providerResponsePayload
    || ''
  ).trim();

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
          {phaseLabel(effectiveToolCall) ? (
            <span className="app-detail-status-chip">{phaseLabel(effectiveToolCall)}</span>
          ) : null}
          <span className="app-detail-status-chip">{String(effectiveToolCall?.status || '').toLowerCase() || 'unknown'}</span>
        </div>
        <Button minimal small icon="cross" onClick={onClose} />
      </div>

      <div className="app-detail-section app-detail-section-summary">
        {isModel && effectiveToolCall?.provider ? <div><strong>Provider:</strong> {effectiveToolCall.provider}</div> : null}
        {isModel && effectiveToolCall?.model ? <div><strong>Model:</strong> {effectiveToolCall.model}</div> : null}
        {effectiveToolCall?.latencyMs ? <div><strong>Duration:</strong> {Math.round(effectiveToolCall.latencyMs)}ms</div> : null}
        {errorText ? <div className="app-detail-error"><strong>Error:</strong> {errorText}</div> : null}
        {isModel && tokenUsage ? (
          <div className="app-detail-token-usage">
            <span className="app-detail-token-chip">Prompt {tokenUsage.prompt ?? 'n/a'}</span>
            <span className="app-detail-token-chip">Completion {tokenUsage.completion ?? 'n/a'}</span>
            <span className="app-detail-token-chip app-detail-token-chip-total">Total {tokenUsage.total ?? 'n/a'}</span>
            {tokenCost ? (
              <span className="app-detail-token-chip app-detail-token-chip-total">{`Est. ${formatUsdEstimate(tokenCost.total)}`}</span>
            ) : null}
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
            {hasRequestPayload ? <Button small className="app-detail-pill" onClick={() => openPayload('request')}>Request</Button> : null}
            {hasResponsePayload ? <Button small className="app-detail-pill" onClick={() => openPayload('response')}>Response</Button> : null}
            {hasStream ? <Button small className="app-detail-pill" onClick={() => openPayload('stream')}>Stream</Button> : null}
          </div>
          <div className="app-detail-section-label">Provider</div>
          <div className="app-detail-action-bar">
            {String(effectiveToolCall?.providerRequestPayloadId || effectiveToolCall?.providerRequestPayload || '').trim()
              ? <Button small className="app-detail-pill" onClick={() => openPayload('providerRequest')}>Provider Request</Button>
              : null}
            {String(effectiveToolCall?.providerResponsePayloadId || effectiveToolCall?.providerResponsePayload || '').trim()
              ? <Button small className="app-detail-pill" onClick={() => openPayload('providerResponse')}>Provider Response</Button>
              : null}
          </div>
        </>
      ) : (
        <div className="app-detail-action-bar">
          {hasRequestPayload ? <Button small className="app-detail-pill" onClick={() => openPayload('request')}>Request</Button> : null}
          {hasResponsePayload ? <Button small className="app-detail-pill" onClick={() => openPayload('response')}>Response</Button> : null}
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
