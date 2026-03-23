import React from 'react';

const ICON_LABELS = {
  bug: '!',
  chat: '?',
  document: 'D',
  flask: 'T',
  help: 'i',
  pencil: '+',
  rocket: '>',
  'shield-warning': '!',
  'tree-structure': '#'
};

function starterIcon(task = {}) {
  const key = String(task?.icon || '').trim().toLowerCase();
  return ICON_LABELS[key] || '*';
}

export default function StarterTasks({ message, context }) {
  const tasks = Array.isArray(message?.starterTasks) ? message.starterTasks : [];
  const title = String(message?.title || 'Start with an agent prompt').trim();
  const subtitle = String(message?.subtitle || '').trim();

  if (tasks.length === 0) return null;

  const onSelectTask = (task) => {
    const prompt = String(task?.prompt || '').trim();
    if (!prompt || typeof document === 'undefined') return;
    const composer = document.querySelector('[data-testid="chat-composer-input"]');
    if (!composer) return;
    const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value')?.set;
    if (typeof setter === 'function') {
      setter.call(composer, prompt);
    } else {
      composer.value = prompt;
    }
    composer.dispatchEvent(new Event('input', { bubbles: true }));
    composer.focus();
    const end = prompt.length;
    if (typeof composer.setSelectionRange === 'function') {
      composer.setSelectionRange(end, end);
    }
  };

  return (
    <div className="chat-starter-stage">
      <div className="chat-starter-tasks">
        <div className="chat-starter-tasks-head">
          <h3 className="chat-starter-tasks-title">{title}</h3>
          {subtitle ? <div className="chat-starter-tasks-subtitle">{subtitle}</div> : null}
        </div>
        <div className="chat-starter-tasks-grid">
          {tasks.map((task, index) => (
            <button
              key={String(task?.id || `${task?.title || 'starter'}-${index}`)}
              type="button"
              className="chat-starter-task-card"
              onClick={() => onSelectTask(task)}
            >
              <span className="chat-starter-task-icon" aria-hidden="true">{starterIcon(task)}</span>
              <div className="chat-starter-task-title">{String(task?.title || '').trim()}</div>
              <div className="chat-starter-task-description">
                {String(task?.description || task?.agentName || '').trim()}
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
