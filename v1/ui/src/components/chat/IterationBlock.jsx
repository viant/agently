import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Dialog } from '@blueprintjs/core';
import { DetailContext } from '../../context/DetailContext';
import { openLinkedConversationWindow } from '../../services/conversationWindow';
import { fetchTranscript } from '../../services/chatRuntime';
import { displayStepIcon, displayStepTitle, isAgentRunTool, humanizeAgentId } from '../../services/toolPresentation';
import BubbleMessage from './BubbleMessage';
import ElicitationForm from './ElicitationForm';

const GROUPS_VISIBLE = 'all';
const TOOLS_PER_GROUP = 3;
const GROUP_PAGE_SIZES = [1, 5, 10, 'all'];

function statusLabel(value) {
  const text = String(value || '').trim();
  if (!text) return 'running';
  if (text.toLowerCase() === 'cancelled') return 'canceled';
  return text.replace(/_/g, ' ');
}

function isActiveStatus(value) {
  const text = String(value || '').trim().toLowerCase();
  return text === '' || text === 'running' || text === 'thinking' || text === 'processing' || text === 'in_progress' || text === 'queued';
}

function statusTone(value) {
  const text = String(value || '').trim().toLowerCase();
  if (!text || text === 'running' || text === 'thinking' || text === 'processing' || text === 'in_progress') {
    return 'running';
  }
  if (text === 'completed' || text === 'succeeded' || text === 'success') {
    return 'success';
  }
  if (text === 'failed' || text === 'error' || text === 'cancelled' || text === 'canceled' || text === 'terminated') {
    return 'error';
  }
  if (text === 'waiting_for_user' || text === 'blocked') {
    return 'warning';
  }
  return 'neutral';
}

function statusIcon(value) {
  switch (statusTone(value)) {
    case 'success':
      return '✔';
    case 'error':
      return '!';
    case 'warning':
      return '•';
    case 'running':
      return '◔';
    default:
      return '•';
  }
}

function stepTitle(step) {
  return displayStepTitle(step);
}

function rawToolTitle(step = {}) {
  return String(step?.toolName || step?.ToolName || 'tool');
}

export function displayItemRowTitle(step = {}) {
  if (String(step?.kind || '').toLowerCase() === 'elicitation') {
    return String(step?.message || step?.toolName || 'Input required').slice(0, 60);
  }
  return rawToolTitle(step);
}

export function displayItemRowIcon(step = {}) {
  if (String(step?.kind || '').toLowerCase() === 'elicitation') return '⌨️';
  return '🛠';
}

export function displayLinkedConversationTitle() {
  return 'Linked conversation';
}

export function displayLinkedConversationIcon() {
  return '🔗';
}

export function paginateToolSteps(toolSteps = [], rawOffset = null, pageSize = TOOLS_PER_GROUP) {
  const steps = Array.isArray(toolSteps) ? toolSteps : [];
  const total = steps.length;
  if (total <= pageSize) {
    return { tools: steps, start: 0, end: total, total, hasMore: false, atLatest: true, atOldest: true };
  }
  const maxOffset = Math.max(0, total - pageSize);
  const offset = rawOffset == null ? maxOffset : Math.min(Math.max(0, rawOffset), maxOffset);
  return {
    tools: steps.slice(offset, offset + pageSize),
    start: offset,
    end: Math.min(offset + pageSize, total),
    total,
    hasMore: true,
    atLatest: rawOffset == null || offset >= maxOffset,
    atOldest: offset <= 0
  };
}

export function olderToolPageOffset(total = 0, rawOffset = null, pageSize = TOOLS_PER_GROUP) {
  const maxOffset = Math.max(0, Number(total || 0) - pageSize);
  const current = rawOffset == null ? maxOffset : rawOffset;
  return Math.max(0, current - pageSize);
}

export function newerToolPageOffset(total = 0, rawOffset = null, pageSize = TOOLS_PER_GROUP) {
  const maxOffset = Math.max(0, Number(total || 0) - pageSize);
  const current = rawOffset == null ? maxOffset : rawOffset;
  const next = Math.min(maxOffset, current + pageSize);
  return next >= maxOffset ? null : next;
}

function latencyLabel(step, now = Date.now()) {
  const explicit = formatDurationClock(totalLatencyMs([step]));
  if (explicit) return explicit;
  if (!isActiveStatus(step?.status)) return '';
  const startedAt = earliestStartedAt([step]);
  if (!startedAt) return '';
  return formatDurationClock(Math.max(0, now - startedAt));
}

function aggregateLatencyLabel(steps = []) {
  return formatDurationClock(totalLatencyMs(steps));
}

function totalLatencyMs(steps = []) {
  return (Array.isArray(steps) ? steps : [])
    .map((item) => {
      const explicit = Number(item?.latencyMs || item?.durationMs || 0);
      if (Number.isFinite(explicit) && explicit > 0) return explicit;
      const startedAt = Date.parse(String(item?.startedAt || item?.StartedAt || ''));
      const completedAt = Date.parse(String(item?.completedAt || item?.CompletedAt || ''));
      if (Number.isFinite(startedAt) && Number.isFinite(completedAt) && completedAt >= startedAt) {
        return completedAt - startedAt;
      }
      return 0;
    })
    .filter((value) => Number.isFinite(value) && value > 0)
    .reduce((sum, value) => sum + value, 0);
}

function earliestStartedAt(steps = []) {
  let earliest = 0;
  for (const item of Array.isArray(steps) ? steps : []) {
    const value = Date.parse(String(item?.startedAt || item?.StartedAt || item?.createdAt || item?.CreatedAt || ''));
    if (!Number.isFinite(value)) continue;
    if (earliest === 0 || value < earliest) {
      earliest = value;
    }
  }
  return earliest;
}

