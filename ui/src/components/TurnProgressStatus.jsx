import React from 'react';
import { Button, Popover, Spinner, Tooltip } from '@blueprintjs/core';
import {
  resolveActiveTurnProgress,
  summarizeExecutionTokenUsage,
} from 'agently-core-ui-sdk';
import { useChatProjection } from '../services/chatStore';
import { client } from '../services/agentlyClient';
import { getUsage, onUsageChange } from '../services/usageBus';
import { setStage, useStage } from '../services/stageBus';

const NUMBER = new Intl.NumberFormat();

const ACTIVITY_LABELS = {
  connecting: 'Connecting',
  planning: 'Planning',
  thinking: 'Thinking',
  writing: 'Writing response',
  tool: 'Using tool',
  tools: 'Calling tools',
  stopping: 'Stopping',
  waiting_for_user: 'Needs your input',
};

export function activeIteration(rows = []) {
  return [...(Array.isArray(rows) ? rows : [])]
    .reverse()
    .find((row) => row?.kind === 'iteration' && (row?.lifecycle === 'pending' || row?.lifecycle === 'running')) || null;
}

export function groupsFromRow(row = null) {
  if (!row) return [];
  return (Array.isArray(row?.rounds) ? row.rounds : []).map((round) => ({
    turnId: row.turnId,
    pageId: round?.pageId || round?.renderKey || '',
    assistantMessageId: round?.modelSteps?.[0]?.assistantMessageId || round?.pageId || '',
    phase: round?.phase || '',
    narration: round?.narration || '',
    content: round?.content || '',
    status: round?.status || row?.lifecycle || '',
    finalResponse: !!round?.finalResponse,
    modelSteps: Array.isArray(round?.modelSteps) ? round.modelSteps : [],
    toolSteps: Array.isArray(round?.toolCalls) ? round.toolCalls : [],
    toolCallsPlanned: Array.isArray(round?.toolCallsPlanned) ? round.toolCallsPlanned : [],
  }));
}

export function conversationTokenFallback(usage = null) {
  if (!usage || Number(usage?.totalTokens || 0) <= 0) return undefined;
  return {
    scope: 'conversation',
    totalTokens: Number(usage.totalTokens || 0),
    inputTokens: Number(usage.promptTokens || 0),
    outputTokens: Number(usage.completionTokens || 0),
    cachedInputTokens: Number(usage.promptCachedTokens || 0),
  };
}

export function toolProgressText(progress) {
  if (!progress?.identityComplete || progress.totalToolCount <= 0) return '';
  const parts = [`${progress.completedToolCount}/${progress.totalToolCount} done`];
  if (progress.activeToolCount > 0) parts.push(`${progress.activeToolCount} active`);
  if (progress.queuedToolCount > 0) parts.push(`${progress.queuedToolCount} queued`);
  if (progress.failedToolCount > 0) parts.push(`${progress.failedToolCount} failed`);
  return parts.join(' · ');
}

function statusLabel(value = '') {
  return String(value || 'unknown').trim().replace(/_/g, ' ');
}

function ToolDetails({ progress }) {
  return (
    <div className="app-turn-progress-popover" data-testid="turn-progress-tool-details">
      <div className="app-turn-progress-popover-title">Tool progress</div>
      {progress.rows.length > 0 ? progress.rows.map((row) => (
        <div className="app-turn-progress-detail-row" key={row.toolCallId}>
          <span>{row.toolName}</span>
          <span>{statusLabel(row.status)}</span>
        </div>
      )) : <div className="app-turn-progress-empty">Tool identities are not available yet.</div>}
    </div>
  );
}

