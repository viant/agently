import React from 'react';
import {
  BugBeetle,
  Buildings,
  CalendarDots,
  ChartPieSlice,
  ChartLineUp,
  ChatCircleText,
  CirclesThree,
  FileText,
  Flask,
  GlobeHemisphereWest,
  Handshake,
  ImageSquare,
  Info,
  EnvelopeSimple,
  Palette,
  Path,
  PencilSimple,
  RocketLaunch,
  ClipboardText,
  ShieldWarning,
  Tag,
  Target,
  Pulse,
  Stack,
  TrendUp,
  TreeStructure,
  Wrench,
} from '@phosphor-icons/react';
import { resolveStarterPromptMacros } from './starterPromptMacros.js';

const ICONS = {
  bug: BugBeetle,
  buildings: Buildings,
  chat: ChatCircleText,
  'calendar-report': CalendarDots,
  'chart-arcs': ChartPieSlice,
  'chart-line': ChartLineUp,
  document: FileText,
  email: EnvelopeSimple,
  flask: Flask,
  'globe-search': GlobeHemisphereWest,
  help: Info,
  handshake: Handshake,
  image: ImageSquare,
  palette: Palette,
  clipboard: ClipboardText,
  pencil: PencilSimple,
  radar: Target,
  pulse: Pulse,
  rocket: RocketLaunch,
  route: Path,
  'shield-warning': ShieldWarning,
  tags: Tag,
  layers: Stack,
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

function categoryKey(category = {}) {
  return `${String(category?.agentId || '').trim()}|${String(category?.id || '').trim()}`;
}

function taskBelongsToCategory(task = {}, category = {}) {
  const taskCategory = String(task?.categoryId || '').trim();
  const categoryID = String(category?.id || '').trim();
  if (!taskCategory || taskCategory !== categoryID) return false;
  const taskAgent = String(task?.agentId || '').trim();
  const categoryAgent = String(category?.agentId || '').trim();
  return !taskAgent || !categoryAgent || taskAgent === categoryAgent;
}

export default function StarterTasks({ message, context }) {
  const tasks = Array.isArray(message?.starterTasks) ? message.starterTasks : [];
  const declaredCategories = Array.isArray(message?.starterTaskCategories) ? message.starterTaskCategories : [];
  const title = String(message?.title || 'Start with an agent prompt').trim();
  const subtitle = String(message?.subtitle || '').trim();

  const categories = declaredCategories.filter((category) => (
    String(category?.id || '').trim()
    && String(category?.title || '').trim()
    && tasks.some((task) => taskBelongsToCategory(task, category))
  ));
  const categorizedTaskIndexes = new Set();
  categories.forEach((category) => {
    tasks.forEach((task, index) => {
      if (taskBelongsToCategory(task, category)) categorizedTaskIndexes.add(index);
    });
  });
  const uncategorizedTasks = tasks.filter((_, index) => !categorizedTaskIndexes.has(index));
  const presentationCategories = categories.length > 0
    ? [...categories, ...(uncategorizedTasks.length > 0 ? [{ id: '__more__', title: 'More', icon: 'chat', tasks: uncategorizedTasks }] : [])]
    : [];
  const [selectedCategoryKey, setSelectedCategoryKey] = React.useState('');
  React.useEffect(() => {
    if (selectedCategoryKey && !presentationCategories.some((category) => categoryKey(category) === selectedCategoryKey)) {
      setSelectedCategoryKey('');
    }
  }, [presentationCategories, selectedCategoryKey]);

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
    const prompt = resolveStarterPromptMacros(task?.prompt, new Date(), { dateTokens: true }).trim();
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

  const renderTaskCard = (task, index) => (
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
  );

  if (presentationCategories.length > 0) {
    const selectedCategory = presentationCategories.find((category) => categoryKey(category) === selectedCategoryKey) || null;
    const selectedCategoryTone = selectedCategory ? (presentationCategories.indexOf(selectedCategory) % 5) + 1 : undefined;
    const selectedTasks = Array.isArray(selectedCategory?.tasks)
      ? selectedCategory.tasks
      : selectedCategory ? tasks.filter((task) => taskBelongsToCategory(task, selectedCategory)) : [];
    return (
      <div className="chat-starter-stage chat-starter-stage--categorized">
        <div className="chat-starter-tasks chat-starter-tasks--categorized" data-category-tone={selectedCategoryTone}>
          <div className="chat-starter-tasks-head">
            {selectedCategory ? (
              <button type="button" className="chat-starter-category-back" onClick={() => setSelectedCategoryKey('')}>
                <span aria-hidden="true">←</span> Back to categories
              </button>
            ) : (
              <h3 className="chat-starter-tasks-title">Starter tasks</h3>
            )}
          </div>
          {!selectedCategory ? (
            <div className="chat-starter-category-list" role="list" aria-label="Starter task categories">
              {presentationCategories.map((category) => {
                const key = categoryKey(category);
                return (
                  <button key={key} type="button" className="chat-starter-category" onClick={() => setSelectedCategoryKey(key)}>
                    <span className="chat-starter-category-icon" aria-hidden="true">{starterIcon(category)}</span>
                    <span className="chat-starter-category-copy">
                      <span className="chat-starter-category-title">{category.title}</span>
                      {String(category.description || '').trim() ? <span className="chat-starter-category-description">{category.description}</span> : null}
                    </span>
                    <span className="chat-starter-category-arrow" aria-hidden="true">›</span>
                  </button>
                );
              })}
            </div>
          ) : (
            <>
              <div className="chat-starter-category-heading">
                <div>
                  <h4>{selectedCategory.title}</h4>
                  {String(selectedCategory.description || '').trim() ? <p>{selectedCategory.description}</p> : null}
                </div>
                <span>{selectedTasks.length} {selectedTasks.length === 1 ? 'task' : 'tasks'}</span>
              </div>
              <div className="chat-starter-tasks-grid chat-starter-tasks-grid--categorized">
                {selectedTasks.map(renderTaskCard)}
              </div>
            </>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="chat-starter-stage">
      <div className="chat-starter-tasks">
        <div className="chat-starter-tasks-head">
          <h3 className="chat-starter-tasks-title">{title}</h3>
          {subtitle ? <div className="chat-starter-tasks-subtitle">{subtitle}</div> : null}
        </div>
        <div className="chat-starter-tasks-grid">
          {tasks.map(renderTaskCard)}
        </div>
      </div>
    </div>
  );
}
