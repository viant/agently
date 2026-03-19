import React from 'react';
import { Button, Tag } from '@blueprintjs/core';
import {
  PLAN_FEED_ANCHORS,
  setPlanFeedAnchor,
  usePlanFeed
} from '../services/planFeedBus';

const ANCHOR_OPTIONS = [
  { value: PLAN_FEED_ANCHORS.COMPOSER_TOP, label: 'Above composer' },
  { value: PLAN_FEED_ANCHORS.SIDEBAR_TOP, label: 'Sidebar top' },
  { value: PLAN_FEED_ANCHORS.SIDEBAR_BOTTOM, label: 'Sidebar bottom' }
];

function statusIntent(status = '') {
  switch (String(status || '').trim().toLowerCase()) {
    case 'completed':
    case 'done':
    case 'success':
      return 'success';
    case 'in_progress':
    case 'running':
      return 'primary';
    default:
      return 'none';
  }
}

function statusLabel(status = '') {
  const text = String(status || '').trim();
  return text ? text.replace(/_/g, ' ') : 'pending';
}

export default function PlanFeed({ anchor = PLAN_FEED_ANCHORS.COMPOSER_TOP, compact = false }) {
  const plan = usePlanFeed();
  const selectedAnchor = String(plan?.anchor || PLAN_FEED_ANCHORS.COMPOSER_TOP);
  const steps = Array.isArray(plan?.steps) ? plan.steps : [];
  const explanation = String(plan?.explanation || '').trim();
  const completedCount = steps.filter((step) => String(step?.status || '').trim().toLowerCase() === 'completed').length;
  const isComposerTop = anchor === PLAN_FEED_ANCHORS.COMPOSER_TOP;
  if (selectedAnchor !== anchor) return null;
  if (!explanation && steps.length === 0) return null;

  return (
    <section
      className={`app-plan-feed app-plan-feed-${anchor}${compact ? ' is-compact' : ''}`}
      data-testid={`plan-feed-${anchor}`}
    >
      <div className="app-plan-feed-head">
        <div className="app-plan-feed-title-wrap">
          <div className="app-plan-feed-title">
            {isComposerTop ? `${completedCount} out of ${steps.length} tasks completed` : 'Plan feed'}
          </div>
          {!isComposerTop ? <div className="app-plan-feed-subtitle">Anchor: <code>{selectedAnchor}</code></div> : null}
        </div>
        {!isComposerTop ? (
          <div className="app-plan-feed-anchor-switch">
            {ANCHOR_OPTIONS.map((option) => (
              <Button
                key={option.value}
                minimal
                small
                className={`app-plan-feed-anchor-btn${selectedAnchor === option.value ? ' is-active' : ''}`}
                onClick={() => setPlanFeedAnchor(option.value)}
              >
                {option.label}
              </Button>
            ))}
          </div>
        ) : (
          <button
            type="button"
            className="app-plan-feed-dock-action"
            onClick={() => setPlanFeedAnchor(PLAN_FEED_ANCHORS.SIDEBAR_TOP)}
            aria-label="Move plan feed to sidebar"
            title="Move plan feed to sidebar"
          >
            ↗
          </button>
        )}
      </div>

      {explanation ? <div className="app-plan-feed-explanation">{explanation}</div> : null}

      {steps.length > 0 ? (
        <div className="app-plan-feed-steps">
          {steps.map((step, index) => (
            <div className="app-plan-feed-step" key={step?.id || `${index}:${step?.step || 'step'}`}>
              <span className="app-plan-feed-step-index">{String(step?.status || '').trim().toLowerCase() === 'completed' ? '✓' : index + 1}</span>
              <span className="app-plan-feed-step-copy">{String(step?.step || '').trim()}</span>
              <Tag minimal intent={statusIntent(step?.status)}>{statusLabel(step?.status)}</Tag>
            </div>
          ))}
        </div>
      ) : null}
    </section>
  );
}
