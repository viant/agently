import React from 'react';
import { Dialog } from '@blueprintjs/core';
import { client } from '../services/agentlyClient';
import { refreshGoalFeed } from '../services/chatRuntime';
import { showToast } from '../services/httpClient';

const GOAL_TEMPLATES = [
  {
    id: 'outcome',
    label: 'Outcome',
    summary: 'Outcome-first autonomous goal',
    template: `/goal <desired end state> verified by <specific evidence>\nwhile preserving <constraints>.\nUse <allowed inputs, tools, or boundaries>.\nBetween iterations, <how Codex should choose the next best action>.\nIf blocked or no valid paths remain, <what Codex should report and what would unlock progress>.`,
  },
  {
    id: 'verification',
    label: 'Verification',
    summary: 'Lead with concrete proof of done',
    template: `/goal <desired end state>.\nVerified by <specific evidence or command output> while preserving <constraints>.\nUse <allowed inputs, tools, or boundaries>.\nBetween iterations, pick the next best action that increases verification confidence.\nIf blocked or no valid paths remain, report the blocker and the smallest thing that would unlock progress.`,
  },
  {
    id: 'constraints',
    label: 'Constraints',
    summary: 'Emphasize guardrails and safe boundaries',
    template: `/goal <desired end state> while preserving <constraints>.\nVerified by <specific evidence>.\nUse <allowed inputs, tools, or boundaries>.\nBetween iterations, prefer the highest-signal action that stays inside those constraints.\nIf blocked or no valid paths remain, report the blocker, rejected paths, and what would unlock forward motion.`,
  },
];

export default function GoalDraftDialog({ isOpen = false, conversationId = '', initialDraft = '', onClose = null, onCreated = null }) {
  const [selectedTemplate, setSelectedTemplate] = React.useState(GOAL_TEMPLATES[0].id);
  const [draft, setDraft] = React.useState(GOAL_TEMPLATES[0].template);
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    if (!isOpen) return;
    const incoming = String(initialDraft || '').trim();
    if (incoming) {
      setDraft(incoming);
      return;
    }
    const template = GOAL_TEMPLATES.find((item) => item.id === selectedTemplate) || GOAL_TEMPLATES[0];
    setDraft(template.template);
  }, [initialDraft, isOpen, selectedTemplate]);

  const save = async () => {
    const id = String(conversationId || '').trim();
    const objective = String(draft || '').trim();
    if (!id || !objective) return;
    setSaving(true);
    try {
      try {
        await client.createGoal(id, { objective });
      } catch (err) {
        if (!/already exists/i.test(String(err?.message || err || ''))) throw err;
        await client.updateGoal(id, { objective });
      }
      await refreshGoalFeed(id);
      showToast('Autonomous goal created.', { intent: 'success' });
      onCreated?.(objective);
      onClose?.();
    } catch (err) {
      showToast(String(err?.message || err || 'Goal creation failed'), { intent: 'danger' });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog
      isOpen={isOpen}
      onClose={() => onClose?.()}
      title="Autonomous Goal"
      style={{ width: 'min(860px, 92vw)' }}
    >
      <div className="app-goal-draft-shell">
        <div className="app-goal-draft-header">
          <div className="app-goal-draft-title">Create A Structured Autonomous Goal</div>
          <div className="app-goal-draft-copy">
            Start from a strong template so the agent knows the end state, evidence of done, constraints,
            allowed tools, and what to report if progress stalls.
          </div>
        </div>

        <div className="app-goal-draft-template-list">
          {GOAL_TEMPLATES.map((template) => (
            <button
              key={template.id}
              type="button"
              className={`app-goal-draft-template${template.id === selectedTemplate ? ' is-selected' : ''}`}
              onClick={() => setSelectedTemplate(template.id)}
            >
              <span className="app-goal-draft-template-label">{template.label}</span>
              <span className="app-goal-draft-template-summary">{template.summary}</span>
            </button>
          ))}
        </div>

        <div className="app-goal-draft-editor">
          <label className="app-goal-draft-label" htmlFor="goal-draft-textarea">Goal draft</label>
          <textarea
            id="goal-draft-textarea"
            className="app-goal-draft-textarea"
            value={draft}
            onChange={(event) => setDraft(String(event?.target?.value || ''))}
            rows={9}
          />
        </div>

        <div className="app-goal-draft-footer">
          <button type="button" className="app-goal-draft-secondary" onClick={() => onClose?.()} disabled={saving}>
            Cancel
          </button>
          <button type="button" className="app-goal-draft-primary" onClick={save} disabled={saving || !String(draft || '').trim()}>
            {saving ? 'Creating...' : 'Create goal'}
          </button>
        </div>
      </div>
    </Dialog>
  );
}
