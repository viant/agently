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

  const resolveVisibleComposer = (origin) => {
    if (typeof document === 'undefined') return null;
    const candidates = Array.from(document.querySelectorAll('[data-testid="chat-composer-input"]'));
    const visible = candidates.filter((node) => {
      try {
        const rect = node.getBoundingClientRect?.();
        return !!rect && rect.width > 0 && rect.height > 0;
      } catch (_) {
        return false;
      }
    });
    const localRoot = origin?.closest?.('[role="tabpanel"], .app-chat-pane, .chat-starter-stage, .app-shell');
    if (localRoot) {
      const localComposer = visible.find((node) => localRoot.contains(node));
      if (localComposer) return localComposer;
    }
    return visible[visible.length - 1] || candidates[candidates.length - 1] || null;
  };

  const currentConversationId = () => {
    try {
      const form = context?.Context?.('conversations')?.handlers?.dataSource?.peekFormData?.() || {};
      return String(form?.id || '').trim();
    } catch (_) {
      return '';
    }
  };

  const persistStarterPrompt = (prompt, conversationId = '') => {
    if (typeof window === 'undefined') return;
    try {
      const key = 'forge.composerDrafts.v1';
      const raw = window.sessionStorage?.getItem(key) || '{}';
      const parsed = JSON.parse(raw);
      const next = parsed && typeof parsed === 'object' ? parsed : {};
      const targetId = String(conversationId || '__pending__').trim() || '__pending__';
      next[targetId] = String(prompt || '');
      window.sessionStorage?.setItem(key, JSON.stringify(next));
    } catch (_) {}
  };

  const onSelectTask = (task, event) => {
    const prompt = String(task?.prompt || '').trim();
    if (!prompt || typeof document === 'undefined') return;
    const conversationId = currentConversationId();
    persistStarterPrompt(prompt, conversationId);
    try {
      window.dispatchEvent(new CustomEvent('forge:composer-prefill', {
        detail: { prompt, conversationId }
      }));
    } catch (_) {}
    const composer = resolveVisibleComposer(event?.currentTarget || event?.target || null);
    if (!composer) return;
    const proto = Object.getPrototypeOf(composer) || window.HTMLTextAreaElement?.prototype || window.HTMLInputElement?.prototype;
    const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
    if (typeof setter === 'function') {
      setter.call(composer, prompt);
    } else {
      composer.value = prompt;
    }
    composer.dispatchEvent(new Event('input', { bubbles: true }));
    composer.dispatchEvent(new Event('change', { bubbles: true }));
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
              onClick={(event) => onSelectTask(task, event)}
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
