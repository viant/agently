import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import { Button } from '@blueprintjs/core';
import {Button as SharedButton, Icon, Typography, typographyProps, WorkspaceRuntimeTabs} from 'forge/components';
import {ReviewAction} from './RequestReviewDialog.jsx';
import {useWorkspaceStyle} from './WorkspaceStyleProvider.jsx';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
import { getScopedConversationSelection, MAIN_CHAT_WINDOW_ID, openLinkedConversationWindow } from '../services/conversationWindow';
import { displayStepIcon, displayStepTitle, isAgentRunTool } from '../services/toolPresentation';
import { resolvePayload } from '../services/payloads';
import { client } from '../services/agentlyClient';
import { openElicitationDialog } from '../services/elicitationBus';
import {
  estimateToolTokenUsage,
  formatUsdEstimate,
  resolveModelPricing,
} from '../services/tokenUsageEstimation';
import '../styles/detail-panel.css';
import './executionDetails.css';

export {
  estimatePayloadTokenUsage,
  estimateToolTokenUsage,
  formatUsdEstimate,
} from '../services/tokenUsageEstimation';

const payloadCache = new Map();

function firstText(...values) {
  for (const value of values) {
    const text = String(value || '').trim();
    if (text) return text;
  }
  return '';
}

function transcriptTurns(payload = {}) {
  return Array.isArray(payload?.conversation?.turns) ? payload.conversation.turns : [];
}

function normalizeStepPayload(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return resolvePayload(value);
  const encoded = Object.prototype.hasOwnProperty.call(value, 'InlineBody')
    || Object.prototype.hasOwnProperty.call(value, 'inlineBody')
    || Object.prototype.hasOwnProperty.call(value, 'Compression')
    || Object.prototype.hasOwnProperty.call(value, 'compression');
  return encoded ? resolvePayload(value) : value;
}

function flattenTranscriptSteps(turns = []) {
  const result = [];
  for (const turn of Array.isArray(turns) ? turns : []) {
    const turnId = firstText(turn?.turnId);
    for (const page of Array.isArray(turn?.execution?.pages) ? turn.execution.pages : []) {
      for (const step of Array.isArray(page?.modelSteps) ? page.modelSteps : []) {
        const provider = firstText(step?.provider);
        const model = firstText(step?.model);
        result.push({
          ...step,
          id: firstText(step?.id, step?.modelCallId, step?.assistantMessageId, page?.assistantMessageId, page?.pageId),
          turnId,
          modelCallId: firstText(step?.modelCallId),
          assistantMessageId: firstText(step?.assistantMessageId, page?.assistantMessageId, page?.pageId),
          kind: 'model',
          reason: page?.finalResponse ? 'final_response' : 'thinking',
          executionRole: firstText(step?.executionRole, page?.executionRole),
          phase: firstText(step?.phase, page?.phase),
          toolName: firstText(provider && model ? `${provider}/${model}` : '', model, provider, 'model'),
          provider,
          model,
          status: firstText(step?.status, page?.status),
          errorMessage: firstText(step?.errorMessage, page?.errorMessage),
          requestPayload: normalizeStepPayload(step?.requestPayload),
          responsePayload: normalizeStepPayload(step?.responsePayload),
          providerRequestPayload: normalizeStepPayload(step?.providerRequestPayload),
          providerResponsePayload: normalizeStepPayload(step?.providerResponsePayload),
          streamPayload: normalizeStepPayload(step?.streamPayload),
        });
      }
      for (const step of Array.isArray(page?.toolSteps) ? page.toolSteps : []) {
        const pricingModelStep = Array.isArray(page?.modelSteps)
          ? page.modelSteps[page.modelSteps.length - 1]
          : null;
        result.push({
          ...step,
          id: firstText(step?.id, step?.toolCallId, step?.toolMessageId, page?.assistantMessageId, page?.pageId),
          turnId,
          toolCallId: firstText(step?.toolCallId),
          toolMessageId: firstText(step?.toolMessageId),
          kind: 'tool',
          reason: 'tool_call',
          executionRole: firstText(step?.executionRole, page?.executionRole),
          phase: firstText(step?.phase, page?.phase),
          toolName: firstText(step?.toolName, 'tool'),
          pricingProvider: firstText(step?.pricingProvider, step?.provider, pricingModelStep?.provider),
          pricingModel: firstText(step?.pricingModel, step?.model, pricingModelStep?.model),
          status: firstText(step?.status),
          errorMessage: firstText(step?.errorMessage, page?.errorMessage),
          requestPayload: normalizeStepPayload(step?.requestPayload),
          responsePayload: normalizeStepPayload(step?.responsePayload),
          providerRequestPayload: normalizeStepPayload(step?.providerRequestPayload),
          providerResponsePayload: normalizeStepPayload(step?.providerResponsePayload),
          streamPayload: normalizeStepPayload(step?.streamPayload),
        });
      }
    }
  }
  return result;
}

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