function formatDurationClock(total) {
  if (!Number.isFinite(total) || total <= 0) return '';
  const seconds = Math.max(0, Math.round(total / 1000));
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(remainder).padStart(2, '0')}`;
}

function plainText(value) {
  return String(value || '')
    .replace(/[`*_>#-]/g, ' ')
    .replace(/\[(.*?)\]\((.*?)\)/g, '$1')
    .replace(/\s+/g, ' ')
    .trim();
}

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

function truncate(value, limit = 80) {
  const text = plainText(value);
  if (text.length <= limit) return text;
  return `${text.slice(0, Math.max(0, limit - 1)).trimEnd()}…`;
}

function normalizedTextKey(value) {
  return plainText(value)
    .toLowerCase()
    .replace(/\b(i am|i'm|i will|i'll|going to)\b/g, ' ')
    .replace(/[.,;:!?()[\]{}"'`]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function deriveToolNamesFromModelStep(step = {}) {
  const payload = step?.responsePayload
    || step?.providerResponsePayload
    || parseMaybeJSON(step?.responsePayload)
    || parseMaybeJSON(step?.providerResponsePayload)
    || null;
  const choices = Array.isArray(payload?.choices) ? payload.choices : [];
  const names = [];
  for (const choice of choices) {
    const toolCalls = Array.isArray(choice?.message?.tool_calls) ? choice.message.tool_calls : [];
    for (const call of toolCalls) {
      const name = String(call?.name || call?.function?.name || '').trim();
      if (name) names.push(name);
    }
  }
  return [...new Set(names)];
}

function deriveToolNamesFromSteps(steps = []) {
  const names = [];
  (Array.isArray(steps) ? steps : []).forEach((step) => {
    const kind = String(step?.kind || '').toLowerCase();
    if (kind === 'model') {
      names.push(...deriveToolNamesFromModelStep(step));
      return;
    }
    const toolName = String(step?.toolName || '').trim();
    if (toolName && toolName.toLowerCase() !== 'tool') {
      names.push(toolName);
    }
  });
  return [...new Set(names)];
}

function groupTitleFromSteps({ preamble, modelStep, toolSteps = [] } = {}) {
  const explicit = truncate(preamble?.content || '', 80);
  if (explicit) return explicit;
  const derivedNames = deriveToolNamesFromModelStep(modelStep);
  if (derivedNames.length > 0) {
    return truncate(`Using ${derivedNames.join(', ')}.`, 80);
  }
  const namedTools = toolSteps
    .map((step) => String(step?.toolName || '').trim())
    .filter(Boolean);
  if (namedTools.length > 0) {
    return truncate(`Using ${[...new Set(namedTools)].join(', ')}.`, 80);
  }
  if (modelStep) {
    return truncate(stepTitle(modelStep), 80);
  }
  return 'Execution step';
}

function normalizeToolName(name) {
  return String(name || '').trim().toLowerCase().replace(/[:\-_]/g, '');
}

function toolStepKey(step = {}) {
  const explicitID = String(step?.id || '').trim();
  if (explicitID) return explicitID;
  const toolCallID = String(step?.toolCallId || step?.ToolCallID || '').trim();
  if (toolCallID) return `call:${toolCallID}`;
  const requestId = String(step?.requestPayloadId || '').trim();
  const responseId = String(step?.responsePayloadId || '').trim();
  if (requestId || responseId) {
    return `payload:${requestId}:${responseId}:${normalizeToolName(step?.toolName)}`;
  }
  return `name:${normalizeToolName(step?.toolName)}`;
}

function agentLabel(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  return text
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (ch) => ch.toUpperCase());
}

function matchesAgentEntry(entry = {}, target = '') {
  const normalizedTarget = String(target || '').trim();
  if (!normalizedTarget) return false;
  const candidates = [
    entry?.id,
    entry?.value,
    entry?.name,
    entry?.label,
    entry?.title
  ].map((value) => String(value || '').trim()).filter(Boolean);
  return candidates.includes(normalizedTarget);
}

export function resolveIterationAgentLabel(data = {}, context = null) {
  const explicitAgentId = String(data?.agentId || '').trim();
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const convForm = conversationsDS?.peekFormData?.() || {};
  const metaForm = metaDS?.peekFormData?.() || {};

  const selectedAgentId = explicitAgentId
    || String(convForm?.agent || '').trim()
    || String(metaForm?.defaults?.agent || '').trim();
  const explicitAgentName = String(convForm?.agentName || '').trim();
  if (explicitAgentName) return explicitAgentName;
  if (!selectedAgentId) return '';

  const byKey = metaForm?.agentInfo?.[selectedAgentId] || null;
  const keyedName = String(byKey?.label || byKey?.name || byKey?.title || '').trim();
  if (keyedName) return keyedName;

  const optionLists = [
    ...(Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : []),
    ...(Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [])
  ];
  const matched = optionLists.find((entry) => matchesAgentEntry(entry, selectedAgentId)) || null;
  const matchedName = String(matched?.label || matched?.name || matched?.title || '').trim();
  if (matchedName) return matchedName;

  return agentLabel(humanizeAgentId(selectedAgentId));
}

export function resolveIterationModelLabel(context = null) {
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const convForm = conversationsDS?.peekFormData?.() || {};
  const metaForm = metaDS?.peekFormData?.() || {};
  return String(
    convForm?.model
    || metaForm?.model
    || metaForm?.defaults?.model
    || ''
  ).trim();
}

export function mapCanonicalExecutionGroups(groups = []) {
  return (Array.isArray(groups) ? groups : []).map((group, index) => {
    /* Canonical page fields (preferred) with legacy ExecutionGroup fallbacks */
    const modelStep0 = (Array.isArray(group?.modelSteps) ? group.modelSteps : [])[0]
      || group?.modelCall || group?.ModelCall || null;
    const toolStepsRaw = Array.isArray(group?.toolSteps) ? group.toolSteps
      : Array.isArray(group?.toolCalls || group?.ToolCalls) ? (group.toolCalls || group.ToolCalls)
      : [];
    const plannedToolCalls = Array.isArray(group?.toolCallsPlanned || group?.ToolCallsPlanned)
      ? (group.toolCallsPlanned || group.ToolCallsPlanned)
      : [];
    /* Build toolMessage lookup: prefer canonical toolSteps, then legacy toolMessages */
    const toolMessageByID = new Map();
    const legacyToolMessages = Array.isArray(group?.toolMessages || group?.ToolMessages) ? (group.toolMessages || group.ToolMessages) : [];
    legacyToolMessages.forEach((message) => {
      const id = String(message?.id || message?.Id || message?.toolCall?.messageId || message?.ToolCall?.MessageId || '').trim();
      if (id) toolMessageByID.set(id, message);
    });
    toolStepsRaw.forEach((ts) => {
      const id = String(ts?.toolMessageId || ts?.id || ts?.Id || '').trim();
      if (id && !toolMessageByID.has(id)) toolMessageByID.set(id, ts);
    });
    const modelStep = modelStep0 ? {
      id: modelStep0?.modelCallId || group?.modelMessageId || group?.ModelMessageID || group?.parentMessageId || group?.ParentMessageID || `model:${index}`,
      kind: 'model',
      reason: group?.finalResponse || group?.FinalResponse ? 'final_response' : 'thinking',
      provider: modelStep0?.provider || modelStep0?.Provider || '',
      model: modelStep0?.model || modelStep0?.Model || '',
      status: modelStep0?.status || modelStep0?.Status || group?.status || group?.Status || '',
      latencyMs: modelStep0?.latencyMs || modelStep0?.LatencyMs || null,
      startedAt: modelStep0?.startedAt || modelStep0?.StartedAt || '',
      completedAt: modelStep0?.completedAt || modelStep0?.CompletedAt || '',
      requestPayloadId: modelStep0?.requestPayloadId || modelStep0?.RequestPayloadId || '',
      responsePayloadId: modelStep0?.responsePayloadId || modelStep0?.ResponsePayloadId || '',
      providerRequestPayloadId: modelStep0?.providerRequestPayloadId || modelStep0?.ProviderRequestPayloadId || '',
      providerResponsePayloadId: modelStep0?.providerResponsePayloadId || modelStep0?.ProviderResponsePayloadId || '',
      streamPayloadId: modelStep0?.streamPayloadId || modelStep0?.StreamPayloadId || '',
      requestPayload: modelStep0?.modelCallRequestPayload || modelStep0?.ModelCallRequestPayload || modelStep0?.requestPayload || modelStep0?.RequestPayload || null,
      responsePayload: modelStep0?.modelCallResponsePayload || modelStep0?.ModelCallResponsePayload || modelStep0?.responsePayload || modelStep0?.ResponsePayload || null,
      providerRequestPayload: modelStep0?.modelCallProviderRequestPayload || modelStep0?.ModelCallProviderRequestPayload || null,
      providerResponsePayload: modelStep0?.modelCallProviderResponsePayload || modelStep0?.ModelCallProviderResponsePayload || null,
      streamPayload: modelStep0?.modelCallStreamPayload || modelStep0?.ModelCallStreamPayload || null
    } : null;
    const actualToolSteps = toolStepsRaw.map((ts, toolIndex) => {
      const messageID = String(ts?.toolMessageId || ts?.messageId || ts?.MessageId || '').trim();
      const toolMessage = messageID ? toolMessageByID.get(messageID) : null;
      return {
        id: messageID || ts?.toolCallId || ts?.opId || ts?.OpId || `tool:${index}:${toolIndex}`,
        toolCallId: ts?.toolCallId || ts?.ToolCallID || ts?.opId || ts?.OpId || '',
        kind: 'tool',
        reason: 'tool_call',
        toolName: ts?.toolName || ts?.ToolName || toolMessage?.toolName || toolMessage?.ToolName || 'tool',
        status: ts?.status || ts?.Status || '',
        latencyMs: ts?.latencyMs || ts?.LatencyMs || null,
        startedAt: ts?.startedAt || ts?.StartedAt || '',
        completedAt: ts?.completedAt || ts?.CompletedAt || '',
        requestPayloadId: ts?.requestPayloadId || ts?.RequestPayloadId || '',
        responsePayloadId: ts?.responsePayloadId || ts?.ResponsePayloadId || '',
        requestPayload: ts?.requestPayload || ts?.RequestPayload || null,
        responsePayload: ts?.responsePayload || ts?.ResponsePayload || null,
        linkedConversationId: ts?.linkedConversationId || ts?.LinkedConversationId || toolMessage?.linkedConversationId || toolMessage?.LinkedConversationId || ''
      };
    });
    const actualByKey = new Set(actualToolSteps.map((step) => toolStepKey(step)));
    const plannedToolSteps = plannedToolCalls
      .map((call, toolIndex) => ({
        id: call?.toolCallId || call?.ToolCallID || `planned:${index}:${toolIndex}:${call?.toolName || call?.ToolName || 'tool'}`,
        toolCallId: call?.toolCallId || call?.ToolCallID || '',
        kind: 'tool',
        reason: 'tool_call',
        toolName: call?.toolName || call?.ToolName || 'tool',
        status: modelStep0?.status || group?.status || group?.Status || 'planned',
        latencyMs: null,
        requestPayloadId: '',
        responsePayloadId: '',
        requestPayload: null,
        responsePayload: null,
        linkedConversationId: ''
      }))
      .filter((step) => {
        const key = toolStepKey(step);
        return key && !actualByKey.has(key);
      });
    const toolSteps = [...actualToolSteps, ...plannedToolSteps];
    const preambleContent = String(group?.preamble || group?.Preamble || '').trim();
    const status = String(group?.status || group?.Status || modelStep?.status || '').trim();
    const title = groupTitleFromSteps({
      preamble: preambleContent ? { content: preambleContent } : null,
      modelStep,
      toolSteps
    });
    return {
      id: String(group?.parentMessageId || group?.ParentMessageID || group?.modelMessageId || group?.ModelMessageID || `group:${index}`),
      title,
      fullTitle: plainText(preambleContent || title),
      preambleContent,
      modelStep,
      toolSteps,
      detailStep: modelStep || toolSteps[0] || null,
      status,
      finalResponse: Boolean(group?.finalResponse || group?.FinalResponse),
      finalContent: String(group?.content || group?.Content || '').trim(),
      elapsed: aggregateLatencyLabel([...(modelStep ? [modelStep] : []), ...toolSteps]),
      stepCount: (modelStep ? 1 : 0) + toolSteps.length
    };
  });
}

function mapFallbackExecutionGroups(data = {}) {
  const steps = Array.isArray(data?.toolCalls) ? data.toolCalls : [];
  if (steps.length === 0) return [];
  const modelSteps = steps.filter((step) => String(step?.kind || '').toLowerCase() === 'model');
  const toolSteps = steps.filter((step) => String(step?.kind || '').toLowerCase() !== 'model');
  const primaryModel = modelSteps[0] || null;
  const status = String(
    data?.status
    || primaryModel?.status
    || toolSteps[toolSteps.length - 1]?.status
    || ''
  ).trim();
  return [{
    id: `fallback:${String(data?.turnId || 'iteration').trim() || 'iteration'}`,
    title: groupTitleFromSteps({
      preamble: data?.preamble || null,
      modelStep: primaryModel,
      toolSteps
    }),
    fullTitle: '',
    preambleContent: String(data?.preamble?.content || '').trim(),
    modelStep: primaryModel,
    toolSteps,
    detailStep: primaryModel || toolSteps[0] || null,
    status,
    finalResponse: false,
    finalContent: '',
    elapsed: aggregateLatencyLabel([...(primaryModel ? [primaryModel] : []), ...toolSteps]),
    stepCount: (primaryModel ? 1 : 0) + toolSteps.length
  }];
}

export function resolveVisibleBubbleContent(visibleGroups = []) {
  const groups = Array.isArray(visibleGroups) ? visibleGroups : [];
  for (let index = groups.length - 1; index >= 0; index -= 1) {
    const group = groups[index] || {};
    const finalText = String(group?.finalContent || '').trim();
    if (group?.finalResponse && finalText) {
      return finalText;
    }
  }
  for (let index = groups.length - 1; index >= 0; index -= 1) {
    const group = groups[index] || {};
    const preambleText = String(group?.preambleContent || '').trim();
    if (preambleText) {
      return preambleText;
    }
  }
  return '';
}

export function resolveIterationBubbleContent({
  visibleGroups = [],
  iterationContent = '',
  responseContent = '',
  preambleContent = '',
  streamContent = ''
} = {}) {
  return String(
    resolveVisibleBubbleContent(visibleGroups)
    || iterationContent
    || responseContent
    || preambleContent
    || streamContent
    || ''
  ).trim();
}

export function shouldShowPreambleBubble(visibleGroups = [], visibleText = '') {
  return String(visibleText || '').trim() !== '';
}

function linkedConversationId(step = {}) {
  return String(step?.linkedConversationId || step?.LinkedConversationId || '').trim();
}

function canOpenLinkedConversation(step = {}) {
  const id = linkedConversationId(step);
  if (!id) return false;
  const kind = String(step?.kind || '').toLowerCase();
  const reason = String(step?.reason || '').toLowerCase();
  if (reason === 'link') return true;
  if (kind !== 'tool') return false;
  return isAgentRunTool(step);
}

function isTerminalTurnStatus(value) {
  const text = String(value || '').trim().toLowerCase();
  return text === 'completed'
    || text === 'succeeded'
    || text === 'success'
    || text === 'done'
    || text === 'failed'
    || text === 'error'
    || text === 'canceled'
    || text === 'cancelled';
}

function openLinkedConversation(step = {}) {
  const id = linkedConversationId(step);
  if (!id) return;
  openLinkedConversationWindow(id);
}

function paginate(total, visible, offset) {
  const effectiveVisible = visible === 'all' ? total : visible;
  if (total <= effectiveVisible) {
    return { total, start: 0, end: total, maxOffset: 0, anchoredLatest: true };
  }
  const maxOffset = Math.max(0, total - effectiveVisible);
  const anchoredLatest = offset == null;
  const safeOffset = anchoredLatest
    ? maxOffset
    : Math.min(Math.max(0, offset), maxOffset);
  return {
    total,
    start: safeOffset,
    end: Math.min(total, safeOffset + effectiveVisible),
    maxOffset,
    anchoredLatest
  };
}

function isPresentableGroup(group = {}) {
  const preambleText = String(group?.preambleContent || '').trim();
  const finalText = String(group?.finalContent || '').trim();
  const toolCount = Array.isArray(group?.toolSteps) ? group.toolSteps.length : 0;
  const plannedCount = Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned.length : 0;
  const isFinal = !!group?.modelStep && String(group?.modelStep?.reason || '').toLowerCase() === 'final_response';
  return isFinal || toolCount > 0 || plannedCount > 0 || preambleText !== '' || finalText !== '';
}

export function buildSyntheticModelGroup({ data = {}, message = {}, context = null, visibleText = '' } = {}) {
  const text = String(visibleText || '').trim();
  if (!text && !isActiveStatus(data?.status || message?.status)) return null;
  const modelLabel = resolveIterationModelLabel(context);
  const status = String(data?.status || message?.turnStatus || message?.status || (text ? 'completed' : 'running')).trim();
  const finalResponse = text !== '' && !isActiveStatus(status);
  const content = finalResponse ? text : '';
  const preambleContent = finalResponse ? '' : text;
  return {
    id: `synthetic:${String(message?.id || data?.turnId || 'iteration').trim() || 'iteration'}`,
    title: modelLabel || 'model',
    fullTitle: modelLabel || 'model',
    preambleContent,
    modelStep: {
      id: `synthetic-model:${String(message?.id || data?.turnId || 'iteration').trim() || 'iteration'}`,
      kind: 'model',
      reason: finalResponse ? 'final_response' : 'thinking',
      provider: '',
      model: modelLabel || 'model',
      status
    },
    toolSteps: [],
    detailStep: {
      id: `synthetic-model:${String(message?.id || data?.turnId || 'iteration').trim() || 'iteration'}`,
      kind: 'model',
      reason: finalResponse ? 'final_response' : 'thinking',
      provider: '',
      model: modelLabel || 'model',
      status
    },
    status,
    finalResponse,
    finalContent: content,
    elapsed: '',
    stepCount: 1
  };
}

export function resolveIterationStatusDetail(data = {}) {
  const errorText = String(data?.errorMessage || data?.response?.errorMessage || '').trim();
  if (errorText) return errorText;
  const status = String(data?.status || '').trim().toLowerCase();
  if (status === 'canceled' || status === 'cancelled') return 'Canceled';
  if (status === 'terminated') return 'Terminated';
  return '';
}

function lastPresentableGroupIndex(groups = []) {
  const list = Array.isArray(groups) ? groups : [];
  for (let index = list.length - 1; index >= 0; index -= 1) {
    if (isPresentableGroup(list[index])) return index;
  }
  return Math.max(0, list.length - 1);
}

function currentConversationId() {
  if (typeof window === 'undefined') return '';
  const match = String(window.location?.pathname || '').match(/\/conversation\/([^/?#]+)/);
  return match ? decodeURIComponent(match[1]) : '';
}

export default function IterationBlock({ message, context }) {
  const { showDetail } = useContext(DetailContext);
  const data = message?._iterationData || {};
  const toolCalls = Array.isArray(data.toolCalls) ? data.toolCalls : [];
  const displayToolCalls = useMemo(
    () => (Array.isArray(toolCalls) ? [...toolCalls] : []),
    [toolCalls]
  );
  const isLatestIteration = !!data?.isLatestIteration;
  const [preambleOffset, setPreambleOffset] = useState(null);
  const [groupPageSize, setGroupPageSize] = useState(GROUPS_VISIBLE);
  const [groupToolOffsets, setGroupToolOffsets] = useState({});
  const [remotePage, setRemotePage] = useState(null);
  const [now, setNow] = useState(Date.now());
  const [isElicitationOpen, setIsElicitationOpen] = useState(false);

  const stepKey = (step) => {
    const explicitID = String(step?.id || '').trim();
    if (explicitID) return explicitID;
    return [
      String(step?.kind || ''),
      normalizeToolName(step?.toolName),
      String(step?.requestPayloadId || ''),
      String(step?.responsePayloadId || ''),
      String(step?.providerRequestPayloadId || ''),
      String(step?.providerResponsePayloadId || ''),
      String(step?.streamPayloadId || '')
    ].join('::');
  };

  const allGroupEntries = useMemo(() => {
    const canonicalGroups = Array.isArray(data.executionGroups) ? data.executionGroups : [];
    const groups = canonicalGroups.length > 0
      ? mapCanonicalExecutionGroups(canonicalGroups)
      : mapFallbackExecutionGroups(data);
    // Inject elicitation as a step in the last group so it appears in execution details.
    const elic = message?.elicitation || data?.elicitation;
    const elicId = String(message?.elicitationId || data?.elicitationId || '').trim();
    if (elic && elicId && groups.length > 0) {
      const last = groups[groups.length - 1];
      const alreadyPresent = last.toolSteps.some((s) => s.elicitationId === elicId);
      if (!alreadyPresent) {
        const elicStatus = String(message?.status || data?.status || '').trim().toLowerCase();
        const elicStep = {
          id: `elicitation:${elicId}`,
          elicitationId: elicId,
          kind: 'elicitation',
          reason: 'elicitation',
          toolName: String(elic?.message || 'Input required').slice(0, 60),
          message: elic?.message || '',
          status: elicStatus === 'pending' ? 'pending' : (elicStatus || 'pending'),
          startedAt: message?.createdAt || '',
          completedAt: '',
          latencyMs: null,
          requestedSchema: elic?.requestedSchema || null,
          url: elic?.url || '',
          mode: elic?.mode || ''
        };
        groups[groups.length - 1] = {
          ...last,
          toolSteps: [...last.toolSteps, elicStep],
          stepCount: last.stepCount + 1
        };
      }
    }
    return groups;
  }, [data, message?.elicitation, message?.elicitationId, message?.status, message?.createdAt, data?.elicitation, data?.elicitationId]);
  const visiblePreambleText = useMemo(() => resolveIterationBubbleContent({
    visibleGroups: allGroupEntries.filter((group) => isPresentableGroup(group)),
    iterationContent: message?.content,
    responseContent: data?.response?.content,
    preambleContent: data?.preamble?.content,
    streamContent: data?.streamContent
  }), [allGroupEntries, data?.preamble?.content, data?.response?.content, data?.streamContent, message?.content]);
  const displayGroupEntries = useMemo(
    () => {
      const presentable = allGroupEntries.filter((group) => isPresentableGroup(group));
      if (presentable.length > 0) return presentable;
      const synthetic = buildSyntheticModelGroup({ data, message, context, visibleText: visiblePreambleText });
      return synthetic ? [synthetic] : [];
    },
    [allGroupEntries, context, data, message, visiblePreambleText]
  );
  const isActiveIteration = useMemo(() => {
    if (isActiveStatus(data?.status)) return true;
    return allGroupEntries.some((group) => (
      isActiveStatus(group?.status)
      || isActiveStatus(group?.modelStep?.status)
      || (Array.isArray(group?.toolSteps) && group.toolSteps.some((step) => isActiveStatus(step?.status)))
    ));
  }, [allGroupEntries, data?.status]);
  const [collapsed, setCollapsed] = useState(() => !(isLatestIteration && isActiveIteration));
  const prevLatestRef = useRef(isLatestIteration);
  const prevActiveRef = useRef(isActiveIteration);
  const prevTerminalRef = useRef(isTerminalTurnStatus(data?.status));

  const elapsedLabel = useMemo(() => {
    const sourceSteps = allGroupEntries.flatMap((group) => [
      ...(group?.modelStep ? [group.modelStep] : []),
      ...(Array.isArray(group?.toolSteps) ? group.toolSteps : [])
    ]);
    const explicit = formatDurationClock(totalLatencyMs(sourceSteps));
    if (!isActiveIteration) return explicit;
    const startedAt = earliestStartedAt(sourceSteps) || Date.parse(String(message?.createdAt || ''));
    if (!startedAt) return explicit;
    return formatDurationClock(Math.max(0, now - startedAt)) || explicit;
  }, [allGroupEntries, isActiveIteration, message?.createdAt, now]);

  const canonicalTotal = Number(data?.executionGroupsTotal || 0) || displayGroupEntries.length;
  const effectiveVisibleCount = groupPageSize === 'all' ? canonicalTotal : groupPageSize;

  const linkedConversations = useMemo(() => {
    const seen = new Map();
    for (const group of allGroupEntries) {
      for (const step of group.toolSteps) {
        const id = linkedConversationId(step);
        if (!id || !canOpenLinkedConversation(step)) continue;
        if (!seen.has(id)) seen.set(id, step);
      }
    }
    return [...seen.values()];
  }, [allGroupEntries]);

  const iterationAgentLabel = useMemo(
    () => resolveIterationAgentLabel(data, context),
    [context, data]
  );
  const iterationStatusDetail = useMemo(
    () => resolveIterationStatusDetail(data),
    [data]
  );

  const getGroupToolPage = (group) => {
    return paginateToolSteps(group.toolSteps, groupToolOffsets[group.id], TOOLS_PER_GROUP);
  };

  const goToOlderGroupTools = (group) => {
    setGroupToolOffsets((prev) => {
      const total = group.toolSteps.length;
      return { ...prev, [group.id]: olderToolPageOffset(total, prev[group.id], TOOLS_PER_GROUP) };
    });
  };

  const goToNewerGroupTools = (group) => {
    setGroupToolOffsets((prev) => {
      const total = group.toolSteps.length;
      const next = newerToolPageOffset(total, prev[group.id], TOOLS_PER_GROUP);
      if (next == null) {
        const { [group.id]: _removed, ...rest } = prev;
        return rest;
      }
      return { ...prev, [group.id]: next };
    });
  };

  const groupsPage = useMemo(() => {
    if (remotePage) {
      const total = Number(remotePage.total || 0);
      const start = Number(remotePage.offset || 0);
      const end = Math.min(total, start + Number(remotePage.limit || 0));
      return {
        total,
        start,
        end,
        maxOffset: Math.max(0, total - (remotePage.limit || 0)),
        anchoredLatest: false
      };
    }
    return paginate(displayGroupEntries.length, groupPageSize, preambleOffset);
  }, [displayGroupEntries.length, groupPageSize, preambleOffset, remotePage]);

  const visibleGroups = useMemo(() => {
    if (remotePage) {
      return remotePage.groups;
    }
    const effectiveVisible = groupPageSize === 'all' ? displayGroupEntries.length : groupPageSize;
    if (displayGroupEntries.length <= effectiveVisible) return displayGroupEntries;
    let start = groupsPage.start;
    let end = groupsPage.end;
    if (groupPageSize === 1 && groupsPage.anchoredLatest && end === displayGroupEntries.length && displayGroupEntries.length > 1) {
      const targetIndex = lastPresentableGroupIndex(displayGroupEntries);
      start = targetIndex;
      end = targetIndex + 1;
    }
    return displayGroupEntries.slice(start, end);
  }, [displayGroupEntries, groupPageSize, groupsPage.end, groupsPage.start, remotePage]);

  const visibleRenderedText = useMemo(() => resolveIterationBubbleContent({
    visibleGroups,
    iterationContent: message?.content,
    responseContent: data?.response?.content,
    preambleContent: data?.preamble?.content,
    streamContent: data?.streamContent
  }), [data?.preamble?.content, data?.response?.content, data?.streamContent, message?.content, visibleGroups]);
  const hasVisibleElicitation = !!data?.response?.elicitation?.requestedSchema;
  const elicitationStatus = String(data?.response?.status || '').trim().toLowerCase();

  const renderGroupRow = (group, groupIndex) => (
    (() => {
      const modelStep = group.modelStep || null;
      return (
        <div className="app-iteration-group" key={`${group.id}:${groupIndex}`}>
      {modelStep ? (
        <div className="app-iteration-model-row">
          <div className="app-iteration-tool-row-main">
            <span className="app-iteration-model-icon">{displayStepIcon({ ...modelStep, kind: 'model' })}</span>
            <span className="app-iteration-model-title">{stepTitle({ ...modelStep, kind: 'model' })}</span>
            {group.title && group.title !== stepTitle({ ...modelStep, kind: 'model' }) ? (
              <span className="app-iteration-model-summary">{group.title}</span>
            ) : null}
          </div>
          <div className="app-iteration-tool-row-meta">
            <span className={`app-iteration-status tone-${statusTone(modelStep?.status)}`}>{statusLabel(modelStep?.status)}</span>
            <span className="app-iteration-group-time">{latencyLabel(modelStep, now) || '00:00'}</span>
            <Button
              minimal
              small
              className="app-iteration-link"
              onClick={() => showDetail?.({ ...modelStep, kind: 'model' })}
            >
              Details
            </Button>
          </div>
        </div>
      ) : null}
      {group.toolSteps.length > 0 ? (() => {
        const toolPage = getGroupToolPage(group);
        return (
          <div className="app-iteration-tool-list">
            {toolPage.hasMore ? (
              <div className="app-iteration-tool-paginator">
                <Button minimal small className="app-iteration-link app-iteration-link-subtle" disabled={toolPage.atOldest} onClick={() => goToOlderGroupTools(group)}>&laquo;</Button>
                <span className="app-iteration-inline-meta">{toolPage.start + 1}–{toolPage.end}/{toolPage.total}</span>
                <Button minimal small className="app-iteration-link app-iteration-link-subtle" disabled={toolPage.atLatest} onClick={() => goToNewerGroupTools(group)}>&raquo;</Button>
              </div>
            ) : null}
            {toolPage.tools.map((toolStep, toolIndex) => (
              <div className="app-iteration-tool-row" key={`${group.id}:tool:${toolPage.start + toolIndex}:${stepKey(toolStep)}`}>
                <div className="app-iteration-tool-row-main">
                  <span className="app-iteration-tool-icon">{displayItemRowIcon(toolStep)}</span>
                  <span className="app-iteration-tool-row-title">{displayItemRowTitle(toolStep)}</span>
                </div>
                <div className="app-iteration-tool-row-meta">
                  <span className={`app-iteration-status tone-${statusTone(toolStep?.status)}`}>{statusLabel(toolStep?.status)}</span>
                  <span className="app-iteration-group-time">{latencyLabel(toolStep, now) || '00:00'}</span>
                  <Button minimal small className="app-iteration-link" onClick={() => showDetail?.(toolStep)}>Details</Button>
                </div>
              </div>
            ))}
          </div>
        );
      })() : null}
        </div>
      );
    })()
  );

  const goToOlderPreambles = () => {
    if (displayGroupEntries.length > 0 && canonicalTotal > effectiveVisibleCount) {
      void loadHistoricalPage(Math.max(0, (remotePage ? remotePage.offset : Math.max(0, canonicalTotal - effectiveVisibleCount)) - effectiveVisibleCount));
      return;
    }
    setPreambleOffset((value) => {
      const current = value == null ? groupsPage.maxOffset : value;
      return Math.max(0, current - GROUPS_VISIBLE);
    });
  };

  const goToNewerPreambles = () => {
    if (remotePage) {
      const next = Math.min(groupsPage.maxOffset, remotePage.offset + effectiveVisibleCount);
      if (next >= groupsPage.maxOffset) {
        setRemotePage(null);
        setPreambleOffset(null);
        return;
      }
      void loadHistoricalPage(next);
      return;
    }
    setPreambleOffset((value) => {
      const current = value == null ? groupsPage.maxOffset : value;
      const next = Math.min(groupsPage.maxOffset, current + GROUPS_VISIBLE);
      return next >= groupsPage.maxOffset ? null : next;
    });
  };

  const goToLatestPreambles = () => {
    setRemotePage(null);
    setPreambleOffset(null);
  };

  const changeGroupPageSize = (value) => {
    setGroupPageSize(value);
    setRemotePage(null);
    setPreambleOffset(null);
  };

  const loadHistoricalPage = async (offset) => {
    const conversationID = currentConversationId();
    const turnID = String(data?.turnId || '').trim();
    if (!conversationID) return;
    const limit = groupPageSize === 'all' ? canonicalTotal : Number(groupPageSize || GROUPS_VISIBLE);
    const turns = await fetchTranscript(conversationID, '');
    const turn = Array.isArray(turns)
      ? (turns.find((entry) => String(entry?.id || entry?.Id || '').trim() === turnID) || turns[0])
      : null;
    const allGroups = mapCanonicalExecutionGroups(turn?.executionGroups || turn?.ExecutionGroups || []);
    const total = allGroups.length;
    const sliced = groupPageSize === 'all' ? allGroups : allGroups.slice(offset, offset + limit);
    setRemotePage({
      groups: sliced,
      total,
      offset,
      limit
    });
  };

  useEffect(() => {
    // Auto-collapse only when the turn becomes terminal. A completed model/tool
    // phase can be followed by another iteration, so do not treat "inactive"
    // as end-of-turn.
    const turnTerminal = isTerminalTurnStatus(data?.status);
    if (isLatestIteration && isActiveIteration) {
      setCollapsed(false);
    } else if (!prevTerminalRef.current && turnTerminal) {
      setCollapsed(true);
    }
    prevLatestRef.current = isLatestIteration;
    prevActiveRef.current = isActiveIteration;
    prevTerminalRef.current = turnTerminal;
  }, [data?.status, isActiveIteration, isLatestIteration]);

  useEffect(() => {
    if (isLatestIteration && isActiveIteration) {
      setCollapsed(false);
    }
  }, [message?.id, data?.turnId, isActiveIteration, isLatestIteration]);

  useEffect(() => {
    if (!isActiveIteration) return undefined;
    const timer = window.setInterval(() => setNow(Date.now()), 250);
    return () => window.clearInterval(timer);
  }, [isActiveIteration]);

  useEffect(() => {
    setRemotePage(null);
  }, [message?.id, data?.turnId, data?.executionGroupsTotal]);

  useEffect(() => {
    if (hasVisibleElicitation && (elicitationStatus === '' || elicitationStatus === 'pending' || elicitationStatus === 'open')) {
      setIsElicitationOpen(true);
    }
  }, [hasVisibleElicitation, elicitationStatus, message?.id]);

  return (
    <>
      <section className={`app-iteration-card tone-${statusTone(data?.status)}`}>
        <button type="button" className="app-iteration-head" onClick={() => setCollapsed((value) => !value)}>
          <span className="app-iteration-head-main">
            <span className="app-iteration-title">
              <span className="app-iteration-title-time">{elapsedLabel || '00:00'}</span>
              <span>
                Execution details
                {iterationAgentLabel ? ` · ${iterationAgentLabel}` : ''}
                {` (${displayGroupEntries.length})`}
              </span>
            </span>
            {iterationStatusDetail ? (
              <span className="app-iteration-status-detail" title={iterationStatusDetail}>{iterationStatusDetail}</span>
            ) : null}
          </span>
          <span className="app-iteration-toggle">{collapsed ? '▸' : '▾'}</span>
        </button>

        {!collapsed && displayGroupEntries.length > 0 ? (
          <>
            <div className="app-iteration-groups">
              {visibleGroups.map((group, index) => renderGroupRow(group, index))}
            </div>
            {canonicalTotal > effectiveVisibleCount ? (
              <div className="app-iteration-inline-paginator">
                <Button minimal small className="app-iteration-link app-iteration-link-subtle" disabled={groupsPage.start <= 0} onClick={goToOlderPreambles}>
                  &laquo; Older
                </Button>
                <span className="app-iteration-inline-meta">
                  {groupsPage.start + 1}–{groupsPage.end} of {groupsPage.total}
                </span>
                <Button minimal small className="app-iteration-link app-iteration-link-subtle" disabled={groupsPage.anchoredLatest} onClick={goToNewerPreambles}>
                  Newer &raquo;
                </Button>
                {!groupsPage.anchoredLatest ? (
                  <Button minimal small className="app-iteration-link app-iteration-link-subtle" onClick={goToLatestPreambles}>Latest</Button>
                ) : null}
                <label className="app-iteration-inline-meta" htmlFor={`page-size:${message?.id || 'iteration'}`}>
                  Page size
                </label>
                <select
                  id={`page-size:${message?.id || 'iteration'}`}
                  className="app-iteration-page-size"
                  value={String(groupPageSize)}
                  onChange={(event) => {
                    const value = event.target.value === 'all' ? 'all' : Number(event.target.value);
                    changeGroupPageSize(value);
                  }}
                >
                  {GROUP_PAGE_SIZES.map((size) => (
                    <option key={String(size)} value={String(size)}>
                      {size === 'all' ? 'All' : String(size)}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}
            {linkedConversations.length > 0 ? (
              <div className="app-iteration-linked-section">
                <div className="app-iteration-linked-label">
                  <span className="app-iteration-linked-label-icon">🔗</span>
                  <span>Linked conversations</span>
                </div>
                <div className="app-iteration-linked-list">
                  {linkedConversations.map((step, idx) => {
                    const id = linkedConversationId(step);
                    return (
                      <div
                        key={`${id}:${idx}`}
                        className="app-iteration-linked-conv-item"
                        onClick={() => openLinkedConversation(step)}
                        role="button"
                        tabIndex={0}
                        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') openLinkedConversation(step); }}
                        title={`Open linked conversation: ${id}`}
                      >
                        <div className="app-iteration-linked-conv-icon">{displayLinkedConversationIcon()}</div>
                        <div className="app-iteration-linked-conv-info">
                          <div className="app-iteration-linked-conv-head">
                            <span className="app-iteration-linked-conv-title">{displayLinkedConversationTitle()}</span>
                            <div className="app-iteration-linked-conv-meta">
                              <span className={`app-iteration-status tone-${statusTone(step?.status)}`}>{statusLabel(step?.status)}</span>
                              {latencyLabel(step) ? <span className="app-iteration-linked-conv-time">{latencyLabel(step)}</span> : null}
                            </div>
                          </div>
                        </div>
                        <span className="app-iteration-linked-conv-arrow">→</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : null}
          </>
        ) : null}
      </section>
      {!hasVisibleElicitation && shouldShowPreambleBubble(visibleGroups, visibleRenderedText) ? (
        <BubbleMessage
          message={{
            id: `${message?.id || 'iteration'}:preamble`,
            role: 'assistant',
            content: visibleRenderedText
          }}
        />
      ) : null}
      {hasVisibleElicitation ? (
        <Dialog
          isOpen={isElicitationOpen}
          onClose={() => setIsElicitationOpen(false)}
          title="Provide Details"
          style={{ width: '70vw', maxWidth: '1000px' }}
        >
          <div style={{ padding: 20 }}>
            <ElicitationForm
              message={data?.response}
              context={context}
              onResolved={() => setIsElicitationOpen(false)}
            />
          </div>
        </Dialog>
      ) : null}
    </>
  );
}
