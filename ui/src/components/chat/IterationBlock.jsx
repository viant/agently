import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Dialog } from '@blueprintjs/core';
import { summarizeLinkedConversationTranscript as sdkSummarizeLinkedConversationTranscript } from 'agently-core-ui-sdk';
import { DetailContext } from '../../context/DetailContext';
import { openLinkedConversationWindow } from '../../services/conversationWindow';
import { client } from '../../services/agentlyClient';
import { fetchTranscript } from '../../services/chatRuntime';
import { displayStepIcon, displayStepTitle, isAgentRunTool, humanizeAgentId } from '../../services/toolPresentation';
import BubbleMessage from './BubbleMessage';
import ElicitationForm from './ElicitationForm';
import RichContent from './RichContent';
import ToolFeedDetail from '../ToolFeedDetail';

const GROUPS_VISIBLE = 'all';
const TOOLS_PER_GROUP = 3;
const GROUP_PAGE_SIZES = [1, 5, 10, 'all'];

function statusLabel(value) {
  const text = String(value || '').trim();
  if (!text) return 'running';
  if (text.toLowerCase() === 'cancelled') return 'canceled';
  return text.replace(/_/g, ' ');
}

export function isActiveStatus(value) {
  const text = String(value || '').trim().toLowerCase();
  return text === ''
    || text === 'running'
    || text === 'thinking'
    || text === 'processing'
    || text === 'in_progress'
    || text === 'queued'
    || text === 'streaming'
    || text === 'tool_calls'
    || text === 'pending'
    || text === 'open';
}

