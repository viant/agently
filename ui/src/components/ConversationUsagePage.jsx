import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Icon, Spinner } from '@blueprintjs/core';
import { useParams } from 'react-router-dom';
import { client } from '../services/agentlyClient';
import {
  formatTokenCount,
  formatUsageCost,
  summarizeConversationUsage,
} from '../services/conversationUsage';

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
          <span>{formatUsageCost(model.cost)}</span>
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

export default function ConversationUsagePage() {
  const { conversationId = '' } = useParams();
  const [conversation, setConversation] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const backHref = `/conversation/${encodeURIComponent(conversationId)}`;

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setConversation(await client.getConversation(conversationId));
    } catch (err) {
      setError(String(err?.message || err || 'Unable to load conversation usage.'));
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => { void load(); }, [load]);
  const summary = useMemo(() => summarizeConversationUsage(conversation || {}), [conversation]);
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
            <MetricCard icon="dollar" label="Estimated cost" value={formatUsageCost(summary.cost)} detail="Conversation total" tone="cost" />
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
        </div>
      ) : null}
    </main>
  );
}