function openElicitationFromDetail(step = {}, onClose) {
  const conversationId = currentConversationId();
  const opened = openElicitationDialog({
    elicitationId: step?.elicitationId,
    requestedSchema: step?.requestedSchema,
    message: step?.message || step?.content,
    callbackURL: step?.callbackURL,
    conversationId,
    turnId: step?.turnId,
    status: step?.status,
    url: step?.url,
    mode: step?.mode
  }, conversationId);
  if (opened && typeof onClose === 'function') onClose();
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

export function estimateTokenUsageCost(toolCall = {}) {
  const usage = readTokenUsage(toolCall);
  if (!usage) return null;
  const pricing = resolveModelPricing(toolCall);
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

function toolCallHasPayloadReference(toolCall = {}) {
  return Boolean(
    toolCallHasResolvedPayloadContent(toolCall)
    || String(toolCall?.requestPayloadId || '').trim()
    || String(toolCall?.responsePayloadId || '').trim()
    || String(toolCall?.providerRequestPayloadId || '').trim()
    || String(toolCall?.providerResponsePayloadId || '').trim()
    || String(toolCall?.streamPayloadId || '').trim()
  );
}

function currentConversationId() {
  if (typeof window === 'undefined') return '';
  const match = String(window.location?.pathname || '').match(/\/conversation\/([^/?#]+)/);
  if (match) return decodeURIComponent(match[1]);
  return getScopedConversationSelection(MAIN_CHAT_WINDOW_ID);
}

export async function hydrateToolCallFromTranscript(toolCall = {}) {
  if (typeof window === 'undefined') return toolCall;
  const conversationId = currentConversationId();
  if (!conversationId) return toolCall;
  let turns;
  try {
    const transcript = await client.getTranscript({ conversationId, includeModelCalls: true, includeToolCalls: true });
    turns = transcriptTurns(transcript);
  } catch (_) {
    return toolCall;
  }
  const targetIds = new Set(
    [
      toolCall?.id,
      toolCall?.modelCallId,
      toolCall?.assistantMessageId,
      toolCall?.toolCallId,
      toolCall?.toolMessageId
    ]
      .map((value) => String(value || '').trim())
      .filter(Boolean)
  );
  const targetName = String(toolCall?.toolName || '').trim().toLowerCase().replace(/[:\-_]/g, '');
  const targetKind = String(
    toolCall?.kind
    || (toolCall?.model || toolCall?.provider ? 'model' : '')
    || (toolCall?.toolName ? 'tool' : '')
    || ''
  ).trim().toLowerCase();
  const candidates = flattenTranscriptSteps(turns);
  if (targetKind === 'model') {
    if (targetIds.size === 0) return toolCall;
    const exact = candidates.find((candidate) => (
      String(candidate.kind || '').toLowerCase() === 'model'
      && [
        candidate.id,
        candidate.modelCallId,
        candidate.assistantMessageId
      ]
        .map((value) => String(value || '').trim())
        .filter(Boolean)
        .some((value) => targetIds.has(value))
    ));
    if (exact) return mergeHydratedToolCall(toolCall, exact);
    return toolCall;
  }
  for (const candidate of candidates) {
    if ((targetKind === 'tool' || targetKind === '') && String(candidate.kind || '').toLowerCase() === 'tool') {
      if (
        [
          candidate.id,
          candidate.toolCallId,
          candidate.toolMessageId
        ]
          .map((value) => String(value || '').trim())
          .filter(Boolean)
          .some((value) => targetIds.has(value))
      ) {
        return mergeHydratedToolCall(toolCall, candidate);
      }
    }
  }
  return toolCall;
}

export default function DetailPanel({ toolCall, onClose, appearance, headingId }) {
  const documentAppearance = appearance === 'document';
  const workspace = useWorkspaceStyle();
  const panelId = `execution-${useId()}`;
  const [activePart, setActivePart] = useState('overview');
  const [payloadView, setPayloadView] = useState(null);
  const [copyStatus, setCopyStatus] = useState('');
  const payloadRequest = useRef(0);
  const [resolvedToolCall, setResolvedToolCall] = useState(toolCall || null);
  useEffect(() => {
    let cancelled = false;
    payloadRequest.current++;
    setActivePart('overview');setPayloadView(null);setCopyStatus('');
    setResolvedToolCall(toolCall || null);
    if (!toolCall) return () => {};
    hydrateToolCallFromTranscript(toolCall).then((next) => {
      if (!cancelled) setResolvedToolCall(next || toolCall);
    }).catch(() => {
      if (!cancelled) setResolvedToolCall(toolCall);
    });
    return () => {
      cancelled = true;
      payloadRequest.current++;
    };
  }, [toolCall]);
  const effectiveToolCall = resolvedToolCall || toolCall || null;
  const kind = useMemo(() => detailKind(effectiveToolCall), [effectiveToolCall]);
  const tokenUsage = useMemo(() => readTokenUsage(effectiveToolCall || {}), [effectiveToolCall]);
  const tokenCost = useMemo(() => estimateTokenUsageCost(effectiveToolCall || {}), [effectiveToolCall]);
  const toolTokenUsage = useMemo(() => estimateToolTokenUsage(effectiveToolCall || {}), [effectiveToolCall]);
  const linkedConversationId = useMemo(() => findLinkedConversationId(effectiveToolCall || {}), [effectiveToolCall]);
  const errorText = String(
    effectiveToolCall?.errorMessage
    || effectiveToolCall?.ErrorMessage
    || ''
  ).trim();
  const canOpenElicitation = Boolean(
    String(effectiveToolCall?.elicitationId || '').trim()
    && effectiveToolCall?.requestedSchema
  );
  const canOpenLinkedConversation = Boolean(linkedConversationId)
    && (isAgentRunTool(effectiveToolCall || {}) || kind === 'link');
  const payloadCapable = kind === 'tool_call' || kind === 'thinking';
  const hasToolPayloadAccounting = kind === 'tool_call' && Boolean(
    toolTokenUsage.toolInput.available
    || toolTokenUsage.toolOutput.available
    || effectiveToolCall?.requestPayloadId
    || effectiveToolCall?.responsePayloadId
    || effectiveToolCall?.requestPayload != null
    || effectiveToolCall?.responsePayload != null
  );
  if (!effectiveToolCall) return null;

  const openPayload = async (part) => {
    const request = ++payloadRequest.current;
    setActivePart(part);setCopyStatus('');
    if (part === 'overview') {setPayloadView(null);return;}
    setPayloadView({loading:true,payload:null,error:''});
    try {
      const hydratedToolCall = await hydrateToolCallFromTranscript(effectiveToolCall || {});
      if (request !== payloadRequest.current) return;
      setResolvedToolCall(hydratedToolCall);
      let loaded = hydratedToolCall?.[`${part}Payload`] ?? null;
      const compressedPayloadId = extractCompressedPayloadId(loaded);
      const payloadId = String(hydratedToolCall?.[`${part}PayloadId`] || compressedPayloadId || '').trim();
      if ((loaded == null || loaded === '' || compressedPayloadId) && payloadId) {
        loaded = await fetchPayloadById(payloadId);
      }
      if (request === payloadRequest.current) setPayloadView({loading:false,payload:loaded,error:''});
    } catch (err) {
      if (request === payloadRequest.current) setPayloadView({
        loading: false,
        payload: null,
        error: String(err?.message || err || 'failed to load payload')
      });
    }
  };

  const copyPayload = async () => {
    if (payloadView?.payload == null) return;
    try {
      await navigator.clipboard.writeText(stringifyPayload(payloadView.payload));
      setCopyStatus('Copied');
    } catch (_) {setCopyStatus('Copy failed. Try again.');}
  };

  const isModel = String(effectiveToolCall?.kind || '').toLowerCase() === 'model';
  const tabs = [{windowId:'overview',label:'Overview'},{windowId:'request',label:'Request'},{windowId:'response',label:'Response'},
    ...(isModel ? [{windowId:'stream',label:'Stream'},{windowId:'providerRequest',label:'Provider request'},{windowId:'providerResponse',label:'Provider response'}] : [])];
  return (
    <aside className={`app-detail-panel app-detail-panel-dialog${documentAppearance ? ' is-document' : ''}`} {...(documentAppearance ? typographyProps('body') : {})}>
      <div className="app-detail-head app-detail-head-compact">
        {documentAppearance ? <>
          <div className="app-execution-heading"><Icon name={isModel ? 'lucide:message-square' : 'lucide:sliders-horizontal'}/>
            <div><Typography as="h2" id={headingId} typeRole="section">Step details</Typography><Typography as="p" typeRole="metadata">{displayStepTitle(effectiveToolCall)} · {isModel ? 'Model call' : 'Execution step'}</Typography></div>
          </div>
          <Typography typeRole="metadata" className="app-execution-status" data-status={effectiveToolCall.status}>{String(effectiveToolCall.status || 'Unknown').replace(/_/g,' ')}</Typography>
          <SharedButton variant="ghost" size="compact" icon="lucide:x" hideLabel aria-label="Close execution details" onClick={onClose}/>
        </> : <>
        <div className="app-detail-tool-row">
          <span className={`app-detail-kind kind-${isModel ? 'model' : 'tool'}`}>
            {displayStepIcon(effectiveToolCall || {})}
          </span>
          <span id={headingId} className="app-detail-tool-name">{displayStepTitle(effectiveToolCall || {})}</span>
          {phaseLabel(effectiveToolCall) ? (
            <span className="app-detail-status-chip">{phaseLabel(effectiveToolCall)}</span>
          ) : null}
          <span className="app-detail-status-chip">{String(effectiveToolCall?.status || '').toLowerCase() || 'unknown'}</span>
        </div>
        <Button minimal small icon="cross" onClick={onClose} />
        </>}
      </div>

      <Typography as="div" typeRole="label" className="app-execution-navigation">
        <WorkspaceRuntimeTabs id={panelId} entries={tabs} selectedId={activePart} onSelect={part=>{openPayload(part);}} ariaLabel="Step detail views" className="app-execution-tabs" tabClassName="app-execution-tab"/>
      </Typography>
      <div className="app-execution-content" id={`${panelId}-panel-${encodeURIComponent(activePart)}`} role="tabpanel" tabIndex={0} aria-labelledby={`${panelId}-tab-${encodeURIComponent(activePart)}`}>
      {activePart === 'overview' ? <>
      <div className="app-detail-section app-detail-section-summary">
        {documentAppearance ? <dl className="app-execution-facts">
          <div><Typography as="dt" typeRole="metadata">{isModel ? 'Model' : 'Step'}</Typography><Typography as="dd" typeRole="body">{isModel ? effectiveToolCall.model || 'Not recorded' : displayStepTitle(effectiveToolCall)}</Typography></div>
          {isModel ? <div><Typography as="dt" typeRole="metadata">Provider</Typography><Typography as="dd" typeRole="body">{effectiveToolCall.provider || 'Not recorded'}</Typography></div> : null}
          <div><Typography as="dt" typeRole="metadata">Duration</Typography><Typography as="dd" typeRole="body">{effectiveToolCall.latencyMs != null ? `${Math.round(effectiveToolCall.latencyMs)} ms` : 'Not recorded'}</Typography></div>
        </dl> : <>
          {isModel && effectiveToolCall?.provider ? <div><strong>Provider:</strong> {effectiveToolCall.provider}</div> : null}
          {isModel && effectiveToolCall?.model ? <div><strong>Model:</strong> {effectiveToolCall.model}</div> : null}
          {effectiveToolCall?.latencyMs ? <div><strong>Duration:</strong> {Math.round(effectiveToolCall.latencyMs)}ms</div> : null}
        </>}
        {errorText ? <div className="app-detail-error"><strong>Error:</strong> {errorText}</div> : null}
        {isModel && tokenUsage ? (
          <div className="app-detail-token-usage">
            <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip">Prompt {tokenUsage.prompt ?? 'n/a'}</span>
            <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip">Completion {tokenUsage.completion ?? 'n/a'}</span>
            <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip app-detail-token-chip-total">Total {tokenUsage.total ?? 'n/a'}</span>
            {tokenCost ? (
              <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip app-detail-token-chip-total">{`Est. ${formatUsdEstimate(tokenCost.total)}`}</span>
            ) : null}
          </div>
        ) : null}
        {hasToolPayloadAccounting ? (
          <div className="app-detail-tool-token-estimate">
            <div className="app-detail-token-usage">
              {toolTokenUsage.toolInput.available ? (
                <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip" title="Tool arguments generated by the model are billed as model output">
                  {`Arguments ≈${toolTokenUsage.toolInput.tokens} output tokens · ${toolTokenUsage.toolInput.bytes} B${toolTokenUsage.toolInput.cost != null ? ` · ${formatUsdEstimate(toolTokenUsage.toolInput.cost)}` : ''}`}
                </span>
              ) : <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip">Arguments unavailable</span>}
              {toolTokenUsage.toolOutput.available ? (
                <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip" title="Tool results presented to the model are billed as model input">
                  {`Result ≈${toolTokenUsage.toolOutput.tokens} input tokens · ${toolTokenUsage.toolOutput.bytes} B${toolTokenUsage.toolOutput.cost != null ? ` · ${formatUsdEstimate(toolTokenUsage.toolOutput.cost)}` : ''}`}
                </span>
              ) : <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip">Result unavailable</span>}
              <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip app-detail-token-chip-total">
                {`Tool payload ≈${toolTokenUsage.totalTokens} tokens`}
              </span>
              {toolTokenUsage.totalCost != null ? (
                <span {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-chip app-detail-token-chip-total">
                  {`Est. ${formatUsdEstimate(toolTokenUsage.totalCost)}`}
                </span>
              ) : null}
            </div>
            <div {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-estimate-note">
              {`Estimated at ${toolTokenUsage.bytesPerToken} UTF-8 bytes/token${toolTokenUsage.pricingModel ? ` using ${[toolTokenUsage.pricingProvider, toolTokenUsage.pricingModel].filter(Boolean).join('/')} pricing` : ''}; this is an attribution breakdown, not an addition to provider-reported totals.`}
            </div>
            {toolTokenUsage.toolOutput.overflow?.overflow ? (
              <div {...(documentAppearance ? typographyProps('metadata') : {})} className="app-detail-token-overflow-note">
                Output overflowed; only bytes presented to the model are included.
                {toolTokenUsage.toolOutput.overflow.remaining != null ? ` ${toolTokenUsage.toolOutput.overflow.remaining} units remain.` : ''}
              </div>
            ) : null}
          </div>
        ) : null}
        {linkedConversationId ? (
          <div className="app-detail-thread-link">
            <strong>Linked conversation:</strong>
            <ReviewAction appearance={appearance} small minimal onClick={() => openLinkedConversation(linkedConversationId, onClose)}>{documentAppearance ? 'Open conversation' : 'Open Thread →'}</ReviewAction>
          </div>
        ) : null}
        {canOpenElicitation ? (
          <div className="app-detail-thread-link">
            <strong>Review dialog:</strong>
            <ReviewAction appearance={appearance} small minimal onClick={() => openElicitationFromDetail(effectiveToolCall, onClose)}>{documentAppearance ? 'Open review' : 'Open Review →'}</ReviewAction>
          </div>
        ) : null}
      </div>

      </> : <div className="app-execution-payload">
        <div className="app-execution-payload-toolbar">
          <Typography as="h3" typeRole="label">{tabs.find(tab=>tab.windowId===activePart)?.label}</Typography>
          <Typography typeRole="metadata" aria-live="polite">{copyStatus}</Typography>
          <ReviewAction appearance={appearance} small minimal onClick={copyPayload} disabled={payloadView?.loading || !!payloadView?.error || payloadView?.payload == null}>Copy</ReviewAction>
        </div>
        {payloadView?.loading ? <Typography as="div" typeRole="body" className="app-execution-state" role="status">Loading {tabs.find(tab=>tab.windowId===activePart)?.label.toLowerCase()}…</Typography> : null}
        {payloadView?.error ? <div className="app-execution-state" role="alert">
          <Typography as="h3" typeRole="label">Couldn’t load this data</Typography>
          <Typography as="p" typeRole="body">{payloadView.error}</Typography>
          <ReviewAction appearance={appearance} small onClick={()=>openPayload(activePart)}>Try again</ReviewAction>
        </div> : null}
        {!payloadView?.loading && !payloadView?.error && payloadView?.payload != null ? (
          <Typography as="div" typeRole="code" className="app-execution-code">
            <CodeMirror value={stringifyPayload(payloadView.payload)} extensions={[json()]} editable={false}
              basicSetup={{lineNumbers:true,foldGutter:true}} height="100%"
              theme={documentAppearance ? workspace?.state?.mode === 'dark' ? 'dark' : 'light' : 'dark'}/>
          </Typography>
        ) : null}
        {!payloadView?.loading && !payloadView?.error && payloadView?.payload == null ? <div className="app-execution-state">
          <Typography as="h3" typeRole="label">No data available</Typography>
          <Typography as="p" typeRole="body">This step has no {tabs.find(tab=>tab.windowId===activePart)?.label.toLowerCase()} data available.</Typography>
          <ReviewAction appearance={appearance} small onClick={()=>openPayload(activePart)}>Refresh</ReviewAction>
        </div> : null}
      </div>}
      </div>
    </aside>
  );
}
