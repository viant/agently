const COMPOSER_SELECTOR = 'form[data-testid="chat-composer"]';
const INPUT_SELECTOR = '[data-testid="chat-composer-input"]';
const HISTORY_SELECTOR = '[data-testid="chat-composer-history"]';
const TOGGLE_SELECTOR = '[data-testid="chat-composer-history-toggle"]';
const PANEL_SELECTOR = '[data-testid="chat-composer-history-panel"]';
const HISTORY_KEY = 'agently_composer_history';
const HISTORY_MAX = 30;

function readHistory(prefix = '') {
    try {
        const raw = localStorage.getItem(HISTORY_KEY);
        const list = raw ? JSON.parse(raw) : [];
        if (!Array.isArray(list)) return [];
        const normalized = String(prefix || '').trim().toLowerCase();
        const filtered = normalized
            ? list.filter((item) => String(item || '').toLowerCase().includes(normalized))
            : list;
        return filtered.slice(-HISTORY_MAX).reverse();
    } catch (_) {
        return [];
    }
}

function applyInputValue(input, value) {
    if (!input) return;
    const next = String(value || '');
    const proto = Object.getPrototypeOf(input);
    const descriptor = Object.getOwnPropertyDescriptor(proto, 'value');
    if (descriptor?.set) {
        descriptor.set.call(input, next);
    } else {
        input.value = next;
    }
    input.dispatchEvent(new Event('input', {bubbles: true}));
}

function focusInput(input) {
    if (!input) return;
    try {
        input.focus({preventScroll: true});
    } catch (_) {
        try {
            input.focus();
        } catch (_) {}
    }
}

function closeComposerHistory(form) {
    if (!form) return;
    form.classList.remove('agently-history-open');
    const toggle = form.querySelector(TOGGLE_SELECTOR);
    if (toggle) toggle.setAttribute('aria-expanded', 'false');
}

function openComposerHistory(form, input) {
    if (!form || !input) return;
    form.classList.add('agently-history-open');
    const toggle = form.querySelector(TOGGLE_SELECTOR);
    if (toggle) toggle.setAttribute('aria-expanded', 'true');
    renderHistoryPanel(form, input, {resetSelection: true});
    focusInput(input);
}

function toggleComposerHistory(form, input) {
    if (!form || !input) return;
    if (form.classList.contains('agently-history-open')) {
        closeComposerHistory(form);
        return;
    }
    openComposerHistory(form, input);
}

function ensurePanel(form, input) {
    let panel = form.querySelector(PANEL_SELECTOR);
    if (panel) return panel;

    panel = document.createElement('div');
    panel.className = 'agently-history-panel';
    panel.setAttribute('data-testid', 'chat-composer-history-panel');
    panel.innerHTML = `
        <div class="agently-history-list" data-testid="chat-composer-history-list" role="listbox"></div>
        <div class="agently-history-preview-wrap">
            <div class="agently-history-preview-head">
                <div class="agently-history-preview-label">Preview</div>
                <div class="agently-history-help" data-testid="chat-composer-history-help">Click: preview • Double-click/Enter: insert • Esc: close</div>
            </div>
            <pre class="agently-history-preview" data-testid="chat-composer-history-preview"></pre>
            <div class="agently-history-actions">
                <button type="button" class="bp6-button bp6-minimal" data-testid="chat-composer-history-close">Close</button>
                <button type="button" class="bp6-button bp6-intent-primary" data-testid="chat-composer-history-apply">Use Selected</button>
            </div>
        </div>
    `;

    panel.querySelector('[data-testid="chat-composer-history-close"]').addEventListener('click', (event) => {
        event.preventDefault();
        closeComposerHistory(form);
    });

    panel.querySelector('[data-testid="chat-composer-history-apply"]').addEventListener('click', (event) => {
        event.preventDefault();
        const items = form.__agentlyHistoryItems || [];
        const index = Number(form.__agentlyHistoryIndex ?? -1);
        if (index < 0 || index >= items.length) return;
        applyInputValue(input, items[index]);
        closeComposerHistory(form);
        focusInput(input);
    });

    const wrapper = input.closest('.composer-wrapper') || input.parentElement || form;
    wrapper.appendChild(panel);
    return panel;
}

function renderHistoryPanel(form, input, options = {}) {
    const panel = ensurePanel(form, input);
    const listEl = panel.querySelector('[data-testid="chat-composer-history-list"]');
    const previewEl = panel.querySelector('[data-testid="chat-composer-history-preview"]');
    const applyButton = panel.querySelector('[data-testid="chat-composer-history-apply"]');
    const history = readHistory(input?.value || '');
    const hasReset = !!options.resetSelection;

    form.__agentlyHistoryItems = history;
    let selectedIndex = Number(form.__agentlyHistoryIndex ?? -1);
    if (hasReset || selectedIndex < 0 || selectedIndex >= history.length) {
        selectedIndex = history.length > 0 ? 0 : -1;
    }
    form.__agentlyHistoryIndex = selectedIndex;

    listEl.innerHTML = '';

    if (!history.length) {
        const empty = document.createElement('div');
        empty.className = 'agently-history-empty';
        empty.textContent = 'No recent messages';
        listEl.appendChild(empty);
        previewEl.textContent = '';
        applyButton.disabled = true;
        return;
    }

    history.forEach((item, index) => {
        const row = document.createElement('button');
        row.type = 'button';
        row.className = `agently-history-row${index === selectedIndex ? ' is-selected' : ''}`;
        row.setAttribute('data-history-index', String(index));
        row.setAttribute('title', item);
        row.textContent = item;
        row.addEventListener('mousedown', (event) => {
            // Preserve textarea focus so arrow keys keep navigating history after clicks.
            event.preventDefault();
            focusInput(input);
        });
        row.addEventListener('click', (event) => {
            event.preventDefault();
            form.__agentlyHistoryIndex = index;
            renderHistoryPanel(form, input);
            focusInput(input);
        });
        row.addEventListener('dblclick', (event) => {
            event.preventDefault();
            applyInputValue(input, item);
            closeComposerHistory(form);
            focusInput(input);
        });
        listEl.appendChild(row);
    });

    previewEl.textContent = history[selectedIndex] || '';
    applyButton.disabled = selectedIndex < 0;

    if (selectedIndex >= 0) {
        const selectedRow = listEl.querySelector(`.agently-history-row[data-history-index="${selectedIndex}"]`);
        if (selectedRow) {
            try {
                selectedRow.scrollIntoView({block: 'nearest'});
            } catch (_) {
                try {
                    selectedRow.scrollIntoView();
                } catch (_) {}
            }
        }
    }
}

