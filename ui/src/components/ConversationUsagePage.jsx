import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Icon, Spinner } from '@blueprintjs/core';
import { useParams } from 'react-router-dom';
import { client } from '../services/agentlyClient';
import {
  formatTokenCount,
  formatUsageCost,
  summarizeConversationUsage,
} from '../services/conversationUsage';
import {
  formatUsdEstimate,
  summarizeTranscriptToolUsage,
} from '../services/tokenUsageEstimation';
import { readConversationProjectionUsage } from '../services/usageProjectionStore';

function MetricCard({ icon, label, value, detail, tone }) {
  return (
    <article className={`conversation-usage-metric is-${tone}`}>
      <span className="conversation-usage-metric-icon"><Icon icon={icon} size={17} /></span>
      <div>
        <span className="conversation-usage-metric-label">{label}</span>
        <strong>{value}</strong>
        {detail ? <small>{detail}</small> : null}
      </div>
    </article>
  );
}

function usageRoleLabel(value = '') {
  const role = String(value || '').trim().toLowerCase();
  if (role === 'react' || role === 'main') return 'Main';
  if (role === 'intake') return 'Intake';
  if (role === 'sidecar') return 'Sidecar';
  if (role === 'narrator') return 'Narrator';
  if (role === 'summary') return 'Summary';
  return role ? role.charAt(0).toUpperCase() + role.slice(1) : 'Main';
}

function ModelRow({ model, overallTokens }) {
  const inputPct = model.totalTokens > 0 ? (model.inputTokens / model.totalTokens) * 100 : 0;
  const outputPct = model.totalTokens > 0 ? (model.outputTokens / model.totalTokens) * 100 : 0;
  const share = overallTokens > 0 ? Math.round((model.totalTokens / overallTokens) * 100) : 0;
  const role = usageRoleLabel(model.executionRole);
  return (
    <article className="conversation-usage-model">
      <div className="conversation-usage-model-heading">
        <div>
          <div className="conversation-usage-model-title">
            <strong>{model.model}</strong>
            <span className={`conversation-usage-role is-${String(model.executionRole || 'react').toLowerCase()}`}>{role}</span>
          </div>
          <span>{model.provider || 'Model'} · {share}% of conversation</span>
        </div>
        <div className="conversation-usage-model-total">
          <strong>{formatTokenCount(model.totalTokens)}</strong>
          <span>{model.costEstimated ? `Est. ${formatUsageCost(model.cost)}` : formatUsageCost(model.cost)}</span>
        </div>
      </div>
      <div className="conversation-usage-model-bar" aria-label={`${model.model} token distribution`}>
        <span className="is-input" style={{ width: `${inputPct}%` }} />
        <span className="is-output" style={{ width: `${outputPct}%` }} />
      </div>
      <div className="conversation-usage-model-breakdown">
        <span><i className="is-input" />Input {formatTokenCount(model.inputTokens)}</span>
        <span><i className="is-output" />Output {formatTokenCount(model.outputTokens)}</span>
        {model.cachedInputTokens > 0 ? <span>Cached {formatTokenCount(model.cachedInputTokens)}</span> : null}
      </div>
    </article>
  );
}