export function statusTone(value) {
  const text = String(value || '').trim().toLowerCase();
  if (!text
    || text === 'running'
    || text === 'thinking'
    || text === 'processing'
    || text === 'in_progress'
    || text === 'streaming'
    || text === 'tool_calls'
    || text === 'pending'
    || text === 'queued'
    || text === 'open') {
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

function linkedConversationLabel(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  return agentLabel(humanizeAgentId(text));
}

export function displayLinkedConversationTitle(step = {}, context = null) {
  const explicitTitle = String(step?.title || step?.Title || '').trim();
  if (explicitTitle) return explicitTitle;
  const explicitAgent = String(step?.agentName || step?.agentId || step?.AgentId || '').trim();
  if (explicitAgent) return linkedConversationLabel(explicitAgent);
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const metaForm = metaDS?.peekFormData?.() || {};
  const byKey = metaForm?.agentInfo?.[explicitAgent] || null;
  const keyedName = String(byKey?.label || byKey?.name || byKey?.title || '').trim();
  if (keyedName) return keyedName;
  return 'Linked conversation';
}

export function displayLinkedConversationSubtitle(step = {}) {
  const response = String(step?.response || step?.Response || '').trim();
  if (response) return response;
  const explicitId = String(step?.linkedConversationId || step?.conversationId || step?.ConversationId || '').trim();
  return explicitId;
}

function previewGroupContent(entry = {}) {
  return String(entry?.content || '').trim();
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

function latencyLabel(step, now = Date.now(), fallbackStartedAt = 0) {
  const explicitMs = totalLatencyMs([step]);
  if (explicitMs >= 1000) {
    const explicit = formatDurationClock(explicitMs);
    if (explicit) return explicit;
  }
  if (!isActiveStatus(step?.status)) return '';
  const startedAt = earliestStartedAt([step]) || fallbackStartedAt;
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
      const startedAt = parseTimestamp(item?.startedAt || item?.StartedAt);
      const completedAt = parseTimestamp(item?.completedAt || item?.CompletedAt);
      if (startedAt && completedAt && completedAt >= startedAt) {
        return completedAt - startedAt;
      }
      return 0;
    })
    .filter((value) => Number.isFinite(value) && value > 0)
    .reduce((sum, value) => sum + value, 0);
}

function parseTimestamp(raw) {
  if (!raw) return 0;
  let str = String(raw).trim();
  if (!str) return 0;
  let v = Date.parse(str);
  if (Number.isFinite(v)) return v;
  // Go time.Time may include nanoseconds (e.g. ".123456789Z") — trim to 3.
  v = Date.parse(str.replace(/(\.\d{3})\d+/, '$1'));
  if (Number.isFinite(v)) return v;
  return 0;
}

function earliestStartedAt(steps = []) {
  let earliest = 0;
  for (const item of Array.isArray(steps) ? steps : []) {
    const value = parseTimestamp(item?.startedAt || item?.StartedAt || item?.createdAt || item?.CreatedAt);
    if (!value) continue;
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

function extractAgentIdFromPayload(payload = null) {
  const parsed = parseMaybeJSON(payload);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return '';
  return String(
    parsed?.metadata?.agentId
    || parsed?.metadata?.agentID
    || parsed?.agentId
    || parsed?.agentID
    || parsed?.AgentID
    || ''
  ).trim();
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
  return String(name || '').trim().toLowerCase().replace(/[\/:\-_]/g, '');
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
  const explicitAgentName = String(data?.agentName || data?.AgentName || '').trim();
  if (explicitAgentName) return explicitAgentName;
  const groupDerivedAgent = (() => {
    const groups = Array.isArray(data?.executionGroups) ? data.executionGroups : [];
    for (const group of groups) {
      const modelStep = group?.modelStep
        || (Array.isArray(group?.modelSteps) ? group.modelSteps[0] : null)
        || group?.modelCall
        || null;
      const fromModel = extractAgentIdFromPayload(modelStep?.requestPayload)
        || extractAgentIdFromPayload(modelStep?.providerRequestPayload);
      if (fromModel) return fromModel;
      const toolSteps = Array.isArray(group?.toolSteps) ? group.toolSteps : [];
      for (const step of toolSteps) {
        const fromTool = extractAgentIdFromPayload(step?.requestPayload)
          || extractAgentIdFromPayload(step?.providerRequestPayload);
        if (fromTool) return fromTool;
      }
    }
    return '';
  })();
  const explicitAgentId = String(
    data?.agentIdUsed
    || data?.AgentIdUsed
    || data?.agentId
    || data?.AgentId
    || groupDerivedAgent
    || ''
  ).trim();
  const conversationsDS = context?.Context?.('conversations')?.handlers?.dataSource;
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const convForm = conversationsDS?.peekFormData?.() || {};
  const metaForm = metaDS?.peekFormData?.() || {};
  const fallbackAgentId = String(
    convForm?.agent
    || metaForm?.agent
    || metaForm?.defaults?.agent
    || ''
  ).trim();
  const resolvedAgentId = explicitAgentId || fallbackAgentId;
  if (!resolvedAgentId) return '';
  if (String(resolvedAgentId).trim().toLowerCase() === 'auto') {
    return 'Auto-select agent';
  }

  const byKey = metaForm?.agentInfo?.[resolvedAgentId] || null;
  const keyedName = String(byKey?.label || byKey?.name || byKey?.title || '').trim();
  if (keyedName) return keyedName;

  const optionLists = [
    ...(Array.isArray(metaForm?.agentOptions) ? metaForm.agentOptions : []),
    ...(Array.isArray(metaForm?.agentInfos) ? metaForm.agentInfos : [])
  ];
  const matched = optionLists.find((entry) => matchesAgentEntry(entry, resolvedAgentId)) || null;
  const matchedName = String(matched?.label || matched?.name || matched?.title || '').trim();
  if (matchedName) return matchedName;
  return agentLabel(humanizeAgentId(resolvedAgentId));
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
    const rawPreambleContent = String(group?.preamble || group?.Preamble || '').trim();
    const preambleContent = rawPreambleContent;
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
  }).filter((group) => {
    const hasModel = !!group?.modelStep;
    const hasTools = Array.isArray(group?.toolSteps) && group.toolSteps.length > 0;
    const hasPreamble = String(group?.preambleContent || '').trim() !== '';
    const hasFinal = !!group?.finalResponse || String(group?.finalContent || '').trim() !== '';
    return hasModel || hasTools || hasPreamble || hasFinal;
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
    const hasTools = Array.isArray(group?.toolSteps) && group.toolSteps.length > 0;
    const derivedTitle = String(group?.title || '').trim();
    if (hasTools && derivedTitle) {
      return derivedTitle;
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
  const finalVisibleBubble = String(resolveVisibleBubbleContent(visibleGroups) || '').trim();
  const hasFinalVisibleGroup = (Array.isArray(visibleGroups) ? visibleGroups : []).some((group) => {
    const finalText = String(group?.finalContent || '').trim();
    return !!group?.finalResponse && finalText !== '';
  });
  return String(
    (hasFinalVisibleGroup ? finalVisibleBubble : '')
    || streamContent
    || finalVisibleBubble
    || iterationContent
    || responseContent
    || preambleContent
    || ''
  ).trim();
}

export function shouldShowPreambleBubble(visibleGroups = [], visibleText = '') {
  return String(visibleText || '').trim() !== '';
}

function summaryModeMessageContent(value = {}) {
  return String(value?.content || '').trim();
}

function linkedConversationId(step = {}) {
  return String(step?.linkedConversationId || step?.LinkedConversationId || '').trim();
}

function linkedConversationStartedAt(step = {}) {
  return parseTimestamp(step?.startedAt || step?.StartedAt || step?.createdAt || step?.CreatedAt || '');
}

export function linkedConversationLatencyLabel(step = {}, now = Date.now(), fallbackStartedAt = 0) {
  return latencyLabel(step, now, linkedConversationStartedAt(step) || fallbackStartedAt);
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

function iterationDebugEnabled() {
  if (typeof window === 'undefined') return false;
  try {
    const raw = String(window.localStorage?.getItem('agently.debugIterationCollapse') || '').trim().toLowerCase();
    return raw === '1' || raw === 'true' || raw === 'on';
  } catch (_) {
    return false;
  }
}

function logIterationDebug(event, detail = {}) {
  if (!iterationDebugEnabled()) return;
  try {
    // eslint-disable-next-line no-console
    console.log('[iteration-collapse]', {
      event,
      ts: new Date().toISOString(),
      ...detail
    });
  } catch (_) {}
}

function isLinkedConversationActive(status = '') {
  const text = String(status || '').trim().toLowerCase();
  return text === '' || text === 'running' || text === 'thinking' || text === 'processing' || text === 'queued' || text === 'pending' || text === 'in_progress' || text === 'open';
}

function isErrorStatus(value = '') {
  const text = String(value || '').trim().toLowerCase();
  return text === 'failed' || text === 'error' || text === 'terminated';
}

export function resolveIterationDisplayStatus(data = {}, groups = [], linkedStatuses = []) {
  const groupList = Array.isArray(groups) ? groups : [];
  const linkedList = Array.isArray(linkedStatuses) ? linkedStatuses : [];
  const dataStatus = String(data?.status || '').trim();
  if (linkedList.some((status) => isLinkedConversationActive(status))) {
    return 'running';
  }
  if (groupList.some((group) => (
    isActiveStatus(group?.status)
    || isActiveStatus(group?.modelStep?.status)
    || (Array.isArray(group?.toolSteps) && group.toolSteps.some((step) => isActiveStatus(step?.status)))
  ))) {
    return 'running';
  }
  if (isErrorStatus(dataStatus)) {
    return dataStatus;
  }
  if (groupList.some((group) => (
    isErrorStatus(group?.status)
    || isErrorStatus(group?.modelStep?.status)
    || (Array.isArray(group?.toolSteps) && group.toolSteps.some((step) => isErrorStatus(step?.status)))
  )) || linkedList.some((status) => isErrorStatus(status))) {
    return 'failed';
  }
  if (String(dataStatus).trim()) {
    return dataStatus;
  }
  const completedGroup = groupList.some((group) => (
    !isActiveStatus(group?.status)
    && !isErrorStatus(group?.status)
    && (String(group?.status || '').trim() !== ''
      || String(group?.modelStep?.status || '').trim() !== ''
      || (Array.isArray(group?.toolSteps) && group.toolSteps.some((step) => String(step?.status || '').trim() !== '')))
  ));
  if (completedGroup || linkedList.some((status) => String(status || '').trim() !== '')) {
    return 'completed';
  }
  return '';
}

export function isIterationActive(data = {}, groups = [], linkedStatuses = []) {
  return isActiveStatus(resolveIterationDisplayStatus(data, groups, linkedStatuses));
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
  const [linkedConversationStates, setLinkedConversationStates] = useState([]);
  const [expandedLinkedIds, setExpandedLinkedIds] = useState({});
  const groupsRef = useRef(null);

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
  const linkedConversationIds = useMemo(() => {
    const ids = [];
    const seen = new Set();
    for (const group of allGroupEntries) {
      for (const step of Array.isArray(group?.toolSteps) ? group.toolSteps : []) {
        const id = linkedConversationId(step);
        if (!id || seen.has(id)) continue;
        seen.add(id);
        ids.push(id);
      }
    }
    return ids;
  }, [allGroupEntries]);

  const isActiveIteration = useMemo(
    () => isIterationActive(data, allGroupEntries, linkedConversationStates.map((entry) => entry?.status || '')),
    [allGroupEntries, data, linkedConversationStates]
  );
  const iterationDisplayStatus = useMemo(
    () => resolveIterationDisplayStatus(data, allGroupEntries, linkedConversationStates.map((entry) => entry?.status || '')),
    [allGroupEntries, data, linkedConversationStates]
  );
  const [collapsed, setCollapsed] = useState(() => !(isLatestIteration && isActiveIteration));
  const prevLatestRef = useRef(isLatestIteration);
  const prevActiveRef = useRef(isActiveIteration);

  // Shared turn start time — used by both the header timer and individual step timers.
  const turnStartedAt = useMemo(() => {
    const sourceSteps = allGroupEntries.flatMap((group) => [
      ...(group?.modelStep ? [group.modelStep] : []),
      ...(Array.isArray(group?.toolSteps) ? group.toolSteps : [])
    ]);
    const linkedStartedAt = earliestStartedAt(linkedConversationStates);
    return earliestStartedAt(sourceSteps)
      || linkedStartedAt
      || parseTimestamp(data?.startedAt || data?.StartedAt || '')
      || parseTimestamp(message?.createdAt)
      || 0;
  }, [allGroupEntries, linkedConversationStates, data?.startedAt, data?.StartedAt, message?.createdAt]);

  const elapsedLabel = useMemo(() => {
    const sourceSteps = allGroupEntries.flatMap((group) => [
      ...(group?.modelStep ? [group.modelStep] : []),
      ...(Array.isArray(group?.toolSteps) ? group.toolSteps : [])
    ]);
    const explicit = formatDurationClock(totalLatencyMs(sourceSteps));
    if (!isActiveIteration) return explicit;
    if (!turnStartedAt) return explicit;
    return formatDurationClock(Math.max(0, now - turnStartedAt)) || explicit;
  }, [allGroupEntries, isActiveIteration, turnStartedAt, now]);

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
    for (const state of linkedConversationStates) {
      const id = String(state?.conversationId || '').trim();
      if (!id || !seen.has(id)) continue;
      seen.set(id, {
        ...seen.get(id),
        status: state?.status || seen.get(id)?.status || '',
        createdAt: state?.createdAt || seen.get(id)?.createdAt || '',
        updatedAt: state?.updatedAt || seen.get(id)?.updatedAt || '',
        agentId: state?.agentId || seen.get(id)?.agentId || '',
        title: state?.title || seen.get(id)?.title || '',
        response: state?.response || seen.get(id)?.response || '',
        previewGroups: Array.isArray(state?.previewGroups) && state.previewGroups.length > 0
          ? state.previewGroups
          : (Array.isArray(seen.get(id)?.previewGroups) ? seen.get(id).previewGroups : [])
      });
    }
    return [...seen.values()];
  }, [allGroupEntries, linkedConversationStates]);
  const hasActiveLinkedConversation = useMemo(
    () => linkedConversationStates.some((entry) => isLinkedConversationActive(entry?.status)),
    [linkedConversationStates]
  );

  const iterationAgentLabel = useMemo(
    () => resolveIterationAgentLabel(data, context),
    [context, data]
  );
  const iterationStatusDetail = useMemo(
    () => resolveIterationStatusDetail({ ...data, status: iterationDisplayStatus || data?.status || '' }),
    [data, iterationDisplayStatus]
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
  const summaryContent = String(summaryModeMessageContent(data?.summary)).trim();
  const generatedFiles = Array.isArray(message?.generatedFiles) ? message.generatedFiles : [];

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
            {latencyLabel(modelStep, now, turnStartedAt) ? (
              <span className="app-iteration-group-time">{latencyLabel(modelStep, now, turnStartedAt)}</span>
            ) : null}
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
                  {latencyLabel(toolStep, now, turnStartedAt) ? (
                    <span className="app-iteration-group-time">{latencyLabel(toolStep, now, turnStartedAt)}</span>
                  ) : null}
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

  const renderLinkedPreviewGroup = (preview, previewIndex) => {
    const modelStep = preview?.modelStep || null;
    const toolSteps = Array.isArray(preview?.toolSteps) ? preview.toolSteps : [];
    return (
      <div className="app-iteration-group app-iteration-linked-preview-group" key={`${String(preview?.id || previewIndex)}:group`}>
        {modelStep ? (
          <div className="app-iteration-model-row">
            <div className="app-iteration-tool-row-main">
              <span className="app-iteration-model-icon">{displayStepIcon({ ...modelStep, kind: 'model' })}</span>
              <span className="app-iteration-model-title">{stepTitle({ ...modelStep, kind: 'model' })}</span>
              {preview?.title && preview.title !== stepTitle({ ...modelStep, kind: 'model' }) ? (
                <span className="app-iteration-model-summary">{preview.title}</span>
              ) : null}
            </div>
            <div className="app-iteration-tool-row-meta">
              <span className={`app-iteration-status tone-${statusTone(modelStep?.status)}`}>{statusLabel(modelStep?.status)}</span>
              <Button
                minimal
                small
                className="app-iteration-link"
                onClick={() => showDetail?.(modelStep)}
              >
                Details
              </Button>
            </div>
          </div>
        ) : null}
        {toolSteps.length > 0 ? (
          <div className="app-iteration-tool-list">
            {toolSteps.map((toolStep, toolIndex) => (
              <div className="app-iteration-tool-row" key={`${String(preview?.id || previewIndex)}:tool:${toolIndex}`}>
                <div className="app-iteration-tool-row-main">
                  <span className="app-iteration-tool-icon">{displayItemRowIcon(toolStep)}</span>
                  <span className="app-iteration-tool-row-title">{displayItemRowTitle(toolStep)}</span>
                </div>
                <div className="app-iteration-tool-row-meta">
                  <span className={`app-iteration-status tone-${statusTone(toolStep?.status)}`}>{statusLabel(toolStep?.status)}</span>
                  <Button
                    minimal
                    small
                    className="app-iteration-link"
                    onClick={() => showDetail?.(toolStep)}
                  >
                    Details
                  </Button>
                </div>
              </div>
            ))}
          </div>
        ) : null}
        {!modelStep && toolSteps.length === 0 ? (
          <div className="app-iteration-linked-conv-expanded-head">
            <span className="app-iteration-linked-conv-expanded-title-wrap">
              <span className="app-iteration-linked-conv-expanded-icon">{displayStepIcon({ kind: preview?.stepKind })}</span>
              <span className="app-iteration-linked-conv-preview-title">{String(preview?.title || '').trim()}</span>
            </span>
            <span className={`app-iteration-status tone-${statusTone(preview?.status)}`}>{statusLabel(preview?.status)}</span>
          </div>
        ) : null}
        {previewGroupContent(preview) ? (
          <div className="app-iteration-linked-conv-expanded-body">{previewGroupContent(preview)}</div>
        ) : null}
      </div>
    );
  };

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
    const parentConversationID = currentConversationId();
    const parentTurnID = String(data?.turnId || '').trim();
    if (!parentConversationID || !parentTurnID || linkedConversationIds.length === 0) {
      setLinkedConversationStates([]);
      return undefined;
    }
    let cancelled = false;
    let timer = null;
    const loadLinkedStatuses = async () => {
      try {
        const page = await client.listLinkedConversations({
          parentConversationId: parentConversationID,
          parentTurnId: parentTurnID
        });
        if (cancelled) return;
        const baseRows = Array.isArray(page?.data) ? page.data : [];
        const rows = await Promise.all(baseRows.map(async (entry) => {
          const conversationId = String(entry?.conversationId || '').trim();
          if (!conversationId) return entry;
          const next = { ...entry };
          try {
            const [conversation, transcript] = await Promise.all([
              String(next?.title || '').trim() ? Promise.resolve(null) : client.getConversation(conversationId).catch(() => null),
              client.getTranscript({
                conversationId,
                includeModelCalls: true,
                includeToolCalls: true
              }, {
                executionGroupLimit: 1,
                executionGroupOffset: 0
              }).catch(() => null)
            ]);
            if (conversation && !String(next?.title || '').trim()) {
              next.title = String(conversation?.title || conversation?.Title || '').trim();
            }
            if (conversation && !String(next?.agentId || '').trim()) {
              next.agentId = String(conversation?.agentId || conversation?.AgentId || '').trim();
            }
            if (transcript) {
              const preview = sdkSummarizeLinkedConversationTranscript(transcript);
              next.status = String(preview.status || next.status || '').trim();
              next.response = String(preview.response || next.response || '').trim();
              next.updatedAt = String(preview.updatedAt || next.updatedAt || '').trim();
              next.agentId = String(preview.agentId || next.agentId || '').trim();
              next.previewGroups = Array.isArray(preview.previewGroups) ? preview.previewGroups : [];
            }
          } catch (_) {}
          return next;
        }));
        if (cancelled) return;
        setLinkedConversationStates(rows);
        logIterationDebug('linked-status', {
          turnId: parentTurnID,
          messageId: message?.id || '',
          linked: rows.map((entry) => ({
            id: entry?.conversationId || '',
            status: entry?.status || ''
          }))
        });
      } catch (err) {
        if (cancelled) return;
        logIterationDebug('linked-status-error', {
          turnId: parentTurnID,
          messageId: message?.id || '',
          error: String(err?.message || err || '')
        });
      }
      if (!cancelled && isLatestIteration && isActiveIteration) {
        timer = window.setTimeout(loadLinkedStatuses, 1500);
      }
    };
    void loadLinkedStatuses();
    return () => {
      cancelled = true;
      if (timer) window.clearTimeout(timer);
    };
  }, [data?.turnId, isActiveIteration, isLatestIteration, linkedConversationIds, message?.id]);

  useEffect(() => {
    const turnTerminal = isTerminalTurnStatus(iterationDisplayStatus);
    const linkedActive = hasActiveLinkedConversation;
    if (isLatestIteration && isActiveIteration) {
      logIterationDebug('expand-latest-active', {
        messageId: message?.id || '',
        turnId: data?.turnId || '',
        turnStatus: iterationDisplayStatus || '',
        isLatestIteration,
        isActiveIteration,
        linkedActive,
        linkedConversationStates
      });
      setCollapsed(false);
    } else if (turnTerminal) {
      logIterationDebug('terminal-no-autocollapse', {
        messageId: message?.id || '',
        turnId: data?.turnId || '',
        turnStatus: iterationDisplayStatus || '',
        isLatestIteration,
        isActiveIteration,
        linkedActive,
        linkedConversationStates
      });
    }
    prevLatestRef.current = isLatestIteration;
    prevActiveRef.current = isActiveIteration;
  }, [iterationDisplayStatus, isActiveIteration, isLatestIteration, linkedConversationStates, hasActiveLinkedConversation, message?.id, data?.turnId]);

  useEffect(() => {
    if (!hasActiveLinkedConversation) return;
    logIterationDebug('expand-linked-active', {
      messageId: message?.id || '',
      turnId: data?.turnId || '',
      turnStatus: iterationDisplayStatus || '',
      isLatestIteration,
      isActiveIteration,
      linkedConversationStates
    });
    setCollapsed(false);
  }, [hasActiveLinkedConversation, message?.id, data?.turnId, iterationDisplayStatus, isLatestIteration, isActiveIteration, linkedConversationStates]);

  useEffect(() => {
    const activeIds = linkedConversations
      .filter((entry) => isLinkedConversationActive(entry?.status))
      .map((entry) => linkedConversationId(entry))
      .filter(Boolean);
    if (activeIds.length === 0) return;
    setExpandedLinkedIds((current) => {
      const next = { ...current };
      let changed = false;
      for (const id of activeIds) {
        if (!next[id]) {
          next[id] = true;
          changed = true;
        }
      }
      return changed ? next : current;
    });
  }, [linkedConversations]);

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
    setExpandedLinkedIds({});
  }, [message?.id, data?.turnId]);

  useEffect(() => {
    if (hasVisibleElicitation && (elicitationStatus === '' || elicitationStatus === 'pending' || elicitationStatus === 'open')) {
      setIsElicitationOpen(true);
    }
  }, [hasVisibleElicitation, elicitationStatus, message?.id]);

  useEffect(() => {
    if (!isActiveIteration || collapsed) return;
    const el = groupsRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [collapsed, isActiveIteration, visibleGroups]);

  return (
    <>
      <section className={`app-iteration-card tone-${statusTone(iterationDisplayStatus)}`}>
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
            <div className="app-iteration-groups" ref={groupsRef}>
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
                    const previewExpanded = !!expandedLinkedIds[id];
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
                            <span className="app-iteration-linked-conv-title">{displayLinkedConversationTitle(step, context)}</span>
                            <div className="app-iteration-linked-conv-meta">
                              <span className={`app-iteration-status tone-${statusTone(step?.status)}`}>{statusLabel(step?.status)}</span>
                              {linkedConversationLatencyLabel(step, now, turnStartedAt) ? <span className="app-iteration-linked-conv-time">{linkedConversationLatencyLabel(step, now, turnStartedAt)}</span> : null}
                            </div>
                          </div>
                          {displayLinkedConversationSubtitle(step) ? (
                            <div className="app-iteration-linked-conv-subtitle">{displayLinkedConversationSubtitle(step)}</div>
                          ) : null}
                          {Array.isArray(step?.previewGroups) && step.previewGroups.length > 0 ? (
                            <>
                              <div className="app-iteration-linked-conv-preview">
                                {step.previewGroups.map((preview, previewIndex) => (
                                  <div key={`${String(preview?.id || previewIndex)}:${previewIndex}`} className="app-iteration-linked-conv-preview-row">
                                    <span className="app-iteration-linked-conv-preview-title">{String(preview?.title || '').trim()}</span>
                                    <span className={`app-iteration-status tone-${statusTone(preview?.status)}`}>{statusLabel(preview?.status)}</span>
                                  </div>
                                ))}
                              </div>
                              <button
                                type="button"
                                className="app-iteration-linked-conv-toggle"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  setExpandedLinkedIds((current) => ({ ...current, [id]: !current[id] }));
                                }}
                              >
                                {previewExpanded ? 'Hide details' : 'Show details'}
                              </button>
                              {previewExpanded ? (
                                <div className="app-iteration-linked-conv-expanded">
                                  {step.previewGroups.map((preview, previewIndex) => renderLinkedPreviewGroup(preview, previewIndex))}
                                </div>
                              ) : null}
                            </>
                          ) : null}
                        </div>
                        <span className="app-iteration-linked-conv-arrow">→</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            ) : null}
            {summaryContent ? (
              <details className="app-iteration-summary-section">
                <summary className="app-iteration-summary-head">Summary</summary>
                <div className="app-iteration-summary-body">
                  <div className="app-iteration-summary-content">
                    <RichContent content={summaryContent} generatedFiles={generatedFiles} />
                  </div>
                </div>
              </details>
            ) : null}
          </>
        ) : null}
      </section>
      <ToolFeedDetail context={context} />
      {!hasVisibleElicitation && shouldShowPreambleBubble(visibleGroups, visibleRenderedText) ? (
        <BubbleMessage
          message={{
            id: `${message?.id || 'iteration'}:preamble`,
            role: 'assistant',
            content: visibleRenderedText,
            generatedFiles
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