function stepSelection(form, input, delta) {
    const items = form.__agentlyHistoryItems || [];
    if (!items.length) return;
    const maxIndex = items.length - 1;
    const current = Number(form.__agentlyHistoryIndex ?? 0);
    const next = Math.max(0, Math.min(maxIndex, current + delta));
    form.__agentlyHistoryIndex = next;
    renderHistoryPanel(form, input);
}

function mountToggleButton(form, input) {
    if (!form || form.querySelector(TOGGLE_SELECTOR)) return;

    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'bp6-button bp6-minimal composer-icon-btn agently-history-trigger';
    button.setAttribute('data-testid', 'chat-composer-history-toggle');
    button.setAttribute('aria-label', 'Recent prompts');
    button.setAttribute('aria-expanded', 'false');
    button.setAttribute('title', 'Recent prompts. Click to browse recent prompts.');
    button.setAttribute('data-tooltip', 'Recent prompts');
    button.textContent = 'H';

    button.addEventListener('mousedown', (event) => {
        event.preventDefault();
    });
    button.addEventListener('click', (event) => {
        event.preventDefault();
        event.stopPropagation();
        toggleComposerHistory(form, input);
    });

    const barLeft = form.querySelector('.composer-bar-left');
    if (barLeft) {
        barLeft.appendChild(button);
        return;
    }

    const wrapper = input.closest('.composer-wrapper') || input.parentElement || form;
    wrapper.classList.add('agently-history-has-trigger');
    wrapper.appendChild(button);
}

function wireComposer(form) {
    if (!form || form.dataset.agentlyHistoryEnhanced === 'true') return;

    const input = form.querySelector(INPUT_SELECTOR);
    if (!input) return;

    form.dataset.agentlyHistoryEnhanced = 'true';
    form.classList.add('agently-history-managed');

    mountToggleButton(form, input);
    ensurePanel(form, input);

    input.addEventListener('input', () => {
        if (!form.classList.contains('agently-history-open')) return;
        renderHistoryPanel(form, input, {resetSelection: true});
    });

    form.addEventListener('keydown', (event) => {
        if (event.key === 'Escape') {
            closeComposerHistory(form);
            return;
        }
        if (!form.classList.contains('agently-history-open')) return;
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            event.stopPropagation();
            stepSelection(form, input, 1);
            focusInput(input);
            return;
        }
        if (event.key === 'ArrowUp') {
            event.preventDefault();
            event.stopPropagation();
            stepSelection(form, input, -1);
            focusInput(input);
            return;
        }
        if (event.key === 'Enter' && !event.shiftKey) {
            const items = form.__agentlyHistoryItems || [];
            const index = Number(form.__agentlyHistoryIndex ?? -1);
            if (index < 0 || index >= items.length) return;
            event.preventDefault();
            event.stopPropagation();
            applyInputValue(input, items[index]);
            closeComposerHistory(form);
            focusInput(input);
        }
    });

    form.addEventListener('submit', () => {
        closeComposerHistory(form);
    });
}

function enhanceAllComposers(root = document) {
    const forms = root.querySelectorAll(COMPOSER_SELECTOR);
    forms.forEach((form) => wireComposer(form));
}

export function installComposerHistoryEnhancer() {
    if (typeof document === 'undefined') return () => {};
    if (window.__agentlyComposerHistoryEnhancerInstalled) {
        return window.__agentlyComposerHistoryEnhancerInstalled;
    }

    enhanceAllComposers();

    const observer = new MutationObserver((mutations) => {
        for (const mutation of mutations) {
            mutation.addedNodes.forEach((node) => {
                if (!(node instanceof HTMLElement)) return;
                if (node.matches?.(COMPOSER_SELECTOR)) {
                    wireComposer(node);
                    return;
                }
                enhanceAllComposers(node);
            });
        }
    });

    observer.observe(document.body, {childList: true, subtree: true});

    const handlePointerDown = (event) => {
        const forms = document.querySelectorAll(`${COMPOSER_SELECTOR}.agently-history-open`);
        forms.forEach((form) => {
            if (form.contains(event.target)) return;
            closeComposerHistory(form);
        });
    };

    document.addEventListener('pointerdown', handlePointerDown, true);

    const cleanup = () => {
        observer.disconnect();
        document.removeEventListener('pointerdown', handlePointerDown, true);
        delete window.__agentlyComposerHistoryEnhancerInstalled;
    };

    window.__agentlyComposerHistoryEnhancerInstalled = cleanup;
    return cleanup;
}