function formatBytes(value) {
  const bytes = Math.max(0, Number(value) || 0);
  if (bytes < 1024) return `${Math.trunc(bytes)} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function ToolUsageDirection({ label, usage }) {
  if (!usage?.available) {
    return (
      <div className="conversation-usage-tool-direction is-unavailable">
        <span>{label}</span>
        <strong>{usage?.compressed ? 'Compressed payload unavailable' : 'Not captured'}</strong>
      </div>
    );
  }
  return (
    <div className="conversation-usage-tool-direction">
      <span>{label}</span>
      <strong>{`≈${formatTokenCount(usage.tokens)} ${usage.tokenDirection} tokens`}</strong>
      <small>{formatBytes(usage.bytes)}{usage.cost != null ? ` · ${formatUsdEstimate(usage.cost)}` : ''}</small>
    </div>
  );
}

function ToolUsageRow({ call }) {
  const overflow = call?.toolOutput?.overflow || {};
  const outputModelLabel = [call?.toolInput?.pricingProvider, call?.toolInput?.pricingModel].filter(Boolean).join('/');
  const inputModelLabel = [call?.toolOutput?.pricingProvider, call?.toolOutput?.pricingModel].filter(Boolean).join('/');
  const modelLabel = outputModelLabel && inputModelLabel && outputModelLabel !== inputModelLabel
    ? `${outputModelLabel} → ${inputModelLabel}`
    : (outputModelLabel || inputModelLabel);
  return (
    <article className={`conversation-usage-tool${overflow.overflow ? ' has-overflow' : ''}`}>
      <div className="conversation-usage-tool-heading">
        <div>
          <strong>{call.toolName}</strong>
          <span>{modelLabel || 'Pricing unavailable'} · {call.status || 'unknown'}</span>
        </div>
        <div className="conversation-usage-tool-total">
          <strong>≈{formatTokenCount(call.totalTokens)}</strong>
          <span>{call.totalCost == null ? 'Unpriced' : formatUsdEstimate(call.totalCost)}</span>
        </div>
      </div>
      <div className="conversation-usage-tool-directions">
        <ToolUsageDirection label="Arguments generated" usage={call.toolInput} />
        <ToolUsageDirection label="Result presented" usage={call.toolOutput} />
      </div>
      {overflow.overflow ? (
        <div className="conversation-usage-overflow-note">
          <Icon icon="warning-sign" size={13} />
          <span>
            Output overflowed; the estimate covers only the result presented to the model.
            {overflow.remaining != null ? ` ${formatTokenCount(overflow.remaining)} units remain.` : ''}
            {overflow.nextRange?.length ? ` Next range: ${formatBytes(overflow.nextRange.length)}.` : ''}
          </span>
        </div>
      ) : null}
    </article>
  );
}

export default function ConversationUsagePage() {
  const { conversationId = '' } = useParams();
  const [conversation, setConversation] = useState(null);
  const [transcript, setTranscript] = useState(null);
  const [projectionUsage, setProjectionUsage] = useState({ entries: [], tokensFreed: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const backHref = `/conversation/${encodeURIComponent(conversationId)}`;

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const [nextConversation, nextTranscript] = await Promise.all([
        client.getConversation(conversationId),
        client.getTranscript({
          conversationId,
          includeModelCalls: true,
          includeToolCalls: true,
        }).catch(() => null),
      ]);
      setConversation(nextConversation);
      setTranscript(nextTranscript);
      setProjectionUsage(readConversationProjectionUsage(conversationId));
    } catch (err) {
      setError(String(err?.message || err || 'Unable to load conversation usage.'));
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => { void load(); }, [load]);
  const summary = useMemo(() => summarizeConversationUsage(conversation || {}), [conversation]);
  const toolUsage = useMemo(() => summarizeTranscriptToolUsage(transcript || {}, {
    projectionTokensFreed: projectionUsage.tokensFreed,
  }), [projectionUsage.tokensFreed, transcript]);
  const cachedPct = summary.inputTokens > 0 ? Math.round((summary.cachedInputTokens / summary.inputTokens) * 100) : 0;

  return (
    <main className="conversation-usage-page" data-testid="conversation-usage-page">
      <header className="conversation-usage-header">
        <a href={backHref} className="conversation-usage-back" aria-label="Back to conversation"><Icon icon="arrow-left" /> Conversation</a>
        <Button minimal icon="refresh" aria-label="Refresh usage" onClick={load} loading={loading && !!conversation} />
      </header>
      {loading && !conversation ? <div className="conversation-usage-loading"><Spinner size={28} /><span>Loading usage…</span></div> : null}
      {error ? (
        <section className="conversation-usage-error">
          <Icon icon="error" size={22} />
          <div><strong>Usage is unavailable</strong><p>{error}</p></div>
          <Button small text="Try again" onClick={load} />
        </section>
      ) : null}
      {!loading && !error ? (
        <div className="conversation-usage-content">
          <section className="conversation-usage-hero">
            <span className="conversation-usage-eyebrow"><Icon icon="dashboard" /> Conversation usage</span>
            <h1 title={summary.title}>{summary.title}</h1>
            <p>A complete token and cost summary for this conversation.</p>
            {summary.updatedAt ? <time>Updated {new Date(summary.updatedAt).toLocaleString()}</time> : null}
          </section>

          <section className="conversation-usage-metrics" aria-label="Usage summary">
            <MetricCard icon="chart" label="Total tokens" value={formatTokenCount(summary.totalTokens)} detail={`${summary.models.length || 1} model role${summary.models.length === 1 ? '' : 's'}`} tone="total" />
            <MetricCard icon="log-in" label="Input" value={formatTokenCount(summary.inputTokens)} detail={summary.cachedInputTokens > 0 ? `${cachedPct}% cached` : 'Prompt and context'} tone="input" />
            <MetricCard icon="log-out" label="Output" value={formatTokenCount(summary.outputTokens)} detail={summary.reasoningTokens > 0 ? `${formatTokenCount(summary.reasoningTokens)} reasoning` : 'Generated response'} tone="output" />
            <MetricCard icon="dollar" label="Estimated cost" value={formatUsageCost(summary.cost)} detail={summary.costEstimated ? 'Computed from model pricing' : 'Conversation total'} tone="cost" />
          </section>

          <section className="conversation-usage-panel">
            <div className="conversation-usage-panel-heading">
              <div><span>Breakdown</span><h2>Usage by model</h2></div>
              <div className="conversation-usage-legend"><span><i className="is-input" />Input</span><span><i className="is-output" />Output</span></div>
            </div>
            {summary.models.length > 0 ? summary.models.map((model) => (
              <ModelRow key={model.id} model={model} overallTokens={summary.totalTokens} />
            )) : (
              <div className="conversation-usage-empty">Per-model usage has not been reported for this conversation.</div>
            )}
          </section>

          <section className="conversation-usage-panel conversation-usage-tool-panel">
            <div className="conversation-usage-panel-heading">
              <div><span>Attribution estimate</span><h2>Usage by tool call</h2></div>
              <div className="conversation-usage-tool-summary">
                <strong>≈{formatTokenCount(toolUsage.totalTokens)} tokens</strong>
                <span>{toolUsage.totalCost == null ? 'Pricing unavailable' : `${formatUsdEstimate(toolUsage.totalCost)}${toolUsage.costPartial ? ' partial' : ''}`}</span>
              </div>
            </div>
            <div className="conversation-usage-attribution-note">
              Tool arguments are estimated as model output; tool results are estimated as model input at {toolUsage.bytesPerToken} UTF-8 bytes/token. These values attribute portions of provider-reported usage and are not added to billed totals.
            </div>
            {toolUsage.overflowCallCount > 0 || toolUsage.projectionTokensFreed > 0 || toolUsage.unavailablePayloadCount > 0 || toolUsage.unpricedPayloadCount > 0 ? (
              <div className="conversation-usage-overflow-summary">
                {toolUsage.overflowCallCount > 0 ? <span><Icon icon="warning-sign" size={13} />{toolUsage.overflowCallCount} overflowed tool result{toolUsage.overflowCallCount === 1 ? '' : 's'}</span> : null}
                {toolUsage.projectionTokensFreed > 0 ? <span><Icon icon="filter" size={13} />≈{formatTokenCount(toolUsage.projectionTokensFreed)} context tokens freed</span> : null}
                {toolUsage.unavailablePayloadCount > 0 ? <span><Icon icon="database" size={13} />{toolUsage.unavailablePayloadCount} unavailable payload{toolUsage.unavailablePayloadCount === 1 ? '' : 's'} excluded</span> : null}
                {toolUsage.unpricedPayloadCount > 0 ? <span><Icon icon="dollar" size={13} />{toolUsage.unpricedPayloadCount} payload{toolUsage.unpricedPayloadCount === 1 ? '' : 's'} lack model pricing</span> : null}
              </div>
            ) : null}
            {toolUsage.calls.length > 0 ? toolUsage.calls.map((call, index) => (
              <ToolUsageRow key={`${call.id}:${index}`} call={call} />
            )) : (
              <div className="conversation-usage-empty">No tool-call payload usage was found in this conversation.</div>
            )}
          </section>
        </div>
      ) : null}
    </main>
  );
}
