import React from 'react';
import { useSignals } from '@preact/signals-react/runtime';
import { activeWindows, removeWindow, selectedWindowId } from 'forge/core';
import { DetailContext } from '../context/DetailContext';
import { client } from '../services/agentlyClient';
import { setStage } from '../services/stageBus';
import { publishActiveConversation } from '../services/chatRuntime';
import {
  getScopedConversationSelection,
  getSelectedWindow,
  isLinkedChildWindow,
  MAIN_CHAT_WINDOW_ID,
  openLinkedConversationWindow,
  returnToParentConversation
} from '../services/conversationWindow';
import { displayStepIcon, displayStepTitle } from '../services/toolPresentation';
import { openFileViewDialog, updateFileViewDialog } from '../utils/dialogBus';

const MAX_TIMELINE_EVENTS = 240;
const EXECUTION_GROUP_PAGE_SIZE = 10;
const PAGE_SIZE_OPTIONS = ['1', '5', '10', 'all'];

function firstList(payload, keys = []) {
  for (const key of keys) {
    const value = payload?.[key];
    if (Array.isArray(value)) return value;
  }
  return [];
}

function firstString(...values) {
  for (const value of values) {
    const text = String(value || '').trim();
    if (text) return text;
  }
  return '';
}

function firstNumber(...values) {
  for (const value of values) {
    const num = Number(value);
    if (Number.isFinite(num)) return num;
  }
  return 0;
}

function toneForStatus(status) {
  const normalized = String(status || '').trim().toLowerCase();
  if (!normalized || ['running', 'thinking', 'streaming', 'executing'].includes(normalized)) return 'warm';
  if (['completed', 'done', 'success', 'succeeded'].includes(normalized)) return 'cool';
  if (['failed', 'error', 'canceled', 'cancelled', 'terminated'].includes(normalized)) return 'alert';
  return 'muted';
}

function relativeTime(value) {
  const ts = Date.parse(String(value || ''));
  if (!Number.isFinite(ts)) return '';
  const diff = Math.max(0, Date.now() - ts);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (diff < hour) return `${Math.max(1, Math.floor(diff / minute))}m`;
  if (diff < day) return `${Math.floor(diff / hour)}h`;
  return `${Math.floor(diff / day)}d`;
}

function extractMetadata(payload) {
  const data = payload?.data || payload || {};
  return {
    defaults: data.defaults || {
      agent: data.defaultAgent || '',
      model: data.defaultModel || '',
      embedder: data.defaultEmbedder || ''
    },
    capabilities: data.capabilities || {},
    agentInfos: firstList(data, ['agentInfos', 'AgentInfos']),
    modelInfos: firstList(data, ['modelInfos', 'ModelInfos'])
  };
}

function extractTurns(payload) {
  const data = payload?.data || payload || {};
  return firstList(data, ['turns', 'Turns']);
}

function summarizeModel(group) {
  const modelCall = group?.modelCall || {};
  return firstString(
    modelCall.model && modelCall.provider ? `${modelCall.provider}/${modelCall.model}` : '',
    modelCall.model,
    group?.assistantMessageId,
    'model'
  );
}

export function plannedToolCalls(group = {}) {
  return Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned : [];
}

function agentLabel(value = '') {
  const text = String(value || '').trim();
  if (!text) return '';
  return text
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
    .replace(/\b\w/g, (char) => char.toUpperCase());
}

function normalizePageSize(value) {
  const text = String(value || '1').trim().toLowerCase();
  if (text === 'all') return 'all';
  if (['1', '5', '10'].includes(text)) return text;
  return '1';
}

function payloadBadges(step = {}) {
  const items = [
    { key: 'request', label: 'Request', value: step?.requestPayloadId || step?.requestPayload },
    { key: 'response', label: 'Response', value: step?.responsePayloadId || step?.responsePayload },
    { key: 'providerRequest', label: 'Provider Req', value: step?.providerRequestPayloadId || step?.providerRequestPayload },
    { key: 'providerResponse', label: 'Provider Resp', value: step?.providerResponsePayloadId || step?.providerResponsePayload },
    { key: 'stream', label: 'Stream', value: step?.streamPayloadId || step?.streamPayload }
  ];
  return items.filter((item) => !!item.value);
}

function collectFileUris(value, results = []) {
  if (!value) return results;
  if (Array.isArray(value)) {
    value.forEach((item) => collectFileUris(item, results));
    return results;
  }
  if (typeof value !== 'object') return results;
  const candidates = [
    value?.uri,
    value?.URI,
    value?.url,
    value?.URL,
    value?.path,
    value?.Path,
    value?.encodedURI,
    value?.encodedUri
  ];
  for (const candidate of candidates) {
    const text = String(candidate || '').trim();
    if (!text) continue;
    if (text.includes('://') || text.startsWith('/')) {
      results.push(text);
    }
  }
  for (const entry of Object.values(value)) {
    collectFileUris(entry, results);
  }
  return results;
}

async function fetchFileContent(uri = '') {
  const value = firstString(uri);
  if (!value) return '';
  return client.downloadWorkspaceFile(value);
}

