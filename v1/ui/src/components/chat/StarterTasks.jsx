import React from 'react';
import {
  BugBeetle,
  Buildings,
  CalendarDots,
  ChartLineUp,
  ChatCircleText,
  CirclesThree,
  FileText,
  Flask,
  GlobeHemisphereWest,
  Handshake,
  Info,
  Palette,
  Path,
  PencilSimple,
  RocketLaunch,
  ShieldWarning,
  Tag,
  Target,
  TrendUp,
  TreeStructure,
  Wrench,
} from '@phosphor-icons/react';

const ICONS = {
  bug: BugBeetle,
  buildings: Buildings,
  chat: ChatCircleText,
  'calendar-report': CalendarDots,
  'chart-line': ChartLineUp,
  document: FileText,
  flask: Flask,
  'globe-search': GlobeHemisphereWest,
  help: Info,
  handshake: Handshake,
  palette: Palette,
  pencil: PencilSimple,
  radar: Target,
  rocket: RocketLaunch,
  route: Path,
  'shield-warning': ShieldWarning,
  tags: Tag,
  'trend-up': TrendUp,
  'tree-structure': TreeStructure,
  venn: CirclesThree,
  wrench: Wrench,
};

function starterIcon(task = {}) {
  const key = String(task?.icon || '').trim().toLowerCase();
  const Icon = ICONS[key] || ChatCircleText;
  return <Icon size={18} weight="duotone" />;
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
