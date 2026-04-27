import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@blueprintjs/core';
import { summarizeLinkedConversationTranscript as sdkSummarizeLinkedConversationTranscript } from 'agently-core-ui-sdk';
import { DetailContext } from '../../context/DetailContext';
import { ConversationViewContext } from '../../context/ConversationViewContext';
import { openLinkedConversationWindow } from '../../services/conversationWindow';
import { client } from '../../services/agentlyClient';
import { displayStepIcon, displayStepTitle, executionRoleLabel, isAgentRunTool, humanizeAgentId } from '../../services/toolPresentation';
import BubbleMessage from './BubbleMessage';
import RichContent from './RichContent';
import ToolFeedDetail from '../ToolFeedDetail';

function statusLabel(value) {
  const text = String(value || '').trim();
  if (!text) return 'running';
  if (text.toLowerCase() === 'started') return 'running';
  if (text.toLowerCase() === 'cancelled') return 'canceled';
  return text.replace(/_/g, ' ');
}

function normalizeElicitationStatus(value) {
  const text = String(value || '').trim().toLowerCase();
  if (!text) return '';
  if (text === 'cancelled') return 'canceled';
  return text;
}

function elicitationStepTitle(step = {}) {
  const message = String(step?.message || '').trim();
  if (message) return message.slice(0, 60);
  return String(step?.toolName || 'Needs input').trim().slice(0, 60);
}

