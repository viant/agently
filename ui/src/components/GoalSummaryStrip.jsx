import React, { useEffect, useState } from 'react';
import { getFeedData, onFeedDataChange } from '../services/toolFeedBus';
import { client } from '../services/agentlyClient';
import { refreshGoalFeed } from '../services/chatRuntime';
import { showToast } from '../services/httpClient';

function readGoalPayload(feedData) {
  if (!feedData || typeof feedData !== 'object') return { goal: null, controllerSchedule: null };
  const direct = feedData?.data?.goal;
  const directSchedule = feedData?.data?.controllerSchedule;
  if (direct && typeof direct === 'object') {
    return {
      goal: direct,
      controllerSchedule: directSchedule && typeof directSchedule === 'object'
        ? directSchedule
        : (direct.controllerSchedule && typeof direct.controllerSchedule === 'object' ? direct.controllerSchedule : null),
    };
  }
  const nested = feedData?.data?.data?.goal;
  const nestedSchedule = feedData?.data?.data?.controllerSchedule;
  if (nested && typeof nested === 'object') {
    return {
      goal: nested,
      controllerSchedule: nestedSchedule && typeof nestedSchedule === 'object'
        ? nestedSchedule
        : (nested.controllerSchedule && typeof nested.controllerSchedule === 'object' ? nested.controllerSchedule : null),
    };
  }
  return { goal: null, controllerSchedule: null };
}

function normalizeStatus(status = '') {
  return String(status || '').trim().toLowerCase();
}

function formatStatus(status = '') {
  const normalized = normalizeStatus(status);
  if (!normalized) return 'Unknown';
  return normalized.replace(/_/g, ' ').replace(/\b\w/g, (char) => char.toUpperCase());
}

function statusTone(status = '') {
  switch (normalizeStatus(status)) {
    case 'active':
      return 'active';
    case 'paused':
      return 'paused';
    case 'blocked':
    case 'budget_limited':
    case 'usage_limited':
      return 'danger';
    case 'complete':
      return 'complete';
    default:
      return 'neutral';
  }
}

function formatTime(value = 0) {
  const seconds = Math.max(0, Number(value || 0) || 0);
  if (seconds < 60) return `${seconds}s`;
  const mins = Math.floor(seconds / 60);
  const hours = Math.floor(mins / 60);
  if (hours > 0) return `${hours}h ${mins % 60}m`;
  return `${mins}m`;
}

function formatWakeAt(value = '') {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  });
}

export default function GoalSummaryStrip({ conversationId = '', onOpenGoalPanel = null, onOpenGoalDraft = null }) {
  const scopedConversationId = String(conversationId || '').trim();
  const [version, setVersion] = useState(0);
  const [busy, setBusy] = useState('');

  useEffect(() => onFeedDataChange(() => setVersion((value) => value + 1)), []);

  if (!scopedConversationId) return null;
  const _version = version;
  void _version;
  const feedData = getFeedData('goal', scopedConversationId);
  if (!feedData) return null;

  const { goal, controllerSchedule } = readGoalPayload(feedData);
  const status = String(goal?.status || '').trim();
  const objective = String(goal?.objective || '').trim();
  const hasGoal = !!goal;
  const statusText = formatStatus(status);
  const tone = statusTone(status);
  const tokenBudget = Number(goal?.tokenBudget || 0) || 0;
  const tokensUsed = Number(goal?.tokensUsed || 0) || 0;
  const timeUsedSeconds = Number(goal?.timeUsedSeconds || 0) || 0;
  const budgetPct = tokenBudget > 0 ? Math.min(100, Math.round((tokensUsed / tokenBudget) * 100)) : 0;
  const reason = String(goal?.statusReason || goal?.pauseReason || '').trim();
  const controllerMode = String(controllerSchedule?.mode || '').trim().toLowerCase();
  const wakeAtLabel = formatWakeAt(controllerSchedule?.wakeAt);
  const scheduleLabel = controllerMode === 'wakeup'
    ? (wakeAtLabel ? `Scheduled to resume ${wakeAtLabel}` : 'Scheduled to resume later')
    : (controllerMode === 'queue' ? 'Next autonomous step is queued' : '');

  const runQuickAction = async (nextStatus) => {
    if (!scopedConversationId) return;
    setBusy(nextStatus);
    try {
      await client.updateGoal(scopedConversationId, { status: nextStatus });
      await refreshGoalFeed(scopedConversationId);
    } catch (err) {
      showToast(String(err?.message || err || 'Goal action failed'), { intent: 'danger' });
    } finally {
      setBusy('');
    }
  };

  return (
    <section className="app-goal-summary-strip" data-testid="goal-summary-strip">
      <div className="app-goal-summary-strip-copy">
        <div className="app-goal-summary-strip-eyebrow">Goal</div>
        {hasGoal ? (
          <>
            <div className="app-goal-summary-strip-heading">
              <div className="app-goal-summary-strip-title">{objective || '(no objective)'}</div>
              <span className={`app-goal-status-pill tone-${tone}`}>{statusText}</span>
            </div>
            <div className="app-goal-summary-strip-meta">
              <span>{tokenBudget > 0 ? `${tokensUsed}/${tokenBudget} tokens` : `${tokensUsed} tokens`}</span>
              <span>{formatTime(timeUsedSeconds)}</span>
            </div>
            {tokenBudget > 0 ? (
              <div className="app-goal-summary-strip-budget">
                <div className="app-goal-summary-strip-budget-bar">
                  <span className={`app-goal-summary-strip-budget-fill tone-${tone}`} style={{ width: `${budgetPct}%` }} />
                </div>
              </div>
            ) : null}
            {scheduleLabel ? <div className="app-goal-summary-strip-schedule">{scheduleLabel}</div> : null}
            {reason ? <div className="app-goal-summary-strip-reason">{reason}</div> : null}
          </>
        ) : (
          <>
            <div className="app-goal-summary-strip-title">No goal set</div>
            <div className="app-goal-summary-strip-meta">Create one for this conversation.</div>
          </>
        )}
      </div>
      <div className="app-goal-summary-strip-actions">
        {hasGoal && normalizeStatus(status) === 'active' ? (
          <button
            type="button"
            className="app-goal-summary-strip-quick"
            disabled={busy !== ''}
            onClick={() => runQuickAction('paused')}
          >
            Pause
          </button>
        ) : null}
        {hasGoal && normalizeStatus(status) === 'paused' ? (
          <button
            type="button"
            className="app-goal-summary-strip-quick"
            disabled={busy !== ''}
            onClick={() => runQuickAction('active')}
          >
            Resume
          </button>
        ) : null}
        <button
          type="button"
          className="app-goal-summary-strip-action"
          onClick={() => {
            if (hasGoal) {
              onOpenGoalPanel?.();
            } else {
              onOpenGoalDraft?.();
            }
          }}
        >
          {hasGoal ? 'Manage goal' : 'Set goal'}
        </button>
      </div>
    </section>
  );
}