function normalizeModelStep(group = {}) {
  const modelCall = group?.modelCall || {};
  return {
    id: firstString(
      modelCall?.messageId,
      modelCall?.MessageId,
      group?.assistantMessageId,
      group?.modelMessageId
    ),
    kind: 'model',
    reason: group?.finalResponse ? 'final_response' : 'thinking',
    toolName: summarizeModel(group),
    provider: firstString(modelCall?.provider, modelCall?.Provider),
    model: firstString(modelCall?.model, modelCall?.Model),
    status: firstString(modelCall?.status, modelCall?.Status, group?.status),
    latencyMs: firstNumber(modelCall?.latencyMs, modelCall?.LatencyMs),
    linkedConversationId: firstString(group?.linkedConversationId),
    requestPayloadId: firstString(modelCall?.requestPayloadId, modelCall?.RequestPayloadId),
    responsePayloadId: firstString(modelCall?.responsePayloadId, modelCall?.ResponsePayloadId),
    providerRequestPayloadId: firstString(modelCall?.providerRequestPayloadId, modelCall?.ProviderRequestPayloadId),
    providerResponsePayloadId: firstString(modelCall?.providerResponsePayloadId, modelCall?.ProviderResponsePayloadId),
    streamPayloadId: firstString(modelCall?.streamPayloadId, modelCall?.StreamPayloadId),
    requestPayload: modelCall?.modelCallRequestPayload || modelCall?.ModelCallRequestPayload || modelCall?.requestPayload || modelCall?.RequestPayload || null,
    responsePayload: modelCall?.modelCallResponsePayload || modelCall?.ModelCallResponsePayload || modelCall?.responsePayload || modelCall?.ResponsePayload || null,
    providerRequestPayload: modelCall?.modelCallProviderRequestPayload || modelCall?.ModelCallProviderRequestPayload || null,
    providerResponsePayload: modelCall?.modelCallProviderResponsePayload || modelCall?.ModelCallProviderResponsePayload || null,
    streamPayload: modelCall?.modelCallStreamPayload || modelCall?.ModelCallStreamPayload || null,
    toolCallsPlanned: plannedToolCalls(group)
  };
}

function normalizeToolStep(tool = {}, group = {}) {
  return {
    id: firstString(tool?.messageId, tool?.MessageId, tool?.id, tool?.Id),
    kind: 'tool',
    reason: 'tool_call',
    toolName: firstString(tool?.toolName, tool?.ToolName, 'tool'),
    status: firstString(tool?.status, tool?.Status, group?.status),
    latencyMs: firstNumber(tool?.latencyMs, tool?.LatencyMs),
    linkedConversationId: firstString(tool?.linkedConversationId, tool?.LinkedConversationId),
    requestPayloadId: firstString(tool?.requestPayloadId, tool?.RequestPayloadId),
    responsePayloadId: firstString(tool?.responsePayloadId, tool?.ResponsePayloadId),
    providerRequestPayloadId: firstString(tool?.providerRequestPayloadId, tool?.ProviderRequestPayloadId),
    providerResponsePayloadId: firstString(tool?.providerResponsePayloadId, tool?.ProviderResponsePayloadId),
    streamPayloadId: firstString(tool?.streamPayloadId, tool?.StreamPayloadId),
    requestPayload: tool?.requestPayload || tool?.RequestPayload || null,
    responsePayload: tool?.responsePayload || tool?.ResponsePayload || null,
    providerRequestPayload: tool?.providerRequestPayload || tool?.ProviderRequestPayload || null,
    providerResponsePayload: tool?.providerResponsePayload || tool?.ProviderResponsePayload || null,
    streamPayload: tool?.streamPayload || tool?.StreamPayload || null
  };
}

function extractExecutionGroups(turns = []) {
  const groups = [];
  for (const turn of turns) {
    const turnId = firstString(turn?.id, turn?.Id);
    const turnStatus = firstString(turn?.status, turn?.Status);
    const executionGroups = firstList(turn, ['executionGroups', 'ExecutionGroups']);
    for (const group of executionGroups) {
      groups.push({
        ...group,
        turnId,
        turnStatus,
        assistantMessageId: firstString(group?.assistantMessageId, group?.pageId),
        parentMessageId: firstString(group?.parentMessageId),
        sequence: firstNumber(group?.sequence, group?.iteration),
        iteration: firstNumber(group?.iteration),
        preamble: firstString(group?.preamble),
        content: firstString(group?.content),
        status: firstString(group?.status, turnStatus),
        finalResponse: Boolean(group?.finalResponse),
        modelSteps: Array.isArray(group?.modelSteps) ? group.modelSteps : (group?.modelCall ? [group.modelCall] : []),
        toolSteps: Array.isArray(group?.toolSteps) ? group.toolSteps : (Array.isArray(group?.toolCalls) ? group.toolCalls : []),
        toolCallsPlanned: Array.isArray(group?.toolCallsPlanned) ? group.toolCallsPlanned : []
      });
    }
  }
  return groups.sort((left, right) => {
    if (left.sequence !== right.sequence) return left.sequence - right.sequence;
    return String(left.assistantMessageId || '').localeCompare(String(right.assistantMessageId || ''));
  });
}

function transcriptMetaFromTurns(turns = [], pageIndex = 0, pageSize = '1') {
  const firstTurn = Array.isArray(turns) && turns.length > 0 ? turns[0] : null;
  const total = firstNumber(firstTurn?.executionGroupsTotal, firstTurn?.ExecutionGroupsTotal);
  const offset = firstNumber(firstTurn?.executionGroupsOffset, firstTurn?.ExecutionGroupsOffset);
  const limit = firstNumber(firstTurn?.executionGroupsLimit, firstTurn?.ExecutionGroupsLimit);
  const normalizedPageSize = normalizePageSize(pageSize);
  const effectiveLimit = normalizedPageSize === 'all'
    ? Math.max(total, limit || 1)
    : Math.max(1, Number(normalizedPageSize || 1));
  const pageCount = total > 0 ? Math.max(1, Math.ceil(total / Math.max(1, effectiveLimit))) : 1;
  return { total, offset, limit: effectiveLimit, pageCount, pageIndex };
}