export function TokenDetails({ usage }) {
  const rows = [
    ['Total', usage.totalTokens],
    ['Input', usage.inputTokens],
    ['Output', usage.outputTokens],
    ['Cached input', usage.cachedInputTokens],
    ['Reasoning', usage.reasoningTokens],
    ['Embedding', usage.embeddingTokens],
  ];
  const tokenValue = (value) => Number.isFinite(Number(value)) ? NUMBER.format(Number(value)) : 'Not reported';
  return (
    <div className="app-turn-progress-popover app-turn-progress-token-popover" data-testid="turn-progress-token-details">
      <div className="app-turn-progress-popover-title">{usage.scope === 'turn' ? 'This turn' : 'Conversation total'}</div>
      {rows.map(([label, value]) => (
        <div className="app-turn-progress-detail-row" key={label}>
          <span>{label}</span><strong>{tokenValue(value)}</strong>
        </div>
      ))}
      {Array.isArray(usage.models) && usage.models.length > 0 ? (
        <div className="app-turn-progress-models">
          {usage.models.map((model) => (
            <div className="app-turn-progress-model" key={model.modelCallId || `${model.provider}/${model.model}`}>
              <div className="app-turn-progress-model-title">{[model.provider, model.model].filter(Boolean).join('/') || 'Model'}</div>
              {[
                ['Total', model.totalTokens],
                ['Input', model.inputTokens],
                ['Output', model.outputTokens],
                ['Cached input', model.cachedInputTokens],
                ['Reasoning', model.reasoningTokens],
                ['Embedding', model.embeddingTokens],
              ].map(([label, value]) => (
                <div className="app-turn-progress-model-row" key={label}>
                  <span>{label}</span><strong>{tokenValue(value)}</strong>
                </div>
              ))}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function tokenHoverText(usage) {
  return [
    Number(usage?.inputTokens || 0) > 0 ? `Input ${NUMBER.format(usage.inputTokens)}` : '',
    Number(usage?.outputTokens || 0) > 0 ? `Output ${NUMBER.format(usage.outputTokens)}` : '',
    Number(usage?.cachedInputTokens || 0) > 0 ? `Cached ${NUMBER.format(usage.cachedInputTokens)}` : '',
    Array.isArray(usage?.models) && usage.models.length > 0 ? `${usage.models.length} model${usage.models.length === 1 ? '' : 's'}` : '',
  ].filter(Boolean).join(' · ') || 'Show token usage breakdown';
}

export default function TurnProgressStatus({ conversationId = '', developerMode = false }) {
  const rows = useChatProjection(conversationId);
  const stage = useStage();
  const [usage, setUsage] = React.useState(getUsage);
  const [stopping, setStopping] = React.useState(false);

  React.useEffect(() => onUsageChange(() => setUsage(getUsage())), []);

  const row = React.useMemo(() => activeIteration(rows), [rows]);
  const groups = React.useMemo(() => groupsFromRow(row), [row]);
  const turnUsage = React.useMemo(() => summarizeExecutionTokenUsage(groups), [groups]);
  const fallbackUsage = String(usage?.conversationId || '') === String(conversationId || '')
    ? conversationTokenFallback(usage)
    : undefined;
  // Stage is process-global and is also used for workspace/bootstrap loading.
  // Summary progress represents an actual conversation turn, so never infer a
  // turn from the stage alone. This prevents failed/default workspace startup
  // from looking like an assistant is actively working.
  if (!String(conversationId || '').trim() || !row || !String(row?.turnId || '').trim()) return null;
  const latestRound = groups[groups.length - 1] || {};
  const status = stopping
    ? 'stopping'
    : String(latestRound?.status || row?.lifecycle || (stage?.phase === 'waiting' ? 'waiting_for_user' : '')).trim();
  const progress = resolveActiveTurnProgress({
    turnId: row?.turnId || '',
    status,
    phase: latestRound?.phase || stage?.phase || '',
    isSending: !!row || ['thinking', 'executing', 'streaming', 'waiting'].includes(String(stage?.phase || '')),
    isStopping: stopping,
    startedAt: row?.turnStartedAt || (stage?.startedAt ? new Date(stage.startedAt).toISOString() : ''),
    groups,
    tokenUsage: turnUsage || fallbackUsage,
    assistantHasContent: groups.some((group) => String(group?.content || '').trim() !== ''),
  });

  if (developerMode || !progress) return null;

  const activity = progress.activity.label || ACTIVITY_LABELS[progress.activity.kind] || 'Working';
  const toolText = toolProgressText(progress);
  const tokenText = progress.tokenUsage?.totalTokens > 0
    ? `${NUMBER.format(progress.tokenUsage.totalTokens)} ${progress.tokenUsage.scope === 'turn' ? 'turn ' : 'total '}tokens`
    : '';
  const waiting = progress.state === 'waiting_for_user';

  const stop = async () => {
    if (!progress.turnId || stopping) return;
    setStopping(true);
    setStage({ phase: 'executing', text: 'Stopping request…' });
    try {
      await client.cancelTurn(progress.turnId);
    } catch (error) {
      setStopping(false);
      setStage({ phase: 'error', text: String(error?.message || 'Could not stop request') });
    }
  };

  return (
    <section className={`app-turn-progress${waiting ? ' is-waiting' : ''}`} data-testid="turn-progress-status" aria-live="polite">
      <div className="app-turn-progress-spinner" aria-hidden="true">
        {waiting ? <span>!</span> : <Spinner size={18} />}
      </div>
      <div className="app-turn-progress-content">
        <div className="app-turn-progress-title">{waiting ? 'Needs your input' : 'Working on your request'}</div>
        <div className="app-turn-progress-chips">
          <span className="app-turn-progress-chip is-activity">{activity}</span>
          {toolText ? (
            <Popover content={<ToolDetails progress={progress} />} placement="bottom-start" minimal>
              <Tooltip content={<ToolDetails progress={progress} />} placement="bottom">
                <button type="button" className={`app-turn-progress-chip is-button${progress.failedToolCount > 0 ? ' has-failure' : ''}`}>{toolText}</button>
              </Tooltip>
            </Popover>
          ) : (progress.totalToolCount > 0 || !progress.identityComplete) ? <span className="app-turn-progress-chip">Calling tools</span> : null}
          {tokenText ? (
            <Popover content={<TokenDetails usage={progress.tokenUsage} />} placement="bottom-start" minimal>
              <Tooltip content={tokenHoverText(progress.tokenUsage)} placement="bottom">
                <button type="button" className="app-turn-progress-chip is-button is-tokens">{tokenText}</button>
              </Tooltip>
            </Popover>
          ) : null}
        </div>
      </div>
      {progress.canStop ? <Button minimal icon="stop" aria-label="Stop current request" onClick={stop} disabled={stopping} /> : null}
    </section>
  );
}