function elicitationStepLabel(status = '') {
  const normalized = normalizeElicitationStatus(status);
  if (normalized === 'accepted' || normalized === 'submitted' || normalized === 'completed' || normalized === 'succeeded') {
    return 'Input provided';
  }
  if (normalized === 'declined') return 'Input declined';
  if (normalized === 'canceled') return 'Input canceled';
  return 'Needs input';
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
  if (text === 'completed' || text === 'succeeded' || text === 'success' || text === 'accepted' || text === 'submitted') {
    return 'success';
  }
  if (text === 'failed' || text === 'error' || text === 'cancelled' || text === 'canceled' || text === 'terminated' || text === 'declined') {
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
  const kind = String(step?.kind || '').toLowerCase();
  if (kind === 'elicitation') {
    return elicitationStepTitle(step);
  }
  if (kind === 'turn') {
    return displayStepTitle(step);
  }
  return rawToolTitle(step);
}

export function displayItemRowIcon(step = {}) {
  const kind = String(step?.kind || '').toLowerCase();
  if (kind === 'elicitation') return '⌨️';
  if (kind === 'turn') return displayStepIcon(step);
  return '🛠';
}

function linkedConversationLabel(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  return agentLabel(humanizeAgentId(text));
}

function linkedConversationAgent(step = {}) {
  return String(
    step?.agentName
    || step?.agentId
    || step?.AgentId
    || step?.linkedConversationAgentId
    || step?.LinkedConversationAgentId
    || ''
  ).trim();
}

export function displayLinkedConversationTitle(step = {}, context = null) {
  const explicitTitle = String(step?.title || step?.Title || step?.linkedConversationTitle || step?.LinkedConversationTitle || '').trim();
  if (explicitTitle) return explicitTitle;
  const explicitAgent = linkedConversationAgent(step);
  const metaDS = context?.Context?.('meta')?.handlers?.dataSource;
  const metaForm = metaDS?.peekFormData?.() || {};
  const byKey = metaForm?.agentInfo?.[explicitAgent] || null;
  const keyedName = String(byKey?.label || byKey?.name || byKey?.title || '').trim();
  if (keyedName) return keyedName;
  if (explicitAgent) return linkedConversationLabel(explicitAgent);
  return 'Linked conversation';
}

export function displayLinkedConversationSubtitle(step = {}) {
  const response = String(step?.response || step?.Response || '').trim();
  if (response) return response;
  const explicitAgent = linkedConversationAgent(step);
  if (explicitAgent) return linkedConversationLabel(explicitAgent);
  return '';
}

export function isQueuedLinkedConversationPreview(step = {}) {
  const status = String(step?.status || step?.Status || '').trim().toLowerCase();
  const title = String(step?.title || step?.Title || step?.linkedConversationTitle || step?.LinkedConversationTitle || '').trim().toLowerCase();
  return (status === 'queued' || status === 'pending' || status === 'open')
    && (title === 'next' || title === 'queued');
}

function previewGroupContent(entry = {}) {
  return String(entry?.content || '').trim();
}

export function displayLinkedConversationIcon() {
  return '🔗';
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

function latestStartedAt(steps = []) {
  let latest = 0;
  for (const item of Array.isArray(steps) ? steps : []) {
    const value = parseTimestamp(item?.startedAt || item?.StartedAt || item?.createdAt || item?.CreatedAt);
    if (!value) continue;
    if (latest === 0 || value > latest) {
      latest = value;
    }
  }
  return latest;
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

function delegatedAgentAssistantText(step = {}) {
  if (!isAgentRunTool(step)) return '';
  const payloads = [
    step?.responsePayload,
    step?.providerResponsePayload,
    parseMaybeJSON(step?.responsePayload),
    parseMaybeJSON(step?.providerResponsePayload)
  ];
  for (const payload of payloads) {
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) continue;
    const text = plainText(
      payload?.assistantResponse
      || payload?.assistant_response
      || payload?.response
      || payload?.content
      || ''
    );
    if (text) return text;
  }
  return '';
}

export function toolStepSummaryText(step = {}) {
  const explicit = truncate(step?.content || '', 120);
  if (explicit) return explicit;
  const payloads = [
    step?.responsePayload,
    step?.providerResponsePayload,
    parseMaybeJSON(step?.responsePayload),
    parseMaybeJSON(step?.providerResponsePayload)
  ];
  for (const payload of payloads) {
    if (!payload || typeof payload !== 'object' || Array.isArray(payload)) continue;
    const text = truncate(
      payload?.message
      || payload?.assistantResponse
      || payload?.assistant_response
      || payload?.response
      || payload?.content
      || '',
      120
    );
    if (text) return text;
  }
  return '';
}

function groupTitleFromSteps({ narration, modelStep, toolSteps = [] } = {}) {
  const explicit = truncate(narration?.content || '', 80);
  if (explicit) return explicit;
  const delegatedAssistantText = [...(Array.isArray(toolSteps) ? toolSteps : [])]
    .reverse()
    .map((step) => delegatedAgentAssistantText(step))
    .find(Boolean);
  if (delegatedAssistantText) {
    return truncate(delegatedAssistantText, 80);
  }
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
      id: modelStep0?.modelCallId || modelStep0?.assistantMessageId || group?.modelMessageId || group?.ModelMessageID || group?.parentMessageId || group?.ParentMessageID || `model:${index}`,
      modelCallId: modelStep0?.modelCallId || '',
      assistantMessageId: modelStep0?.assistantMessageId || group?.assistantMessageId || group?.pageId || '',
      kind: 'model',
      reason: group?.finalResponse || group?.FinalResponse ? 'final_response' : 'thinking',
      executionRole:
        modelStep0?.executionRole
        || modelStep0?.ExecutionRole
        || group?.executionRole
        || group?.ExecutionRole
        || (String(modelStep0?.phase || modelStep0?.Phase || group?.phase || group?.Phase || '').trim().toLowerCase() === 'intake' ? 'intake' : ''),
      phase: modelStep0?.phase || modelStep0?.Phase || group?.phase || group?.Phase || '',
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
      const stepKind = String(ts?.kind || '').trim().toLowerCase() || 'tool';
      const stepReason = String(ts?.reason || '').trim() || (stepKind === 'turn' ? String(ts?.toolName || '').trim() : 'tool_call');
      return {
        id: ts?.toolCallId || ts?.ToolCallID || ts?.opId || ts?.OpId || messageID || `tool:${index}:${toolIndex}`,
        toolCallId: ts?.toolCallId || ts?.ToolCallID || ts?.opId || ts?.OpId || '',
        kind: stepKind,
        reason: stepReason,
        executionRole: ts?.executionRole || ts?.ExecutionRole || group?.executionRole || group?.ExecutionRole || '',
        toolName: ts?.toolName || ts?.ToolName || toolMessage?.toolName || toolMessage?.ToolName || 'tool',
        content: ts?.content || ts?.Content || toolMessage?.content || toolMessage?.Content || '',
        status: ts?.status || ts?.Status || '',
        latencyMs: ts?.latencyMs || ts?.LatencyMs || null,
        startedAt: ts?.startedAt || ts?.StartedAt || '',
        completedAt: ts?.completedAt || ts?.CompletedAt || '',
        requestPayloadId: ts?.requestPayloadId || ts?.RequestPayloadId || '',
        responsePayloadId: ts?.responsePayloadId || ts?.ResponsePayloadId || '',
        requestPayload: ts?.requestPayload || ts?.RequestPayload || null,
        responsePayload: ts?.responsePayload || ts?.ResponsePayload || null,
        linkedConversationId: ts?.linkedConversationId || ts?.LinkedConversationId || toolMessage?.linkedConversationId || toolMessage?.LinkedConversationId || '',
        linkedConversationAgentId: ts?.linkedConversationAgentId || ts?.LinkedConversationAgentId || toolMessage?.linkedConversationAgentId || toolMessage?.LinkedConversationAgentId || '',
        linkedConversationTitle: ts?.linkedConversationTitle || ts?.LinkedConversationTitle || toolMessage?.linkedConversationTitle || toolMessage?.LinkedConversationTitle || ''
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
    const lifecycleOnly = !modelStep0
      && toolSteps.length > 0
      && toolSteps.every((step) => String(step?.kind || '').trim().toLowerCase() === 'turn');
    const groupPhase = String(
      group?.phase
      || group?.Phase
      || modelStep?.phase
      || ''
    ).trim().toLowerCase();
    const rawPreambleContent = String(group?.narration || group?.Narration || '').trim();
    const narrationContent = rawPreambleContent;
    const status = String(group?.status || group?.Status || modelStep?.status || '').trim();
    const errorMessage = String(group?.errorMessage || group?.ErrorMessage || modelStep0?.errorMessage || modelStep0?.ErrorMessage || '').trim();
    const modelStepWithError = modelStep ? {
      ...modelStep,
      errorMessage: errorMessage || modelStep?.errorMessage || ''
    } : null;
    const effectiveModelStep = modelStepWithError || (errorMessage ? {
      id: String(group?.pageId || group?.assistantMessageId || group?.parentMessageId || group?.ParentMessageID || `failed-model:${index}`),
      kind: 'model',
      reason: 'thinking',
      provider: '',
      model: resolveIterationModelLabel(null) || 'model',
      status,
      errorMessage
    } : null);
    const finalContent = String(group?.content || group?.Content || '').trim();
    const title = groupTitleFromSteps({
      narration: narrationContent ? { content: narrationContent } : null,
      modelStep: effectiveModelStep,
      toolSteps
    });
    const lifecycleTitle = lifecycleOnly
      ? String(displayStepTitle(toolSteps[0] || {})).trim()
      : '';
    return {
      id: String(group?.parentMessageId || group?.ParentMessageID || group?.modelMessageId || group?.ModelMessageID || `group:${index}`),
      groupKind: lifecycleOnly
        ? 'lifecycle'
        : (groupPhase === 'intake'
          ? 'intake'
          : (groupPhase === 'sidecar'
            ? 'sidecar'
            : (groupPhase === 'summary'
              ? 'summary'
              : (effectiveModelStep ? 'model' : 'tool')))),
      title: lifecycleTitle || title,
      fullTitle: plainText(narrationContent || lifecycleTitle || title),
      narrationContent,
      modelStep: effectiveModelStep,
      toolSteps,
      detailStep: effectiveModelStep || toolSteps[0] || null,
      status,
      errorMessage,
      finalResponse: Boolean(group?.finalResponse || group?.FinalResponse),
      finalContent,
      elapsed: aggregateLatencyLabel([...(effectiveModelStep ? [effectiveModelStep] : []), ...toolSteps]),
      stepCount: (effectiveModelStep ? 1 : 0) + toolSteps.length
    };
  }).filter((group) => {
    const hasModel = !!group?.modelStep;
    const hasTools = Array.isArray(group?.toolSteps) && group.toolSteps.length > 0;
    const hasPreamble = String(group?.narrationContent || '').trim() !== '';
    const hasFinal = !!group?.finalResponse || String(group?.finalContent || '').trim() !== '';
    const hasError = String(group?.errorMessage || '').trim() !== '';
    return hasModel || hasTools || hasPreamble || hasFinal || hasError;
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
      narration: data?.narration || null,
      modelStep: primaryModel,
      toolSteps
    }),
    fullTitle: '',
    narrationContent: String(data?.narration?.content || '').trim(),
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
      const embedded = extractEmbeddedElicitationPayload(finalText);
      if (embedded?.message) return String(embedded.message).trim();
      if (!looksLikeStructuredJSON(finalText)) return finalText;
    }
  }
  for (let index = groups.length - 1; index >= 0; index -= 1) {
    const group = groups[index] || {};
    const preambleText = String(group?.narrationContent || '').trim();
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

function looksLikeStructuredJSON(text = '') {
  const raw = String(text || '').trim();
  if (!raw) return false;
  if (!(raw.startsWith('{') || raw.startsWith('['))) return false;
  try {
    const parsed = JSON.parse(raw);
    return !!parsed && typeof parsed === 'object';
  } catch (_) {
    return false;
  }
}

function isStructuredAssistantArtifact(text = '') {
  const raw = String(text || '').trim();
  if (!raw) return false;
  if (raw.startsWith('<!-- CHART_SPEC:')) return true;
  if (/^```(?:json)?\s*/i.test(raw)) return true;
  return looksLikeStructuredJSON(raw);
}

function extractEmbeddedElicitationPayload(text = '') {
  const raw = String(text || '').trim();
  if (!raw) return null;
  const start = raw.indexOf('{');
  const end = raw.lastIndexOf('}');
  if (start === -1 || end <= start) return null;
  try {
    const parsed = JSON.parse(raw.slice(start, end + 1));
    if (!parsed || typeof parsed !== 'object') return null;
    if (String(parsed.type || '').trim().toLowerCase() !== 'elicitation') return null;
    return {
      message: String(parsed.message || parsed.prompt || '').trim(),
      requestedSchema: parsed.requestedSchema || null,
    };
  } catch (_) {
    return null;
  }
}

function resolveFailedBubbleContent(visibleGroups = [], fallbackError = '') {
  const explicit = String(fallbackError || '').trim();
  if (explicit) return 'We experienced an error while processing this request.';
  const groups = Array.isArray(visibleGroups) ? visibleGroups : [];
  const hasError = groups.some((group) => {
    const status = String(group?.status || group?.modelStep?.status || '').trim().toLowerCase();
    return status === 'failed'
      || status === 'error'
      || status === 'terminated'
      || String(group?.errorMessage || group?.modelStep?.errorMessage || '').trim() !== '';
  });
  return hasError ? 'We experienced an error while processing this request.' : '';
}

export function resolveIterationBubbleContent({
  visibleGroups = [],
  iterationContent = '',
  responseContent = '',
  narrationContent = '',
  streamContent = '',
  errorMessage = ''
} = {}) {
  const groups = Array.isArray(visibleGroups) ? visibleGroups : [];
  const finalVisibleBubble = String(resolveVisibleBubbleContent(visibleGroups) || '').trim();
  const visibleStreamBubble = isStructuredAssistantArtifact(streamContent) ? '' : String(streamContent || '').trim();
  const explicitNarrationBubble = isStructuredAssistantArtifact(narrationContent) ? '' : String(narrationContent || '').trim();
  const hasFinalVisibleGroup = groups.some((group) => {
    const finalText = String(group?.finalContent || '').trim();
    return !!group?.finalResponse && finalText !== '';
  });
  if (groups.length > 0) {
    return String(
      (hasFinalVisibleGroup ? finalVisibleBubble : '')
      || (!hasFinalVisibleGroup ? visibleStreamBubble : '')
      || (!hasFinalVisibleGroup ? explicitNarrationBubble : '')
      || responseContent
      || finalVisibleBubble
      || resolveFailedBubbleContent(groups, errorMessage)
      || ''
    ).trim();
  }
  return String(
    (hasFinalVisibleGroup ? finalVisibleBubble : '')
    || streamContent
    || finalVisibleBubble
    || iterationContent
    || responseContent
    || narrationContent
    || resolveFailedBubbleContent(visibleGroups, errorMessage)
    || ''
  ).trim();
}

export function shouldShowNarrationBubble(visibleGroups = [], visibleText = '', responseContent = '') {
  const text = String(visibleText || '').trim();
  if (!text) return false;
  const groups = Array.isArray(visibleGroups) ? visibleGroups : [];
  if (groups.length === 0) return true;
  const hasFinalVisibleGroup = groups.some((group) => {
    const finalText = String(group?.finalContent || '').trim();
    return !!group?.finalResponse && finalText !== '';
  });
  if (hasFinalVisibleGroup) return true;
  return String(responseContent || '').trim() !== '';
}

export function hasPendingElicitationStep(visibleGroups = []) {
  return (Array.isArray(visibleGroups) ? visibleGroups : []).some((group) =>
    (Array.isArray(group?.toolSteps) ? group.toolSteps : []).some((step) => {
      if (String(step?.kind || '').trim().toLowerCase() !== 'elicitation') return false;
      const status = String(step?.status || '').trim().toLowerCase();
      return status === '' || status === 'pending' || status === 'open';
    })
  );
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
  const hasModel = !!group?.modelStep;
  const preambleText = String(group?.narrationContent || '').trim();
  const finalText = String(group?.finalContent || '').trim();
  const errorText = String(group?.errorMessage || group?.modelStep?.errorMessage || '').trim();
  const toolCount = Array.isArray(group?.toolSteps) ? group.toolSteps.length : 0;
  const plannedCount = Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned.length : 0;
  const groupKind = String(group?.groupKind || '').trim().toLowerCase();
  const isFinal = !!group?.modelStep && String(group?.modelStep?.reason || '').toLowerCase() === 'final_response';
  return isFinal
    || hasModel
    || groupKind === 'intake'
    || toolCount > 0
    || plannedCount > 0
    || preambleText !== ''
    || finalText !== ''
    || errorText !== '';
}

export function buildSyntheticModelGroup({ data = {}, message = {}, context = null, visibleText = '' } = {}) {
  const text = String(visibleText || '').trim();
  const errorText = String(data?.errorMessage || message?.errorMessage || '').trim();
  if (!text && !errorText && !isActiveStatus(data?.status || message?.status)) return null;
  const modelLabel = resolveIterationModelLabel(context);
  const status = String(data?.status || message?.turnStatus || message?.status || (text ? 'completed' : 'running')).trim();
  const isFailed = isErrorStatus(status);
  const finalResponse = text !== '' && !isActiveStatus(status) && !isFailed;
  const content = finalResponse ? text : '';
  const narrationContent = finalResponse ? '' : text;
  return {
    id: `synthetic:${String(message?.id || data?.turnId || 'iteration').trim() || 'iteration'}`,
    title: modelLabel || 'model',
    fullTitle: modelLabel || 'model',
    narrationContent,
    modelStep: {
      id: `synthetic-model:${String(message?.id || data?.turnId || 'iteration').trim() || 'iteration'}`,
      kind: 'model',
      reason: finalResponse ? 'final_response' : 'thinking',
      provider: '',
      model: modelLabel || 'model',
      status,
      errorMessage: errorText
    },
    toolSteps: [],
    detailStep: {
      id: `synthetic-model:${String(message?.id || data?.turnId || 'iteration').trim() || 'iteration'}`,
      kind: 'model',
      reason: finalResponse ? 'final_response' : 'thinking',
      provider: '',
      model: modelLabel || 'model',
      status,
      errorMessage: errorText
    },
    status,
    errorMessage: errorText,
    finalResponse,
    finalContent: content,
    elapsed: '',
    stepCount: 1
  };
}

export function phaseBadgeLabel(group = {}) {
  const modelStep = group?.modelStep || null;
  if (modelStep) return executionRoleLabel({ ...group, ...modelStep, groupKind: group?.groupKind, mode: group?.mode });
  const firstToolStep = Array.isArray(group?.toolSteps) ? group.toolSteps[0] : null;
  if (firstToolStep) return executionRoleLabel({ ...group, ...firstToolStep, groupKind: group?.groupKind, mode: group?.mode });
  return executionRoleLabel(group);
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
  const normalizedDataStatus = dataStatus.toLowerCase();
  const lifecycleSteps = groupList
    .filter((group) => String(group?.groupKind || '').trim().toLowerCase() === 'lifecycle')
    .flatMap((group) => Array.isArray(group?.toolSteps) ? group.toolSteps : []);
  const terminalLifecycleStep = [...lifecycleSteps].reverse().find((step) => {
    const reason = String(step?.reason || step?.toolName || '').trim().toLowerCase();
    return reason === 'turn_completed'
      || reason === 'turn_failed'
      || reason === 'turn_canceled'
      || reason === 'turn_cancelled';
  });
  if (terminalLifecycleStep) {
    const reason = String(terminalLifecycleStep?.reason || terminalLifecycleStep?.toolName || '').trim().toLowerCase();
    if (reason === 'turn_failed') return 'failed';
    if (reason === 'turn_canceled' || reason === 'turn_cancelled') return 'canceled';
    return 'completed';
  }
  const hasStartedLifecycle = lifecycleSteps.some((step) => {
    const reason = String(step?.reason || step?.toolName || '').trim().toLowerCase();
    return reason === 'turn_started';
  });
  if (hasStartedLifecycle) {
    return 'running';
  }
  if (isTerminalTurnStatus(normalizedDataStatus)) {
    return normalizedDataStatus === 'success' || normalizedDataStatus === 'succeeded' || normalizedDataStatus === 'done'
      ? 'completed'
      : normalizedDataStatus === 'cancelled'
        ? 'canceled'
        : normalizedDataStatus;
  }
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
  if (isErrorStatus(normalizedDataStatus)) {
    return normalizedDataStatus;
  }
  if (groupList.some((group) => (
    isErrorStatus(group?.status)
    || isErrorStatus(group?.modelStep?.status)
    || (Array.isArray(group?.toolSteps) && group.toolSteps.some((step) => isErrorStatus(step?.status)))
  )) || linkedList.some((status) => isErrorStatus(status))) {
    return 'failed';
  }
  if (normalizedDataStatus) {
    return normalizedDataStatus;
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

export function resolveIterationElapsedAnchor(data = {}, groups = [], linkedConversationStates = [], message = {}, isActive = false) {
  const turnStartedAt = parseTimestamp(data?.turnStartedAt || '');
  const turnCompletedAt = parseTimestamp(data?.turnCompletedAt || '');
  const allSteps = (Array.isArray(groups) ? groups : []).flatMap((group) => [
    ...(group?.modelStep ? [group.modelStep] : []),
    ...(Array.isArray(group?.toolSteps) ? group.toolSteps : [])
  ]);
  const latestPreambleStartedAt = latestStartedAt([
    data?.narration,
    ...(Array.isArray(data?.narrations) ? data.narrations : []),
    data?.response
  ]);
  const streamStartedAt = parseTimestamp(data?.streamCreatedAt || '');
  if (!isActive) {
    return turnStartedAt
      || earliestStartedAt(allSteps)
      || earliestStartedAt(linkedConversationStates)
      || streamStartedAt
      || latestPreambleStartedAt
      || parseTimestamp(data?.startedAt || data?.StartedAt || '')
      || parseTimestamp(message?.createdAt)
      || 0;
  }
  const activeGroups = (Array.isArray(groups) ? groups : []).filter((group) => {
    if (isActiveStatus(group?.status)) return true;
    if (isActiveStatus(group?.modelStep?.status)) return true;
    return (Array.isArray(group?.toolSteps) ? group.toolSteps : []).some((step) => isActiveStatus(step?.status));
  });
  const activeSteps = activeGroups.flatMap((group) => [
    ...(group?.modelStep ? [group.modelStep] : []),
    ...(Array.isArray(group?.toolSteps) ? group.toolSteps : []).filter((step) => isActiveStatus(step?.status))
  ]);
  const activeLinked = (Array.isArray(linkedConversationStates) ? linkedConversationStates : []).filter((entry) => isLinkedConversationActive(entry?.status));
  return turnStartedAt
    || latestStartedAt(activeSteps)
    || latestStartedAt(activeLinked)
    || streamStartedAt
    || latestPreambleStartedAt
    || parseTimestamp(data?.startedAt || data?.StartedAt || '')
    || parseTimestamp(message?.createdAt)
    || earliestStartedAt(allSteps)
    || earliestStartedAt(linkedConversationStates)
    || 0;
}

export function shouldAutoScrollExecutionGroups({ collapsed = false, isActiveIteration = false, iterationDisplayStatus = '' } = {}) {
  if (collapsed) return false;
  if (!isActiveIteration) return false;
  if (isTerminalTurnStatus(iterationDisplayStatus)) return false;
  return true;
}

export function buildIterationDataFromCanonicalRow(canonicalRow = null, message = {}) {
  if (!canonicalRow || String(canonicalRow?.kind || '').trim().toLowerCase() !== 'iteration') {
    return message?._iterationData || {};
  }
  const rounds = Array.isArray(canonicalRow?.rounds) ? canonicalRow.rounds : [];
  const executionGroups = rounds.map((round) => ({
    pageId: round?.pageId || round?.renderKey || '',
    assistantMessageId: (Array.isArray(round?.modelSteps) ? round.modelSteps[0]?.assistantMessageId : '') || '',
    iteration: Number(round?.iteration || 0) || 0,
    phase: round?.phase || '',
    narration: round?.narration || '',
    content: round?.content || '',
    status: round?.status || '',
    finalResponse: !!round?.finalResponse,
    executionRole: (Array.isArray(round?.modelSteps) ? round.modelSteps[0]?.executionRole : '') || round?.executionRole || '',
    modelSteps: Array.isArray(round?.modelSteps) ? round.modelSteps : [],
    toolSteps: Array.isArray(round?.toolCalls) ? round.toolCalls : [],
    toolCallsPlanned: [],
  }));
  const firstNarration = rounds.map((round) => String(round?.narration || '').trim()).find(Boolean) || '';
  const finalContent = [...rounds].reverse().map((round) => String(round?.content || '').trim()).find(Boolean) || '';
  return {
    ...(message?._iterationData || {}),
    turnId: canonicalRow?.turnId || message?._iterationData?.turnId || '',
    status: canonicalRow?.lifecycle || message?._iterationData?.status || '',
    turnStartedAt: canonicalRow?.createdAt || message?._iterationData?.turnStartedAt || '',
    narration: firstNarration ? { content: firstNarration } : (message?._iterationData?.narration || null),
    response: {
      ...(message?._iterationData?.response || {}),
      content: finalContent || message?._iterationData?.response?.content || '',
      status: canonicalRow?.lifecycle || message?._iterationData?.response?.status || '',
    },
    executionGroups,
    linkedConversations: Array.isArray(canonicalRow?.linkedConversations) ? canonicalRow.linkedConversations : (message?._iterationData?.linkedConversations || []),
    isLatestIteration: !!canonicalRow?.isStreaming,
  };
}

export function resolveCanonicalDetailStep(canonicalRow = null, step = {}) {
  if (!canonicalRow || !step || typeof step !== 'object') return step;
  const rounds = Array.isArray(canonicalRow?.rounds) ? canonicalRow.rounds : [];
  const targetKind = String(step?.kind || '').trim().toLowerCase();
  const targetId = String(step?.modelCallId || step?.toolCallId || step?.id || '').trim();
  if (!targetKind || !targetId) return step;
  for (const round of rounds) {
    if (targetKind === 'model') {
      for (const modelStep of Array.isArray(round?.modelSteps) ? round.modelSteps : []) {
        const candidateId = String(modelStep?.modelCallId || modelStep?.renderKey || '').trim();
        if (candidateId === targetId) {
          return {
            ...step,
            modelCallId: modelStep?.modelCallId || step?.modelCallId || '',
            assistantMessageId: modelStep?.assistantMessageId || step?.assistantMessageId || '',
            requestPayloadId: modelStep?.requestPayloadId || step?.requestPayloadId || '',
            responsePayloadId: modelStep?.responsePayloadId || step?.responsePayloadId || '',
            providerRequestPayloadId: modelStep?.providerRequestPayloadId || step?.providerRequestPayloadId || '',
            providerResponsePayloadId: modelStep?.providerResponsePayloadId || step?.providerResponsePayloadId || '',
            streamPayloadId: modelStep?.streamPayloadId || step?.streamPayloadId || '',
            requestPayload: modelStep?.requestPayload ?? step?.requestPayload ?? null,
            responsePayload: modelStep?.responsePayload ?? step?.responsePayload ?? null,
            providerRequestPayload: modelStep?.providerRequestPayload ?? step?.providerRequestPayload ?? null,
            providerResponsePayload: modelStep?.providerResponsePayload ?? step?.providerResponsePayload ?? null,
            streamPayload: modelStep?.streamPayload ?? step?.streamPayload ?? null,
          };
        }
      }
      continue;
    }
    for (const toolStep of Array.isArray(round?.toolCalls) ? round.toolCalls : []) {
      const candidateId = String(toolStep?.toolCallId || toolStep?.renderKey || '').trim();
      if (candidateId === targetId) {
        return {
          ...step,
          toolCallId: toolStep?.toolCallId || step?.toolCallId || '',
          toolMessageId: toolStep?.toolMessageId || step?.toolMessageId || '',
          requestPayloadId: toolStep?.requestPayloadId || step?.requestPayloadId || '',
          responsePayloadId: toolStep?.responsePayloadId || step?.responsePayloadId || '',
          requestPayload: toolStep?.requestPayload ?? step?.requestPayload ?? null,
          responsePayload: toolStep?.responsePayload ?? step?.responsePayload ?? null,
        };
      }
    }
  }
  return step;
}

export default function IterationBlock({ message, canonicalRow = null, context, showToolFeedDetail = true }) {
  const { showDetail } = useContext(DetailContext);
  const { showExecutionDetails = true } = useContext(ConversationViewContext);
  const data = useMemo(() => buildIterationDataFromCanonicalRow(canonicalRow, message), [canonicalRow, message]);
  const toolCalls = Array.isArray(data.toolCalls) ? data.toolCalls : [];
  const displayToolCalls = useMemo(
    () => (Array.isArray(toolCalls) ? [...toolCalls] : []),
    [toolCalls]
  );
  const isLatestIteration = !!data?.isLatestIteration;
  const [now, setNow] = useState(Date.now());
  const [isElicitationOpen, setIsElicitationOpen] = useState(false);
  const [linkedConversationStates, setLinkedConversationStates] = useState([]);
  const [expandedLinkedIds, setExpandedLinkedIds] = useState({});
  const [linkedSectionExpanded, setLinkedSectionExpanded] = useState(false);
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
    let groups = canonicalGroups.length > 0
      ? mapCanonicalExecutionGroups(canonicalGroups)
      : mapFallbackExecutionGroups(data);
    const responseContent = String(data?.response?.content || '').trim();
    const hasExplicitFinalGroup = groups.some((group) => {
      const finalText = String(group?.finalContent || '').trim();
      return !!group?.finalResponse && finalText !== '';
    });
    if (responseContent && groups.length > 0 && !hasExplicitFinalGroup) {
      const lastIndex = groups.length - 1;
      const last = groups[lastIndex] || {};
      groups = groups.slice();
      groups[lastIndex] = {
        ...last,
        finalResponse: true,
        finalContent: responseContent,
        status: String(last?.status || data?.status || 'completed').trim() || 'completed'
      };
    }
    // Inject elicitation as a step in the last group so it appears in execution details.
    const elic = message?.elicitation || data?.elicitation;
    const elicId = String(message?.elicitationId || data?.elicitationId || '').trim();
    if (elic && elicId && groups.length > 0) {
      const last = groups[groups.length - 1];
      const alreadyPresent = last.toolSteps.some((s) => s.elicitationId === elicId);
      if (!alreadyPresent) {
        const elicStatus = normalizeElicitationStatus(
          elic?.status
          || message?.elicitationStatus
          || data?.elicitationStatus
          || message?.status
          || data?.status
        ) || 'pending';
        const elicStep = {
          id: `elicitation:${elicId}`,
          elicitationId: elicId,
          kind: 'elicitation',
          reason: 'elicitation',
          toolName: elicitationStepLabel(elicStatus),
          message: elic?.message || '',
          status: elicStatus,
          startedAt: message?.createdAt || '',
          completedAt: '',
          latencyMs: null,
          requestedSchema: elic?.requestedSchema || null,
          url: elic?.url || '',
          mode: elic?.mode || ''
        };
        const existingToolSteps = Array.isArray(last.toolSteps) ? last.toolSteps : [];
        const terminalLifecycleSteps = existingToolSteps.filter((step) => {
          const kind = String(step?.kind || '').trim().toLowerCase();
          const reason = String(step?.reason || step?.toolName || '').trim().toLowerCase();
          return kind === 'turn' && (reason === 'turn_completed' || reason === 'turn_failed' || reason === 'turn_canceled' || reason === 'turn_cancelled');
        });
        const nonTerminalSteps = existingToolSteps.filter((step) => !terminalLifecycleSteps.includes(step));
        groups[groups.length - 1] = {
          ...last,
          toolSteps: [...nonTerminalSteps, elicStep, ...terminalLifecycleSteps],
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
    narrationContent: data?.narration?.content,
    streamContent: data?.streamContent,
    errorMessage: data?.errorMessage
  }), [allGroupEntries, data?.errorMessage, data?.narration?.content, data?.response?.content, data?.streamContent, message?.content]);
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
    for (const entry of Array.isArray(data?.linkedConversations) ? data.linkedConversations : []) {
      const id = String(entry?.conversationId || entry?.ConversationId || '').trim();
      if (!id || seen.has(id)) continue;
      seen.add(id);
      ids.push(id);
    }
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
    return resolveIterationElapsedAnchor(data, allGroupEntries, linkedConversationStates, message, isActiveIteration);
  }, [allGroupEntries, linkedConversationStates, data, message, isActiveIteration]);

  const elapsedLabel = useMemo(() => {
    const sourceSteps = allGroupEntries.flatMap((group) => [
      ...(group?.modelStep ? [group.modelStep] : []),
      ...(Array.isArray(group?.toolSteps) ? group.toolSteps : [])
    ]);
    const explicit = formatDurationClock(totalLatencyMs(sourceSteps));
    const turnCompletedAt = parseTimestamp(data?.turnCompletedAt || '');
    if (!isActiveIteration) {
      if (turnStartedAt && turnCompletedAt && turnCompletedAt >= turnStartedAt) {
        return formatDurationClock(turnCompletedAt - turnStartedAt) || explicit;
      }
      return explicit;
    }
    if (!turnStartedAt) return explicit;
    return formatDurationClock(Math.max(0, now - turnStartedAt)) || explicit;
  }, [allGroupEntries, isActiveIteration, turnStartedAt, data?.turnCompletedAt, now]);

  const linkedConversations = useMemo(() => {
    const seen = new Map();
    const canonicalLinked = Array.isArray(data?.linkedConversations) ? data.linkedConversations : [];
    for (const entry of canonicalLinked) {
      const id = String(entry?.conversationId || entry?.ConversationId || '').trim();
      if (!id) continue;
      if (!seen.has(id)) {
        seen.set(id, {
          linkedConversationId: id,
          conversationId: id,
          linkedConversationAgentId: String(entry?.agentId || entry?.AgentId || '').trim(),
          agentId: String(entry?.agentId || entry?.AgentId || '').trim(),
          linkedConversationTitle: String(entry?.title || entry?.Title || '').trim(),
          title: String(entry?.title || entry?.Title || '').trim(),
          status: String(entry?.status || entry?.Status || '').trim(),
          response: String(entry?.response || entry?.Response || '').trim(),
          createdAt: entry?.createdAt || entry?.CreatedAt || '',
          updatedAt: entry?.updatedAt || entry?.UpdatedAt || '',
        });
      }
    }
    for (const group of allGroupEntries) {
      for (const step of group.toolSteps) {
        const id = linkedConversationId(step);
        if (!id || !canOpenLinkedConversation(step)) continue;
        if (!seen.has(id)) seen.set(id, step);
      }
    }
    for (const step of Array.isArray(data?.toolCalls) ? data.toolCalls : []) {
      const id = linkedConversationId(step);
      if (!id || !canOpenLinkedConversation(step)) continue;
      if (!seen.has(id)) seen.set(id, step);
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
    return [...seen.values()].filter((entry) => !isQueuedLinkedConversationPreview(entry));
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

  const visibleGroups = displayGroupEntries;

  const visibleRenderedText = useMemo(() => resolveIterationBubbleContent({
    visibleGroups,
    iterationContent: message?.content,
    responseContent: data?.response?.content,
    narrationContent: data?.narration?.content,
    streamContent: data?.streamContent,
    errorMessage: data?.errorMessage
  }), [data?.errorMessage, data?.narration?.content, data?.response?.content, data?.streamContent, message?.content, visibleGroups]);
  const hasVisibleElicitation = !!data?.response?.elicitation?.requestedSchema;
  const hasPendingExecutionElicitation = useMemo(
    () => hasPendingElicitationStep(visibleGroups),
    [visibleGroups]
  );
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
            {phaseBadgeLabel(group) ? (
              <span className="app-iteration-model-summary">{phaseBadgeLabel(group)}</span>
            ) : null}
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
              onClick={() => showDetail?.(resolveCanonicalDetailStep(canonicalRow, { ...modelStep, kind: 'model' }))}
            >
              Details
            </Button>
          </div>
        </div>
      ) : null}
      {group.toolSteps.length > 0 ? (() => {
        const lifecycleTools = group.toolSteps.filter((toolStep) => String(toolStep?.kind || '').toLowerCase() === 'turn');
        const regularTools = group.toolSteps.filter((toolStep) => String(toolStep?.kind || '').toLowerCase() !== 'turn');
        return (
          <div className="app-iteration-tool-list">
            {lifecycleTools.map((toolStep, toolIndex) => (
              <div className="app-iteration-tool-row" key={`${group.id}:lifecycle:${toolIndex}:${stepKey(toolStep)}`}>
                <div className="app-iteration-tool-row-main">
                  <span className="app-iteration-tool-icon">{displayItemRowIcon(toolStep)}</span>
                  <span className="app-iteration-tool-row-title">{displayItemRowTitle(toolStep)}</span>
                  {toolStepSummaryText(toolStep) ? (
                    <span className="app-iteration-model-summary">{toolStepSummaryText(toolStep)}</span>
                  ) : null}
                </div>
                <div className="app-iteration-tool-row-meta">
                  <span className={`app-iteration-status tone-${statusTone(toolStep?.status)}`}>{statusLabel(toolStep?.status)}</span>
                  {latencyLabel(toolStep, now, turnStartedAt) ? (
                    <span className="app-iteration-group-time">{latencyLabel(toolStep, now, turnStartedAt)}</span>
                  ) : null}
                  <Button minimal small className="app-iteration-link" onClick={() => showDetail?.(resolveCanonicalDetailStep(canonicalRow, toolStep))}>Details</Button>
                </div>
              </div>
            ))}
            {regularTools.map((toolStep, toolIndex) => (
              <div className="app-iteration-tool-row" key={`${group.id}:tool:${toolIndex}:${stepKey(toolStep)}`}>
                <div className="app-iteration-tool-row-main">
                  <span className="app-iteration-tool-icon">{displayItemRowIcon(toolStep)}</span>
                  <span className="app-iteration-tool-row-title">{displayItemRowTitle(toolStep)}</span>
                  {toolStepSummaryText(toolStep) ? (
                    <span className="app-iteration-model-summary">{toolStepSummaryText(toolStep)}</span>
                  ) : null}
                </div>
                <div className="app-iteration-tool-row-meta">
                  <span className={`app-iteration-status tone-${statusTone(toolStep?.status)}`}>{statusLabel(toolStep?.status)}</span>
                  {latencyLabel(toolStep, now, turnStartedAt) ? (
                    <span className="app-iteration-group-time">{latencyLabel(toolStep, now, turnStartedAt)}</span>
                  ) : null}
                  <Button minimal small className="app-iteration-link" onClick={() => showDetail?.(resolveCanonicalDetailStep(canonicalRow, toolStep))}>Details</Button>
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
                onClick={() => showDetail?.(resolveCanonicalDetailStep(canonicalRow, modelStep))}
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
                    onClick={() => showDetail?.(resolveCanonicalDetailStep(canonicalRow, toolStep))}
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
        const previousById = new Map((Array.isArray(linkedConversationStates) ? linkedConversationStates : []).map((entry) => [
          String(entry?.conversationId || '').trim(),
          entry
        ]));
        const rows = baseRows.map((entry) => {
          const conversationId = String(entry?.conversationId || '').trim();
          const previous = conversationId ? previousById.get(conversationId) : null;
          return {
            ...(previous || {}),
            ...(entry || {}),
            conversationId,
            title: String(entry?.title || previous?.title || '').trim(),
            agentId: String(entry?.agentId || previous?.agentId || '').trim(),
            status: String(entry?.status || previous?.status || '').trim(),
            response: String(entry?.response || previous?.response || '').trim(),
            updatedAt: String(entry?.updatedAt || previous?.updatedAt || '').trim(),
            previewGroups: Array.isArray(previous?.previewGroups) ? previous.previewGroups : []
          };
        });
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
    if (isLatestIteration && isActiveIteration) {
      return undefined;
    }
    const idsToHydrate = linkedConversations
      .map((step) => String(step?.conversationId || step?.linkedConversationId || '').trim())
      .filter(Boolean)
      .filter((id) => linkedSectionExpanded || expandedLinkedIds[id]);
    if (idsToHydrate.length === 0) return undefined;
    let cancelled = false;
    const loadLinkedPreview = async () => {
      const rows = await Promise.all(idsToHydrate.map(async (conversationId) => {
        try {
          const [conversation, transcript] = await Promise.all([
            client.getConversation(conversationId).catch(() => null),
            client.getTranscript({
              conversationId,
              includeModelCalls: true,
              includeToolCalls: true
            }, {
              executionGroupLimit: 1,
              executionGroupOffset: 0
            }).catch(() => null)
          ]);
          const preview = transcript ? sdkSummarizeLinkedConversationTranscript(transcript) : null;
          return {
            conversationId,
            title: String(conversation?.title || '').trim(),
            agentId: String(conversation?.agentId || '').trim(),
            status: String(preview?.status || '').trim(),
            response: String(preview?.response || '').trim(),
            updatedAt: String(preview?.updatedAt || '').trim(),
            previewGroups: Array.isArray(preview?.previewGroups) ? preview.previewGroups : []
          };
        } catch (_) {
          return null;
        }
      }));
      if (cancelled) return;
      const updates = new Map(rows.filter(Boolean).map((entry) => [String(entry?.conversationId || '').trim(), entry]));
      if (updates.size === 0) return;
      setLinkedConversationStates((current) => (Array.isArray(current) ? current : []).map((entry) => {
        const id = String(entry?.conversationId || '').trim();
        const update = updates.get(id);
        if (!update) return entry;
        return {
          ...entry,
          ...update,
          title: String(update?.title || entry?.title || '').trim(),
          agentId: String(update?.agentId || entry?.agentId || '').trim(),
          status: String(update?.status || entry?.status || '').trim(),
          response: String(update?.response || entry?.response || '').trim(),
          updatedAt: String(update?.updatedAt || entry?.updatedAt || '').trim(),
          previewGroups: Array.isArray(update?.previewGroups) && update.previewGroups.length > 0
            ? update.previewGroups
            : (Array.isArray(entry?.previewGroups) ? entry.previewGroups : [])
        };
      }));
    };
    void loadLinkedPreview();
    return () => {
      cancelled = true;
    };
  }, [linkedConversations, linkedSectionExpanded, expandedLinkedIds, isLatestIteration, isActiveIteration]);

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
    const lifecycleOnly = displayGroupEntries.length > 0
      && displayGroupEntries.every((group) => String(group?.groupKind || '').trim().toLowerCase() === 'lifecycle');
    if (displayGroupEntries.length === 0 || lifecycleOnly) {
      logIterationDebug('structure-anomaly', {
        messageId: message?.id || '',
        turnId: data?.turnId || '',
        displayGroupCount: displayGroupEntries.length,
        allGroupCount: allGroupEntries.length,
        lifecycleOnly,
        iterationDisplayStatus: iterationDisplayStatus || '',
        dataStatus: String(data?.status || '').trim(),
        messageStatus: String(message?.status || message?.turnStatus || '').trim(),
        visibleRenderedText: visibleRenderedText || '',
        allGroupKinds: allGroupEntries.map((group) => String(group?.groupKind || '').trim()),
        displayGroupKinds: displayGroupEntries.map((group) => String(group?.groupKind || '').trim()),
      });
    }
  }, [allGroupEntries, data?.status, data?.turnId, displayGroupEntries, iterationDisplayStatus, message?.id, message?.status, message?.turnStatus, visibleRenderedText]);

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
    setExpandedLinkedIds({});
    setLinkedSectionExpanded(false);
  }, [message?.id, data?.turnId]);

  useEffect(() => {
    if (hasVisibleElicitation && (elicitationStatus === '' || elicitationStatus === 'pending' || elicitationStatus === 'open')) {
      setIsElicitationOpen(true);
    }
  }, [hasVisibleElicitation, elicitationStatus, message?.id]);

  useEffect(() => {
    if (!shouldAutoScrollExecutionGroups({ collapsed, isActiveIteration, iterationDisplayStatus })) return;
    const el = groupsRef.current;
    if (el) {
      el.scrollTop = el.scrollHeight;
    }
  }, [collapsed, isActiveIteration, iterationDisplayStatus, visibleGroups]);

  return (
    <>
      {showExecutionDetails ? (
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
              {linkedConversations.length > 0 ? (
                <div className="app-iteration-linked-section">
                  <button
                    type="button"
                    className="app-iteration-linked-label"
                    onClick={() => setLinkedSectionExpanded((value) => !value)}
                  >
                    <span className="app-iteration-linked-label-icon">🔗</span>
                    <span>Linked conversations</span>
                    <span className="app-iteration-linked-conv-arrow">{linkedSectionExpanded ? '▾' : '▸'}</span>
                  </button>
                  {linkedSectionExpanded ? (
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
                  ) : null}
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
      ) : null}
      {showExecutionDetails && showToolFeedDetail ? <ToolFeedDetail context={context} /> : null}
      {!hasVisibleElicitation && !hasPendingExecutionElicitation && shouldShowNarrationBubble(visibleGroups, visibleRenderedText, data?.response?.content) ? (
        <BubbleMessage
          message={{
            id: `${message?.id || 'iteration'}:narration`,
            role: 'assistant',
            content: visibleRenderedText,
            generatedFiles
          }}
        />
      ) : null}
    </>
  );
}