function plainText(value) {
  return String(value || '')
    .replace(/[`*_>#-]/g, ' ')
    .replace(/\[(.*?)\]\((.*?)\)/g, '$1')
    .replace(/\s+/g, ' ')
    .trim();
}

function truncateText(value, limit = 72) {
  const text = plainText(value);
  if (text.length <= limit) return text;
  return `${text.slice(0, Math.max(0, limit - 1)).trimEnd()}…`;
}

export function modelPreamblePreview(group = {}, limit = 72) {
  return truncateText(group?.preamble || '', limit);
}

export function isPresentableGroup(group = {}) {
  return Boolean(
    firstString(group?.preamble)
    || (Array.isArray(group?.toolSteps) && group.toolSteps.length > 0)
    || (Array.isArray(group?.toolCalls) && group.toolCalls.length > 0)
    || (Array.isArray(group?.toolCallsPlanned) && group.toolCallsPlanned.length > 0)
    || (group?.finalResponse && firstString(group?.content))
  );
}

function mergeGroup(existing = {}, incoming = {}) {
  const toolStepsByKey = new Map();
  const mergedToolSteps = [];
  const existToolSteps = existing?.toolSteps || existing?.toolCalls || [];
  const incToolSteps = incoming?.toolSteps || incoming?.toolCalls || [];
  for (const entry of [...existToolSteps, ...incToolSteps]) {
    const key = firstString(entry?.toolCallId, entry?.toolMessageId, entry?.id, entry?.toolName);
    if (!key) {
      mergedToolSteps.push(entry);
      continue;
    }
    const prior = toolStepsByKey.get(key) || {};
    toolStepsByKey.set(key, { ...prior, ...entry });
  }
  for (const entry of toolStepsByKey.values()) {
    mergedToolSteps.push(entry);
  }
  const existModelSteps = existing?.modelSteps || (existing?.modelCall ? [existing.modelCall] : []);
  const incModelSteps = incoming?.modelSteps || (incoming?.modelCall ? [incoming.modelCall] : []);
  const mergedModelSteps = incModelSteps.length > 0 ? incModelSteps.map((ms, i) => ({ ...(existModelSteps[i] || {}), ...ms })) : existModelSteps;
  return {
    ...existing,
    ...incoming,
    assistantMessageId: firstString(incoming?.assistantMessageId, existing?.assistantMessageId),
    parentMessageId: firstString(incoming?.parentMessageId, existing?.parentMessageId),
    sequence: firstNumber(incoming?.sequence, existing?.sequence),
    iteration: firstNumber(incoming?.iteration, existing?.iteration),
    preamble: firstString(incoming?.preamble, existing?.preamble),
    content: firstString(incoming?.content, existing?.content),
    status: firstString(incoming?.status, existing?.status),
    finalResponse: Boolean(incoming?.finalResponse ?? existing?.finalResponse),
    modelSteps: mergedModelSteps,
    toolSteps: mergedToolSteps,
    toolCallsPlanned: Array.isArray(incoming?.toolCallsPlanned) && incoming.toolCallsPlanned.length > 0
      ? incoming.toolCallsPlanned
      : (Array.isArray(existing?.toolCallsPlanned) ? existing.toolCallsPlanned : [])
  };
}

function createLiveGroup(event = {}) {
  const assistantMessageId = firstString(event?.assistantMessageId, event?.messageId, event?.id);
  if (!assistantMessageId) return null;
  return {
    pageId: assistantMessageId,
    assistantMessageId,
    parentMessageId: firstString(event?.parentMessageId),
    sequence: firstNumber(event?.pageIndex, event?.iteration, 1),
    iteration: firstNumber(event?.iteration, event?.pageIndex, 1),
    preamble: firstString(event?.preamble),
    content: firstString(event?.content),
    status: firstString(event?.status, 'running'),
    finalResponse: Boolean(event?.finalResponse),
    modelSteps: event?.model ? [{
      modelCallId: assistantMessageId,
      provider: firstString(event?.model?.provider),
      model: firstString(event?.model?.model),
      status: firstString(event?.status, 'running'),
      requestPayloadId: firstString(event?.requestPayloadId),
      responsePayloadId: firstString(event?.responsePayloadId),
      providerRequestPayloadId: firstString(event?.providerRequestPayloadId),
      providerResponsePayloadId: firstString(event?.providerResponsePayloadId),
      streamPayloadId: firstString(event?.streamPayloadId)
    }] : [],
    toolSteps: [],
    toolCallsPlanned: Array.isArray(event?.toolCallsPlanned) ? event.toolCallsPlanned : []
  };
}

function applyStreamEventToGroups(groupsById = {}, rawEvent = {}) {
  const event = rawEvent || {};
  const type = firstString(event?.type).toLowerCase();
  const assistantMessageId = firstString(event?.assistantMessageId, event?.messageId, event?.id);
  const next = { ...groupsById };

  if (type === 'model_started' && assistantMessageId) {
    next[assistantMessageId] = mergeGroup(next[assistantMessageId], createLiveGroup(event));
    return next;
  }

  if ((type === 'model_completed' || type === 'text_delta') && assistantMessageId) {
    const current = next[assistantMessageId] || createLiveGroup(event);
    if (!current) return next;
    current.content = type === 'text_delta'
      ? `${firstString(current.content)}${firstString(event?.content)}`
      : firstString(event?.content, current.content);
    current.preamble = firstString(event?.preamble, current.preamble);
    current.status = firstString(event?.status, current.status);
    current.finalResponse = Boolean(event?.finalResponse ?? current.finalResponse);
    current.toolCallsPlanned = Array.isArray(event?.toolCallsPlanned) && event.toolCallsPlanned.length > 0
      ? event.toolCallsPlanned
      : (Array.isArray(current?.toolCallsPlanned) ? current.toolCallsPlanned : []);
    const existMs = Array.isArray(current.modelSteps) && current.modelSteps.length > 0 ? current.modelSteps[0] : {};
    current.modelSteps = [{
      ...existMs,
      modelCallId: firstString(assistantMessageId, existMs?.modelCallId),
      provider: firstString(event?.model?.provider, existMs?.provider),
      model: firstString(event?.model?.model, existMs?.model),
      status: firstString(event?.status, existMs?.status),
      requestPayloadId: firstString(event?.requestPayloadId, existMs?.requestPayloadId),
      responsePayloadId: firstString(event?.responsePayloadId, existMs?.responsePayloadId),
      providerRequestPayloadId: firstString(event?.providerRequestPayloadId, existMs?.providerRequestPayloadId),
      providerResponsePayloadId: firstString(event?.providerResponsePayloadId, existMs?.providerResponsePayloadId),
      streamPayloadId: firstString(event?.streamPayloadId, existMs?.streamPayloadId)
    }];
    next[assistantMessageId] = current;
    return next;
  }

  if (type === 'reasoning_delta' && assistantMessageId) {
    const current = next[assistantMessageId] || createLiveGroup(event);
    if (!current) return next;
    current.preamble = `${firstString(current.preamble)}${firstString(event?.content)}`;
    next[assistantMessageId] = current;
    return next;
  }

  if ((type === 'tool_call_started' || type === 'tool_call_completed') && assistantMessageId) {
    const current = next[assistantMessageId] || createLiveGroup(event);
    if (!current) return next;
    const toolKey = firstString(event?.toolCallId, event?.toolMessageId, event?.id, event?.toolName);
    const priorList = Array.isArray(current.toolSteps) ? current.toolSteps : (Array.isArray(current.toolCalls) ? current.toolCalls : []);
    const existingIndex = priorList.findIndex((entry) => firstString(entry?.toolCallId, entry?.toolMessageId, entry?.id, entry?.toolName) === toolKey);
    const toolEntry = {
      toolCallId: firstString(event?.toolCallId),
      toolMessageId: firstString(event?.toolMessageId, event?.id),
      toolName: firstString(event?.toolName),
      status: firstString(event?.status, existingIndex >= 0 ? priorList[existingIndex]?.status : ''),
      requestPayloadId: firstString(event?.requestPayloadId),
      responsePayloadId: firstString(event?.responsePayloadId),
      linkedConversationId: firstString(event?.linkedConversationId)
    };
    const nextTools = [...priorList];
    if (existingIndex >= 0) nextTools[existingIndex] = { ...nextTools[existingIndex], ...toolEntry };
    else nextTools.push(toolEntry);
    current.toolSteps = nextTools;
    current.status = firstString(event?.status, current.status);
    next[assistantMessageId] = current;
    return next;
  }

  if (type === 'turn_completed' && assistantMessageId && next[assistantMessageId]) {
    next[assistantMessageId] = { ...next[assistantMessageId], status: firstString(event?.status, next[assistantMessageId]?.status) };
  }
  return next;
}

export function mergeLatestTranscriptAndLiveGroups(transcriptGroups = [], liveGroupsById = {}, pageSize = '1') {
  const normalizedPageSize = normalizePageSize(pageSize);
  const mergedById = new Map();
  for (const group of transcriptGroups) {
    const key = firstString(group?.assistantMessageId);
    if (!key) continue;
    mergedById.set(key, group);
  }
  Object.values(liveGroupsById || {}).forEach((liveGroup) => {
    if (!isPresentableGroup(liveGroup)) return;
    const key = firstString(liveGroup?.assistantMessageId);
    if (!key) return;
    mergedById.set(key, mergeGroup(mergedById.get(key), liveGroup));
  });
  const merged = Array.from(mergedById.values()).sort((left, right) => {
    if (left.sequence !== right.sequence) return left.sequence - right.sequence;
    return String(left.assistantMessageId || '').localeCompare(String(right.assistantMessageId || ''));
  });
  if (normalizedPageSize === 'all') return merged;
  const limit = Math.max(1, Number(normalizedPageSize || 1));
  if (limit === 1) {
    const latestPresentable = [...merged].reverse().find((group) => isPresentableGroup(group));
    return latestPresentable ? [latestPresentable] : transcriptGroups.slice(-1);
  }
  return merged.slice(Math.max(0, merged.length - limit));
}

export function describeTimelineEvent(event) {
  const type = firstString(event?.type, 'event');
  const status = firstString(event?.status);
  const toolName = firstString(event?.toolName);
  const message = firstString(event?.content, event?.preamble, event?.error);
  const parts = [type];
  if (status) parts.push(status);
  if (toolName) parts.push(toolName);
  if (Array.isArray(event?.toolCallsPlanned) && event.toolCallsPlanned.length > 0) {
    parts.push(`planned ${event.toolCallsPlanned.map((item) => firstString(item?.toolName, item?.ToolName, 'tool')).join(', ')}`);
  }
  if (message) parts.push(message);
  return parts.join(' · ');
}

function eventKey(event, index) {
  return firstString(
    event?.id,
    event?.toolCallId,
    event?.assistantMessageId,
    `${event?.type || 'event'}:${index}`
  );
}

function readSelectedConversationId(windowId = '') {
  if (typeof window === 'undefined') return '';
  return getScopedConversationSelection(windowId || MAIN_CHAT_WINDOW_ID);
}

export default function ExecutionWorkspace() {
  useSignals();
  void selectedWindowId.value;
  void activeWindows.value;
  const { showDetail } = React.useContext(DetailContext);
  const [instanceWindowId] = React.useState(() => {
    const current = String(selectedWindowId.peek?.() || selectedWindowId.value || '').trim();
    return current || MAIN_CHAT_WINDOW_ID;
  });
  const [metadata, setMetadata] = React.useState({ defaults: {}, capabilities: {}, agentInfos: [], modelInfos: [] });
  const [conversationId, setConversationId] = React.useState(() => readSelectedConversationId(instanceWindowId));
  const [conversationTitle, setConversationTitle] = React.useState('');
  const [selectedAgent, setSelectedAgent] = React.useState('');
  const [selectedModel, setSelectedModel] = React.useState('');
  const [prompt, setPrompt] = React.useState('');
  const [turns, setTurns] = React.useState([]);
  const [transcriptMeta, setTranscriptMeta] = React.useState({ total: 0, offset: 0, limit: 1, pageCount: 1, pageIndex: 0 });
  const [timeline, setTimeline] = React.useState([]);
  const [liveGroups, setLiveGroups] = React.useState({});
  const [connectionState, setConnectionState] = React.useState('idle');
  const [isSending, setIsSending] = React.useState(false);
  const [error, setError] = React.useState('');
  const [pageSize, setPageSize] = React.useState('1');
  const [pageIndex, setPageIndex] = React.useState(0);
  const streamRef = React.useRef(null);
  const selectedWindow = getSelectedWindow();
  const linkedChildWindow = isLinkedChildWindow(selectedWindow) ? selectedWindow : null;

  const executionGroups = React.useMemo(() => extractExecutionGroups(turns), [turns]);
  const selectedAgentName = React.useMemo(() => {
    const match = metadata.agentInfos.find((item) => firstString(item?.id, item?.ID) === firstString(selectedAgent));
    return firstString(match?.name, match?.Name, agentLabel(selectedAgent), 'Agent');
  }, [metadata.agentInfos, selectedAgent]);
  const visibleGroups = React.useMemo(() => (
    pageIndex === 0
      ? mergeLatestTranscriptAndLiveGroups(executionGroups, liveGroups, pageSize)
      : executionGroups
  ), [executionGroups, liveGroups, pageIndex, pageSize]);

  const loadMetadata = React.useCallback(async () => {
    const payload = await client.getWorkspaceMetadata();
    const next = extractMetadata(payload);
    setMetadata(next);
    setSelectedAgent((current) => current || firstString(next.defaults?.agent));
    setSelectedModel((current) => current || firstString(next.defaults?.model));
  }, []);

  const loadConversation = React.useCallback(async (id) => {
    const nextId = firstString(id);
    if (!nextId) {
      setConversationTitle('');
      return;
    }
    const data = await client.getConversation(nextId);
    setConversationTitle(firstString(data?.title, data?.Title, nextId));
  }, []);

  const loadTranscript = React.useCallback(async (id, requestedPageSize = '1', requestedPageIndex = 0) => {
    const nextId = firstString(id);
    if (!nextId) {
      setTurns([]);
      setTranscriptMeta({ total: 0, offset: 0, limit: 1, pageCount: 1, pageIndex: 0 });
      return;
    }
    const transcriptInput = { conversationId: nextId, includeModelCalls: true, includeToolCalls: true };
    const probePayload = await client.getTranscript(transcriptInput, { executionGroupLimit: 1, executionGroupOffset: 0 });
    const probeTurns = extractTurns(probePayload);
    const probeMeta = transcriptMetaFromTurns(probeTurns, 0, requestedPageSize);
    const normalizedPageSize = normalizePageSize(requestedPageSize);
    const effectiveLimit = normalizedPageSize === 'all'
      ? Math.max(1, probeMeta.total)
      : Math.max(1, Number(normalizedPageSize || 1));
    const maxPageIndex = probeMeta.total > 0 ? Math.max(Math.ceil(probeMeta.total / effectiveLimit) - 1, 0) : 0;
    const safePageIndex = Math.min(Math.max(0, requestedPageIndex), maxPageIndex);
    const offset = normalizedPageSize === 'all'
      ? 0
      : Math.max(probeMeta.total - (effectiveLimit * (safePageIndex + 1)), 0);
    const payload = (offset === 0 && effectiveLimit === 1)
      ? probePayload
      : await client.getTranscript(transcriptInput, { executionGroupLimit: effectiveLimit, executionGroupOffset: offset });
    const actualTurns = extractTurns(payload);
    setTurns(actualTurns);
    setTranscriptMeta(transcriptMetaFromTurns(actualTurns, safePageIndex, normalizedPageSize));
  }, []);

  React.useEffect(() => {
    loadMetadata().catch((err) => {
      setError(String(err?.message || err));
      setStage({ phase: 'error', text: 'Metadata load failed' });
    });
  }, [loadMetadata]);

  React.useEffect(() => {
    if (!conversationId) {
      setTurns([]);
      setConversationTitle('');
      setLiveGroups({});
      return;
    }
    Promise.all([loadConversation(conversationId), loadTranscript(conversationId, pageSize, pageIndex)])
      .catch((err) => {
        setError(String(err?.message || err));
        setStage({ phase: 'error', text: 'Transcript load failed' });
      });
  }, [conversationId, loadConversation, loadTranscript, pageSize, pageIndex]);

  React.useEffect(() => {
    if (typeof window === 'undefined') return undefined;
    const onSelect = (event) => {
      const targetWindowId = firstString(event?.detail?.windowId);
      if (targetWindowId && targetWindowId !== instanceWindowId) return;
      const id = firstString(event?.detail?.id);
      setConversationId(id);
      if (!id) {
        setTimeline([]);
        setConversationTitle('');
        setLiveGroups({});
      }
    };
    const onNew = (event) => {
      const targetWindowId = firstString(event?.detail?.windowId);
      if (targetWindowId && targetWindowId !== instanceWindowId) return;
      setConversationId('');
      setTimeline([]);
      setTurns([]);
      setConversationTitle('');
      setLiveGroups({});
      setPageIndex(0);
      setPrompt('');
      setError('');
      setStage({ phase: 'ready', text: 'Ready' });
    };
    window.addEventListener('agently:conversation-select', onSelect);
    window.addEventListener('agently:conversation-new', onNew);
    return () => {
      window.removeEventListener('agently:conversation-select', onSelect);
      window.removeEventListener('agently:conversation-new', onNew);
    };
  }, [instanceWindowId]);

  React.useEffect(() => {
    if (streamRef.current) {
      streamRef.current.close();
      streamRef.current = null;
    }
    if (!conversationId) {
      setConnectionState('idle');
      return undefined;
    }
    setConnectionState('connecting');
    const subscription = client.streamEvents(conversationId, {
      onEvent: (payload) => {
        setConnectionState('live');
        setTimeline((current) => {
          const next = [{ ...payload, receivedAt: new Date().toISOString() }, ...current];
          return next.slice(0, MAX_TIMELINE_EVENTS);
        });
        setLiveGroups((current) => applyStreamEventToGroups(current, payload));
        const type = firstString(payload?.type).toLowerCase();
        if (type === 'text_delta') {
          setStage({ phase: 'streaming', text: 'Streaming response…' });
        } else if (type === 'reasoning_delta') {
          setStage({ phase: 'streaming', text: 'Assistant reasoning…' });
        } else if (type === 'tool_call_started') {
          setStage({ phase: 'executing', text: `Executing ${firstString(payload?.toolName, 'tool')}…` });
        } else if (type === 'tool_call_delta') {
          setStage({ phase: 'executing', text: `Building ${firstString(payload?.toolName, 'tool')} arguments…` });
        } else if (type === 'turn_completed' || type === 'turn_failed' || type === 'turn_canceled') {
          setStage({ phase: 'done', text: type === 'turn_failed' ? 'Failed' : type === 'turn_canceled' ? 'Canceled' : 'Done' });
          window.setTimeout(() => setStage({ phase: 'ready', text: 'Ready' }), 900);
        }
        if (['turn_completed', 'turn_failed', 'turn_canceled', 'tool_call_completed', 'model_completed', 'control'].includes(type)) {
          loadTranscript(conversationId, pageSize, pageIndex).catch(() => {});
          loadConversation(conversationId).catch(() => {});
        }
      },
      onError: () => {
        setConnectionState('disconnected');
        setStage({ phase: 'error', text: 'Stream disconnected' });
      },
    });
    streamRef.current = subscription;

    return () => {
      subscription.close();
      if (streamRef.current === subscription) {
        streamRef.current = null;
      }
    };
  }, [conversationId, loadConversation, loadTranscript, pageIndex, pageSize]);

  async function handleSubmit(event) {
    event.preventDefault();
    const query = prompt.trim();
    if (!query) return;
    setIsSending(true);
    setError('');
    setStage({ phase: 'executing', text: 'Executing…' });
    try {
      const payload = await client.query({
        conversationId: firstString(conversationId),
        query,
        agentId: firstString(selectedAgent, metadata.defaults?.agent),
        model: firstString(selectedModel, metadata.defaults?.model)
      });
      const nextConversationId = firstString(payload?.conversationId, payload?.ConversationID, conversationId);
      if (nextConversationId) {
        setConversationId(nextConversationId);
        setPageIndex(0);
        setLiveGroups({});
        publishActiveConversation(nextConversationId);
      }
      setPrompt('');
      await Promise.all([loadConversation(nextConversationId), loadTranscript(nextConversationId, pageSize, 0)]);
    } catch (err) {
      setError(String(err?.message || err));
      setStage({ phase: 'error', text: 'Query failed' });
    } finally {
      setIsSending(false);
    }
  }

  function openStepDetail(step) {
    if (!step || typeof showDetail !== 'function') return;
    showDetail(step);
  }

  async function openFileFromStep(step, event) {
    event?.stopPropagation?.();
    const candidates = [
      ...(collectFileUris(step?.responsePayload || {})),
      ...(collectFileUris(step?.providerResponsePayload || {})),
      ...(collectFileUris(step?.streamPayload || {}))
    ];
    const seen = new Set();
    const uri = candidates.find((item) => {
      const key = firstString(item);
      if (!key || seen.has(key)) return false;
      seen.add(key);
      return true;
    }) || '';
    if (!uri) return;
    const title = uri.split('/').pop() || 'File';
    openFileViewDialog({ title, uri, loading: true, content: '' });
    try {
      const content = await fetchFileContent(uri);
      updateFileViewDialog({ content, loading: false });
    } catch (err) {
      updateFileViewDialog({ content: String(err?.message || err || 'failed to load file'), loading: false });
    }
  }

  function openLinkedConversationFromStep(step, event) {
    event?.stopPropagation?.();
    const linkedId = firstString(step?.linkedConversationId);
    if (!linkedId) return;
    openLinkedConversationWindow(linkedId);
  }

  function goOlder() {
    setPageIndex((current) => Math.min(current + 1, Math.max(0, transcriptMeta.pageCount - 1)));
  }

  function goNewer() {
    setPageIndex((current) => Math.max(0, current - 1));
  }

  function goLatest() {
    setPageIndex(0);
  }

  return (
    <section className="app-execution-workspace" data-testid="execution-workspace">
      {linkedChildWindow ? (
        <div className="app-linked-child-banner">
          <div className="app-linked-child-dots">
            <button
              type="button"
              className="app-linked-child-dot app-linked-child-dot-close"
              aria-label="Close linked conversation"
              title="Close linked conversation"
              onClick={() => removeWindow(linkedChildWindow.windowId)}
            >
              <span className="app-linked-child-dot-icon">×</span>
            </button>
            <button
              type="button"
              className="app-linked-child-dot app-linked-child-dot-back"
              aria-label="Return to parent conversation"
              title="Return to parent conversation"
              onClick={() => returnToParentConversation(linkedChildWindow)}
            >
              <span className="app-linked-child-dot-icon">←</span>
            </button>
            <span className="app-linked-child-dot app-linked-child-dot-inert" aria-hidden="true">
              <span className="app-linked-child-dot-icon">•</span>
            </span>
          </div>
          <div className="app-linked-child-title">Linked conversation</div>
        </div>
      ) : null}
      <div className="app-execution-toolbar">
        <div>
          <div className="app-execution-kicker">Conversation</div>
          <h2>{conversationTitle || 'New conversation'}</h2>
          {conversationId ? <div className="app-execution-subtitle">{conversationId}</div> : null}
        </div>
        <div className="app-execution-controls">
          <label>
            <span>Agent</span>
            <select value={selectedAgent} onChange={(event) => setSelectedAgent(event.target.value)}>
              {metadata.agentInfos.map((item) => {
                const id = firstString(item?.id, item?.ID);
                return <option key={id} value={id}>{firstString(item?.name, item?.Name, id)}</option>;
              })}
            </select>
          </label>
          <label>
            <span>Model</span>
            <select value={selectedModel} onChange={(event) => setSelectedModel(event.target.value)}>
              {metadata.modelInfos.map((item) => {
                const id = firstString(item?.id, item?.ID);
                return <option key={id} value={id}>{firstString(item?.name, item?.Name, id)}</option>;
              })}
            </select>
          </label>
          <div className={`app-execution-connection tone-${connectionState}`}>
            <span className="app-execution-dot" />
            {connectionState}
          </div>
        </div>
      </div>

      <div className="app-execution-composer">
        <form onSubmit={handleSubmit}>
          <textarea
            value={prompt}
            onChange={(event) => setPrompt(event.target.value)}
            placeholder="Ask the agent to inspect, plan, or execute."
          />
          <div className="app-execution-composer-actions">
            <p>Rendering transcript `executionGroups` directly from `agently-core`.</p>
            <button type="submit" disabled={isSending}>{isSending ? 'Sending...' : 'Run'}</button>
          </div>
        </form>
        {error ? <div className="app-execution-error">{error}</div> : null}
      </div>

      <div className="app-execution-board">
        <div className="app-execution-column">
          <div className="app-execution-section-head">
            <span>{`Execution details · ${selectedAgentName} (${visibleGroups.length})`}</span>
            <div className="app-execution-section-meta">
              <span className="app-execution-meta-text">
                {pageIndex === 0 ? 'Latest' : `History +${pageIndex}`}
              </span>
              <span className="app-execution-pill">{transcriptMeta.total || visibleGroups.length}</span>
            </div>
          </div>
          <div className="app-execution-page-list">
            {visibleGroups.map((group) => (
              <article key={group.assistantMessageId} className="app-execution-page-card">
                <header className="app-execution-page-header">
                  <div>
                    <div className="app-execution-page-title">Iteration {group.iteration || group.sequence || 1}</div>
                    <div className="app-execution-page-subtitle">{summarizeModel(group)}</div>
                  </div>
                  <span className={`app-execution-pill tone-${toneForStatus(group.status)}`}>{group.status || 'unknown'}</span>
                </header>
                {group.modelCall ? (
                  <div
                    role="button"
                    tabIndex={0}
                    className="app-execution-step-card model is-clickable"
                    onClick={() => openStepDetail(normalizeModelStep(group))}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        openStepDetail(normalizeModelStep(group));
                      }
                    }}
                  >
                    <div className="app-execution-step-head">
                      <span>{`${displayStepIcon(normalizeModelStep(group))} ${displayStepTitle(normalizeModelStep(group))}`}</span>
                      <span title={String(group?.preamble || '').trim()}>
                        {modelPreamblePreview(group) || ''}
                      </span>
                    </div>
                    <div className="app-execution-step-meta">
                      <span>assistantMessageId: {group.assistantMessageId}</span>
                      {group.parentMessageId ? <span>parent: {group.parentMessageId}</span> : null}
                    </div>
                    {(() => {
                      const step = normalizeModelStep(group);
                      const badges = payloadBadges(step);
                      const planned = plannedToolCalls(group);
                      if (badges.length === 0 && planned.length === 0) return null;
                      return (
                        <div className="app-execution-step-actions">
                          {badges.map((item) => (
                            <span key={item.key} className="app-execution-badge">{item.label}</span>
                          ))}
                          {planned.map((item, index) => (
                            <span key={`${firstString(item?.toolCallId, index)}:${firstString(item?.toolName)}`} className="app-execution-badge planned">
                              {`Planned ${firstString(item?.toolName, item?.ToolName, 'tool')}`}
                            </span>
                          ))}
                        </div>
                      );
                    })()}
                  </div>
                ) : null}
                {group.toolCalls.map((tool, index) => {
                  const step = normalizeToolStep(tool, group);
                  const badges = payloadBadges(step);
                  const hasLinkedConversation = !!firstString(step?.linkedConversationId);
                  const hasFile = collectFileUris(step?.responsePayload || {}).length > 0
                    || collectFileUris(step?.providerResponsePayload || {}).length > 0
                    || collectFileUris(step?.streamPayload || {}).length > 0;
                  return (
                  <div
                    key={`${group.assistantMessageId}:tool:${index}`}
                    role="button"
                    tabIndex={0}
                    className="app-execution-step-card tool is-clickable"
                    onClick={() => openStepDetail(step)}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        openStepDetail(step);
                      }
                    }}
                  >
                    <div className="app-execution-step-head">
                      <span>{`${displayStepIcon(step)} ${displayStepTitle(step)}`}</span>
                      <span>{firstString(tool?.status, tool?.Status, 'unknown')}</span>
                    </div>
                    <div className="app-execution-step-meta">
                      {firstString(tool?.responsePayloadId, tool?.ResponsePayloadId) ? (
                        <span>responsePayloadId: {firstString(tool?.responsePayloadId, tool?.ResponsePayloadId)}</span>
                      ) : null}
                      {firstString(tool?.traceId, tool?.TraceId) ? (
                        <span>traceId: {firstString(tool?.traceId, tool?.TraceId)}</span>
                      ) : null}
                    </div>
                    {(badges.length > 0 || hasLinkedConversation || hasFile) ? (
                      <div className="app-execution-step-actions">
                        {badges.map((item) => (
                          <span key={item.key} className="app-execution-badge">{item.label}</span>
                        ))}
                        {hasLinkedConversation ? (
                          <button type="button" className="app-execution-inline-button" onClick={(event) => openLinkedConversationFromStep(step, event)}>
                            Open Thread
                          </button>
                        ) : null}
                        {hasFile ? (
                          <button type="button" className="app-execution-inline-button" onClick={(event) => openFileFromStep(step, event)}>
                            Open File
                          </button>
                        ) : null}
                      </div>
                    ) : null}
                  </div>
                );})}
                {group.preamble ? <div className="app-execution-response preamble">{group.preamble}</div> : null}
                {group.content ? <div className="app-execution-response final">{group.content}</div> : null}
              </article>
            ))}
            {visibleGroups.length === 0 ? (
              <div className="app-execution-empty large">No execution pages yet. Select a conversation or submit a prompt.</div>
            ) : null}
          </div>
          {(transcriptMeta.pageCount > 1 || pageSize !== '1') ? (
            <div className="app-execution-footer">
              <div className="app-execution-paging">
                <button type="button" className="app-execution-inline-button" disabled={pageIndex >= Math.max(0, transcriptMeta.pageCount - 1)} onClick={goOlder}>Older</button>
                <button type="button" className="app-execution-inline-button" disabled={pageIndex === 0} onClick={goNewer}>Newer</button>
                <button type="button" className="app-execution-inline-button" disabled={pageIndex === 0} onClick={goLatest}>Latest</button>
              </div>
              <div className="app-execution-page-size">
                {PAGE_SIZE_OPTIONS.map((option) => (
                  <button
                    key={option}
                    type="button"
                    className={`app-execution-inline-button${pageSize === option ? ' is-active' : ''}`}
                    onClick={() => {
                      setPageSize(option);
                      setPageIndex(0);
                    }}
                  >
                    {option === 'all' ? 'All' : option}
                  </button>
                ))}
              </div>
            </div>
          ) : null}
        </div>

        <div className="app-execution-column timeline">
          <div className="app-execution-section-head">
            <span>Timeline Debug</span>
            <span className="app-execution-pill">{timeline.length}</span>
          </div>
          <div className="app-execution-timeline-list">
            {timeline.map((item, index) => (
              <div key={eventKey(item, index)} className={`app-execution-timeline-entry tone-${toneForStatus(item?.status || item?.type)}`}>
                <div className="app-execution-timeline-top">
                  <strong>{firstString(item?.type, 'event')}</strong>
                  <span>{relativeTime(firstString(item?.receivedAt, item?.createdAt))}</span>
                </div>
                <div className="app-execution-timeline-body">{describeTimelineEvent(item)}</div>
                <div className="app-execution-timeline-meta">
                  {firstString(item?.assistantMessageId) ? <span>assistant: {item.assistantMessageId}</span> : null}
                  {firstString(item?.toolCallId) ? <span>toolCall: {item.toolCallId}</span> : null}
                  {firstString(item?.turnId) ? <span>turn: {item.turnId}</span> : null}
                </div>
              </div>
            ))}
            {timeline.length === 0 ? <div className="app-execution-empty">Waiting for stream events.</div> : null}
          </div>
        </div>
      </div>
    </section>
  );
}
