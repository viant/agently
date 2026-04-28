import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import {
  Popover,
  Menu,
  MenuItem,
  InputGroup,
  Spinner,
  Tag,
  Button,
} from '@blueprintjs/core';
import {
  parseAuthored,
  parseTokens,
  rehydrate,
  serializeToken,
  serializeManualToken,
} from './tokens.js';
import { listLookupRegistry, fetchDatasource } from './client.js';
import { applyResolvedChipToken, createEditingChipState, shouldSkipEditorSync } from './chipEditing.js';

const DEFAULT_TRIGGER = '/';

function displaySegments(storedValue = '') {
  return rehydrate(storedValue);
}

function serializeEditor(root) {
  if (!root) return '';
  const parts = [];
  for (const node of Array.from(root.childNodes || [])) {
    if (node.nodeType === Node.TEXT_NODE) {
      parts.push(node.textContent || '');
      continue;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) continue;
    const token = node.getAttribute('data-token');
    if (token) {
      parts.push(token);
      continue;
    }
    parts.push(node.textContent || '');
  }
  return parts.join('');
}

function caretOffsetWithin(root) {
  const selection = window.getSelection?.();
  if (!root || !selection || selection.rangeCount === 0) return 0;
  const range = selection.getRangeAt(0).cloneRange();
  const pre = range.cloneRange();
  pre.selectNodeContents(root);
  pre.setEnd(range.endContainer, range.endOffset);
  return pre.toString().length;
}

function locatePoint(root, target) {
  const walker = document.createTreeWalker(
    root,
    NodeFilter.SHOW_TEXT,
    null
  );
  let node = walker.nextNode();
  let remaining = Math.max(0, target);
  while (node) {
    const text = node.textContent || '';
    if (remaining <= text.length) {
      return { node, offset: remaining };
    }
    remaining -= text.length;
    node = walker.nextNode();
  }
  return { node: root, offset: root.childNodes.length };
}

function replaceDisplayRangeWithChip(root, start, end, tokenText, label) {
  if (!root) return '';
  const range = document.createRange();
  const startPoint = locatePoint(root, start);
  const endPoint = locatePoint(root, end);
  range.setStart(startPoint.node, startPoint.offset);
  range.setEnd(endPoint.node, endPoint.offset);
  range.deleteContents();

  const chip = document.createElement('span');
  chip.setAttribute('data-token', tokenText);
  chip.setAttribute('contenteditable', 'false');
  chip.className = 'named-lookup-inline-chip';
  chip.textContent = label;
  range.insertNode(chip);

  const spacer = document.createTextNode('');
  chip.after(spacer);

  const selection = window.getSelection?.();
  if (selection) {
    const nextRange = document.createRange();
    nextRange.setStart(spacer, 0);
    nextRange.collapse(true);
    selection.removeAllRanges();
    selection.addRange(nextRange);
  }

  return serializeEditor(root);
}

function replaceChipToken(root, rawToken, nextToken, nextLabel) {
  if (!root) return '';
  const chip = Array.from(root.querySelectorAll('[data-token]')).find(
    (node) => node.getAttribute('data-token') === rawToken
  );
  if (!chip) return '';
  chip.setAttribute('data-token', nextToken);
  chip.textContent = nextLabel;
  return serializeEditor(root);
}

function syncEditorContent(root, segments, options = {}) {
  const {
    editingChip = null,
    setChipEditInputElement = null,
    onChipEditFocus = null,
    onChipEditInput = null,
    onChipEditCommit = null,
    onChipEditCancel = null,
    onChipEditLookup = null,
    onChipActivate = null,
  } = options;
  if (!root) return;
  if (typeof setChipEditInputElement === 'function') {
    setChipEditInputElement(null);
  }
  while (root.firstChild) {
    root.removeChild(root.firstChild);
  }
  for (const segment of segments) {
    if (segment.kind === 'text') {
      root.appendChild(document.createTextNode(segment.value || ''));
      continue;
    }
    if (editingChip && editingChip.raw === segment.raw) {
      const wrap = document.createElement('span');
      wrap.setAttribute('data-token', segment.raw);
      wrap.setAttribute('data-name', segment.name);
      wrap.setAttribute('data-chip-editor', 'true');
      wrap.setAttribute('contenteditable', 'false');
      wrap.style.display = 'inline-flex';
      wrap.style.alignItems = 'center';
      wrap.style.gap = '6px';
      wrap.style.margin = '0 2px';
      wrap.style.padding = '3px 6px 3px 10px';
      wrap.style.borderRadius = '999px';
      wrap.style.minHeight = '34px';
      wrap.style.border = editingChip.error ? '1px solid #c23030' : '1px solid #d7e3f0';
      wrap.style.background = editingChip.error ? '#fff7f7' : '#ffffff';
      wrap.style.boxShadow = editingChip.error
        ? '0 0 0 2px rgba(193, 48, 48, 0.08), 0 1px 2px rgba(15, 23, 42, 0.06)'
        : '0 0 0 2px rgba(47, 112, 225, 0.08), 0 1px 2px rgba(15, 23, 42, 0.06)';
      wrap.style.verticalAlign = 'middle';

      const input = document.createElement('input');
      if (typeof setChipEditInputElement === 'function') {
        setChipEditInputElement(input);
      }
      input.type = 'text';
      input.setAttribute('data-testid', `named-lookup-inline-editor-${segment.name}`);
      input.value = String(editingChip.value || '');
      input.placeholder = unresolvedChipLabel(segment.name);
      input.style.border = 'none';
      input.style.outline = 'none';
      input.style.background = 'transparent';
      input.style.font = 'inherit';
      input.style.fontSize = '15px';
      input.style.fontWeight = '500';
      input.style.lineHeight = '20px';
      input.style.color = '#182026';
      input.style.caretColor = '#2f70e1';
      input.style.padding = '0';
      input.style.minWidth = '56px';
      input.style.width = `${Math.max(56, Math.min(220, (String(editingChip.value || '').length + 1) * 10))}px`;
      input.title = editingChip.error || '';
      input.addEventListener('focus', () => {
        onChipEditFocus?.();
      });
      input.addEventListener('input', (event) => {
        onChipEditInput?.(event?.target?.value ?? '');
      });
      input.addEventListener('keydown', (event) => {
        if (event.key === 'Enter') {
          event.preventDefault();
          onChipEditCommit?.(input.value, segment);
        } else if (event.key === 'Escape') {
          event.preventDefault();
          onChipEditCancel?.();
        } else {
          emitNamedLookupDebug('chip.input.keydown', {
            raw: segment.raw,
            name: segment.name,
            key: event.key,
            value: input.value,
          });
        }
      });
      input.addEventListener('blur', () => {
        emitNamedLookupDebug('chip.input.blur', {
          raw: segment.raw,
          name: segment.name,
          value: input.value,
        });
        if (skipChipBlurRef.current) {
          skipChipBlurRef.current = false;
          return;
        }
        onChipEditCommit?.(input.value, segment);
      });

      const button = document.createElement('button');
      button.type = 'button';
      button.textContent = '▾';
      button.setAttribute('aria-label', `Open lookup for ${segment.name}`);
      button.setAttribute('title', `Browse ${segment.name}`);
      button.setAttribute('data-testid', `named-lookup-inline-browse-${segment.name}`);
      button.tabIndex = -1;
      button.style.border = '1px solid #cfe0f7';
      button.style.background = 'linear-gradient(180deg, #f8fbff 0%, #edf4ff 100%)';
      button.style.cursor = 'pointer';
      button.style.padding = '0';
      button.style.height = '26px';
      button.style.minWidth = '26px';
      button.style.borderRadius = '999px';
      button.style.color = '#4e78b8';
      button.style.boxShadow = '0 1px 1px rgba(15, 23, 42, 0.05)';
      button.style.font = 'inherit';
      button.style.fontSize = '14px';
      button.style.fontWeight = '600';
      button.style.lineHeight = '1';
      button.style.display = 'inline-flex';
      button.style.alignItems = 'center';
      button.style.justifyContent = 'center';
      button.addEventListener('mousedown', (event) => {
        event.preventDefault();
      });
      button.addEventListener('click', (event) => {
        event.preventDefault();
        onChipEditLookup?.();
      });

      wrap.appendChild(input);
      wrap.appendChild(button);
      root.appendChild(wrap);
      continue;
    }
    const chip = document.createElement('button');
    chip.setAttribute('type', 'button');
    chip.setAttribute('data-token', segment.raw);
    chip.setAttribute('data-name', segment.name);
    chip.setAttribute('contenteditable', 'false');
    chip.setAttribute('data-testid', `named-lookup-chip-${segment.name}`);
    chip.tabIndex = -1;
    chip.style.display = 'inline-flex';
    chip.style.alignItems = 'center';
    chip.style.borderRadius = '999px';
    chip.style.padding = '2px 8px';
    chip.style.margin = '0 2px';
    chip.style.background = segment.unresolved ? '#fff5d6' : '#eef8f0';
    chip.style.border = `1px solid ${segment.unresolved ? '#d9822b' : '#0f9960'}`;
    chip.style.color = segment.unresolved ? '#8a5d00' : '#0a6640';
    chip.style.cursor = 'pointer';
    chip.style.userSelect = 'none';
    chip.style.font = 'inherit';
    chip.style.lineHeight = 'inherit';
    chip.style.appearance = 'none';
    chip.textContent = segment.unresolved
      ? unresolvedChipLabel(segment.name)
      : (segment.label || unresolvedChipLabel(segment.name));
    chip.addEventListener('mousedown', (event) => {
      event.preventDefault();
      event.stopPropagation();
    });
    chip.addEventListener('click', (event) => {
      event.preventDefault();
      event.stopPropagation();
      onChipActivate?.(segment);
    });
    root.appendChild(chip);
  }
}

function placeCaretAtEnd(root) {
  if (!root) return;
  const selection = window.getSelection?.();
  if (!selection) return;
  const range = document.createRange();
  const nodes = Array.from(root.childNodes || []);
  const last = nodes[nodes.length - 1] || root;
  if (last.nodeType === Node.TEXT_NODE) {
    const text = last.textContent || '';
    range.setStart(last, text.length);
    range.collapse(true);
  } else {
    range.selectNodeContents(root);
    range.collapse(false);
  }
  selection.removeAllRanges();
  selection.addRange(range);
}

function textBeforeCaret(displayText = '', caret = 0) {
  return displayText.slice(0, Math.max(0, caret));
}

function humanizeLookupName(name = '') {
  const spaced = String(name || '')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .trim();
  if (!spaced) return '';
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

function unresolvedChipLabel(name = '') {
  return humanizeLookupName(name) || String(name || '');
}

function lookupMenuLabel(entry = {}) {
  const explicit = String(entry?.title || '').trim();
  const base = explicit || unresolvedChipLabel(entry?.name || '');
  return base;
}

function emitNamedLookupDebug(type, payload = {}) {
  if (!import.meta.env?.DEV || typeof window === 'undefined') return;
  const event = {
    at: Date.now(),
    type,
    ...payload,
  };
  window.__namedLookupDebug = window.__namedLookupDebug || [];
  window.__namedLookupDebug.push(event);
  if (window.__namedLookupDebug.length > 200) {
    window.__namedLookupDebug.splice(0, window.__namedLookupDebug.length - 200);
  }
  window.dispatchEvent(new CustomEvent('named-lookup:debug', { detail: event }));
  console.debug('[named-lookup]', event);
}

function tokenLabel(tokenText = '', fallback = '') {
  const parsed = parseTokens(tokenText);
  return parsed[0]?.label || fallback;
}

function unwrapSelection(record) {
  if (!record || typeof record !== 'object') return record;
  if (record.selected) return record.selected;
  if (Array.isArray(record.selection) && record.selection.length > 0) {
    const first = record.selection[0];
    return first?.selected || first;
  }
  return record;
}

function normalizeLookupInputs(inputs = []) {
  return (Array.isArray(inputs) ? inputs : []).map((p) => ({
    ...p,
    from: p?.from || ':form',
    to: p?.to || ':query',
  }));
}

function chipEditorFromTarget(target) {
  return target?.closest?.('[data-chip-editor="true"]') || null;
}

function chipStateFromElement(chip) {
  const raw = chip?.getAttribute?.('data-token') || '';
  const name = chip?.getAttribute?.('data-name') || '';
  const parsed = parseTokens(raw)[0];
  return {
    raw,
    name,
    label: parsed?.label || unresolvedChipLabel(name),
    unresolved: parsed?.id === '?',
  };
}

export default function NamedLookupInput({
  value = '',
  onChange,
  onValidityChange,
  onValueResolver,
  context,
  contextKind = 'chat-composer',
  contextID = 'default',
  multiline = false,
  placeholder,
  disabled,
  debounceMs = 150,
  autoResolveAuthored = true,
  onRegistryLoaded,
  style,
  className,
  onFocus,
  onBlur,
  'data-testid': dataTestId,
  ...rest
}) {
  const [registry, setRegistry] = useState([]);
  const [activeTrigger, setActiveTrigger] = useState(null);
  const [rows, setRows] = useState([]);
  const [rowsLoading, setRowsLoading] = useState(false);
  const [retainInlineEditor, setRetainInlineEditor] = useState(false);
  const [editingChip, setEditingChip] = useState(null);
  const [debugEvents, setDebugEvents] = useState([]);
  const editorRef = useRef(null);
  const inputRef = useRef(null);
  const chipEditInputRef = useRef(null);
  const fetchTimer = useRef(null);
  const lastSyncedValueRef = useRef(value);
  const previousInlineModeRef = useRef(null);
  const skipChipBlurRef = useRef(false);
  const editingChipRef = useRef(editingChip);
  const registryRef = useRef(registry);
  const chipEditFocusedRef = useRef(false);

  useEffect(() => {
    if (typeof onValueResolver !== 'function') return;
    onValueResolver(() => {
      if (multiline && editorRef.current) {
        return serializeEditor(editorRef.current);
      }
      return value;
    });
  }, [multiline, onValueResolver, value]);

  useEffect(() => {
    let cancelled = false;
    listLookupRegistry(contextKind, contextID)
      .then((entries) => {
        if (cancelled) return;
        emitNamedLookupDebug('registry.loaded', {
          context: `${contextKind}:${contextID}`,
          count: entries.length,
          hasOrder: entries.some((entry) => entry?.name === 'order'),
        });
        setRegistry(entries);
        onRegistryLoaded?.(entries);
      })
      .catch((e) => {
        emitNamedLookupDebug('registry.error', {
          context: `${contextKind}:${contextID}`,
          reason: String(e?.message || e || 'unknown'),
        });
        console.warn('lookup registry load failed', e);
      });
    return () => {
      cancelled = true;
    };
  }, [contextKind, contextID, onRegistryLoaded]);

  useEffect(() => {
    if (!autoResolveAuthored || registry.length === 0) return;
    const segments = parseAuthored(value, registry);
    if (!segments.some((s) => s.kind === 'picker')) return;
    const rebuilt = segments
      .map((segment) => {
        if (segment.kind === 'text') return segment.value;
        return `@{${segment.entry.name}:? "${unresolvedChipLabel(segment.entry.name)}"}`;
      })
      .join('');
    if (rebuilt !== value) onChange(rebuilt);
  }, [autoResolveAuthored, onChange, registry, value]);

  const segments = useMemo(() => displaySegments(value), [value]);
  const chips = useMemo(
    () => segments.filter((segment) => segment.kind === 'chip'),
    [segments]
  );
  const hasInlineChips = chips.length > 0;
  const useInlineEditor = multiline && (hasInlineChips || retainInlineEditor);
  const hasUnresolvedRequired = useMemo(
    () =>
      chips.some((chip) => {
        if (!chip.unresolved) return false;
        const entry = registry.find((item) => item.name === chip.name);
        return !!entry?.required;
      }),
    [chips, registry]
  );
  const editingChipEntry = useMemo(
    () => registry.find((entry) => entry.name === editingChip?.name) || null,
    [editingChip?.name, registry]
  );

  useEffect(() => {
    editingChipRef.current = editingChip;
  }, [editingChip]);

  useEffect(() => {
    registryRef.current = registry;
  }, [registry]);

  useEffect(() => {
    if (!import.meta.env?.DEV || typeof window === 'undefined') return undefined;
    const handleDebugEvent = (event) => {
      const detail = event?.detail;
      if (!detail) return;
      setDebugEvents((current) => [...current.slice(-5), detail]);
    };
    window.addEventListener('named-lookup:debug', handleDebugEvent);
    return () => window.removeEventListener('named-lookup:debug', handleDebugEvent);
  }, []);

  useEffect(() => {
    onValidityChange?.(hasUnresolvedRequired);
  }, [hasUnresolvedRequired, onValidityChange]);

  useEffect(() => {
    if (hasInlineChips) {
      setRetainInlineEditor(true);
    }
  }, [hasInlineChips]);

  useEffect(() => {
    if (!editingChip || !chipEditInputRef.current || disabled) return;
    requestAnimationFrame(() => {
      try {
        chipEditInputRef.current?.focus();
        const nextValue = String(editingChip.value || '');
        const end = nextValue.length;
        chipEditInputRef.current?.setSelectionRange?.(end, end);
      } catch (_) {}
    });
  }, [disabled, editingChip]);

  useLayoutEffect(() => {
    const previous = previousInlineModeRef.current;
    previousInlineModeRef.current = useInlineEditor;
    if (previous === null || previous === useInlineEditor) {
      return;
    }

    if (useInlineEditor) {
      if (!editorRef.current || disabled) return;
      requestAnimationFrame(() => {
        try {
          editorRef.current?.focus();
          placeCaretAtEnd(editorRef.current);
        } catch (_) {}
      });
      return;
    }

    if (!inputRef.current || disabled) return;
    requestAnimationFrame(() => {
      try {
        const target = inputRef.current;
        target.focus();
        const nextValue = String(value || '');
        if (typeof target.setSelectionRange === 'function') {
          const end = nextValue.length;
          target.setSelectionRange(end, end);
        }
      } catch (_) {}
    });
  }, [disabled, useInlineEditor, value]);

  const scheduleFetch = useCallback(
    (entry, q) => {
      if (fetchTimer.current) clearTimeout(fetchTimer.current);
      fetchTimer.current = setTimeout(async () => {
        setRowsLoading(true);
        try {
          const input = {};
          const qKey = entry?.token?.queryInput || 'q';
          input[qKey] = q;
          const res = await fetchDatasource(entry.dataSource, input);
          setRows(res?.rows || []);
        } catch (e) {
          console.error('datasource fetch failed', e);
          setRows([]);
        } finally {
          setRowsLoading(false);
        }
      }, debounceMs);
    },
    [debounceMs]
  );

  const handleTextChange = useCallback(
    (nextValue, caret) => {
      onChange(nextValue);
      const display = textBeforeCaret(
        editorRef.current ? editorRef.current.innerText || '' : String(nextValue || ''),
        caret
      );
      const slash = display.lastIndexOf(DEFAULT_TRIGGER);
      if (slash === -1) {
        setActiveTrigger(null);
        return;
      }
      const query = display.slice(slash + 1);
      if (/\s/.test(query)) {
        setActiveTrigger(null);
        return;
      }
      if (!activeTrigger) {
        setActiveTrigger({ phase: 'namePicker', start: slash, caret, query });
        return;
      }
      if (activeTrigger.phase === 'namePicker') {
        setActiveTrigger({ ...activeTrigger, start: slash, caret, query });
        return;
      }
      const rowQuery = query.replace(`${activeTrigger.entry?.name || ''}`, '').trimStart();
      setActiveTrigger({ ...activeTrigger, caret, query: rowQuery });
      scheduleFetch(activeTrigger.entry, rowQuery);
    },
    [activeTrigger, onChange, scheduleFetch]
  );

  const handleEditableInput = useCallback(
    (event) => {
      const nextStored = serializeEditor(editorRef.current);
      lastSyncedValueRef.current = nextStored;
      const caret = caretOffsetWithin(editorRef.current);
      handleTextChange(nextStored, caret);
      if (typeof onFocus === 'function' && event?.type === 'focus') onFocus(event);
    },
    [handleTextChange, onFocus]
  );

  const handlePlainInput = useCallback(
    (event) => {
      const next = event?.target?.value ?? event;
      const caret = event?.target?.selectionStart ?? String(next || '').length;
      handleTextChange(next, caret);
    },
    [handleTextChange]
  );

  const pickName = useCallback(
    (entry) => {
      const caret = multiline
        ? caretOffsetWithin(editorRef.current)
        : inputRef.current?.selectionStart ?? String(value || '').length;
      setActiveTrigger({
        phase: 'rowPicker',
        start: activeTrigger?.start ?? caret - 1,
        caret,
        query: '',
        entry,
      });
      scheduleFetch(entry, '');
    },
    [activeTrigger, multiline, scheduleFetch, value]
  );

  const openLookupSurface = useCallback(
    async (entry, options = {}) => {
      if (!entry?.dialogId && !entry?.windowId) return false;
      const windowHandlers = context?.handlers?.window;
      if (!windowHandlers) {
        console.error('lookup window handlers unavailable', { entry });
        return true;
      }

      const triggerStart = options.start ?? activeTrigger?.start ?? 0;
      const triggerCaret = options.caret ?? activeTrigger?.caret ?? triggerStart;
      const chipRaw = options.chipRaw ?? activeTrigger?.chipRaw ?? '';
      const initialParameters = (options.parameters && typeof options.parameters === 'object')
        ? options.parameters
        : {};
      const paramDefs = normalizeLookupInputs(entry.inputs);

      setRowsLoading(true);
      try {
        let record;
        if (entry.dialogId) {
          record = await windowHandlers.openDialog({
            execution: {
              args: [entry.dialogId, { awaitResult: true, parameters: paramDefs }],
              parameters: paramDefs,
            },
            parameters: initialParameters,
            context,
          });
        } else {
          record = await windowHandlers.openWindow({
            execution: {
              args: [entry.windowId, '', '', false, { awaitResult: true, parameters: paramDefs, modal: true }],
              parameters: paramDefs,
            },
            parameters: initialParameters,
            context,
          });
        }
        const selected = unwrapSelection(record);
        if (!selected) return true;

        const token = serializeToken(entry, selected);
        const label = tokenLabel(token, unresolvedChipLabel(entry.name));
        let nextStored = '';
        if (chipRaw) {
          const resolved = applyResolvedChipToken(value, chipRaw, token);
          nextStored = resolved.ok ? resolved.nextStored : token;
        } else if (multiline && editorRef.current) {
          nextStored = replaceDisplayRangeWithChip(
            editorRef.current,
            triggerStart,
            triggerCaret,
            token,
            label
          );
        } else {
          const before = String(value || '').slice(0, triggerStart);
          const after = String(value || '').slice(triggerCaret);
          nextStored = before + token + after;
        }
        onChange(nextStored || token);
        lastSyncedValueRef.current = nextStored || token;
        return true;
      } catch (e) {
        console.error('lookup open failed', e);
        return true;
      } finally {
        setRows([]);
        setRowsLoading(false);
        setActiveTrigger(null);
      }
    },
    [activeTrigger, context, multiline, onChange, value]
  );

  const pickRow = useCallback(
    (entry, row) => {
      const token = serializeToken(entry, row);
      const label = tokenLabel(token, row?.name || row?.adOrderName || unresolvedChipLabel(entry?.name));
      let nextStored = '';
      if (multiline && editorRef.current) {
        nextStored = replaceDisplayRangeWithChip(
          editorRef.current,
          activeTrigger?.start ?? 0,
          activeTrigger?.caret ?? activeTrigger?.start ?? 0,
          token,
          label
        );
      } else {
        const before = String(value || '').slice(0, activeTrigger?.start ?? 0);
        const after = String(value || '').slice(activeTrigger?.caret ?? 0);
        nextStored = before + token + after;
      }
      onChange(nextStored);
      lastSyncedValueRef.current = nextStored;
      setRows([]);
      setActiveTrigger(null);
    },
    [activeTrigger, multiline, onChange, value]
  );

  const handleChipClick = useCallback(
    async (chip) => {
      emitNamedLookupDebug('chip.activate', {
        name: chip.name,
        raw: chip.raw,
        unresolved: !!chip.unresolved,
        registryReady: registry.some((item) => item.name === chip.name),
      });
      setActiveTrigger(null);
      setRows([]);
      chipEditFocusedRef.current = false;
      setEditingChip(createEditingChipState(chip));
    },
    [registry]
  );

  const handleChipRowPick = useCallback(
    (entry, row) => {
      const token = serializeToken(entry, row);
      const label = tokenLabel(token, row?.name || row?.adOrderName || unresolvedChipLabel(entry?.name));
      let nextStored = '';
      if (editorRef.current && activeTrigger?.chipRaw) {
        nextStored = replaceChipToken(editorRef.current, activeTrigger.chipRaw, token, label);
      }
      if (!nextStored) {
        nextStored = token;
      }
      onChange(nextStored);
      lastSyncedValueRef.current = nextStored;
      setRows([]);
      setActiveTrigger(null);
    },
    [activeTrigger, onChange]
  );

  const nameMenuItems = useMemo(() => {
    if (!activeTrigger || activeTrigger.phase !== 'namePicker') return [];
    const q = String(activeTrigger.query || '').toLowerCase();
    return registry.filter((entry) => entry.name.toLowerCase().startsWith(q));
  }, [activeTrigger, registry]);

  const resolveEditingChip = useCallback(async (options = {}) => {
    const current = options.raw || options.name
      ? {
          raw: options.raw || editingChipRef.current?.raw || '',
          name: options.name || editingChipRef.current?.name || '',
          value: options.valueOverride ?? editingChipRef.current?.value ?? '',
        }
      : editingChipRef.current;
    const entry = (registryRef.current || []).find((item) => item.name === current?.name) || null;
    if (!current || !entry) return false;
    const raw = String(options.valueOverride ?? current.value ?? '').trim();
    if (!raw) {
      chipEditFocusedRef.current = false;
      setEditingChip(null);
      return true;
    }
    const token = serializeManualToken(current.name, raw);
    const resolved = applyResolvedChipToken(value, current.raw, token);
    if (!resolved.ok) {
      setEditingChip((prev) => prev ? { ...prev, error: resolved.error } : prev);
      return false;
    }
    emitNamedLookupDebug('chip.input.commit-manual', {
      raw: current.raw,
      name: current.name,
      value: raw,
    });
    const nextStored = resolved.nextStored;
    onChange(nextStored);
    lastSyncedValueRef.current = nextStored;
    chipEditFocusedRef.current = false;
    setEditingChip(null);
    return true;
  }, [onChange, value]);

  const openEditingChipLookup = useCallback(async () => {
    const current = editingChip;
    const entry = editingChipEntry;
    if (!current || !entry) return;
    chipEditFocusedRef.current = false;
    const liveInputValue = chipEditInputRef.current?.value;
    const currentValue = String(
      liveInputValue != null ? liveInputValue : (current.value || '')
    ).trim();
    const seededParameters = {};
    if (currentValue) {
      const resolveInput = String(entry?.token?.resolveInput || '').trim();
      const queryInput = String(entry?.token?.queryInput || '').trim();
      if (resolveInput) {
        seededParameters[resolveInput] = currentValue;
      } else if (queryInput) {
        seededParameters[queryInput] = currentValue;
      }
    }
    emitNamedLookupDebug('chip.lookup.open', {
      raw: current.raw,
      name: current.name,
      value: currentValue,
      liveValue: liveInputValue != null ? String(liveInputValue) : '',
      seededKeys: Object.keys(seededParameters).join(','),
    });
    console.debug(
      '[named-lookup][chip.lookup.open]',
      JSON.stringify({
        raw: current.raw,
        name: current.name,
        currentValue,
        liveValue: liveInputValue != null ? String(liveInputValue) : '',
        seededParameters,
      })
    );
    const opened = await openLookupSurface(entry, {
      chipRaw: current.raw,
      parameters: seededParameters,
    });
    if (opened) {
      setEditingChip(null);
    }
  }, [editingChip, editingChipEntry, openLookupSurface]);

  useEffect(() => {
    if (!useInlineEditor || !editorRef.current) return;
    const currentStored = serializeEditor(editorRef.current);
    const activeChipEditor = editorRef.current.querySelector?.('[data-chip-editor="true"]');
    const hasChipEditor = !!activeChipEditor;
    const activeChipRaw = activeChipEditor?.getAttribute?.('data-token') || '';
    if (shouldSkipEditorSync({
      editingChip,
      currentStored,
      lastSyncedValue: lastSyncedValueRef.current,
      nextValue: value,
      hasChipEditor,
      activeChipRaw,
    })) {
      emitNamedLookupDebug('chip.sync.skipped', {
        raw: editingChip?.raw,
        activeChipRaw,
      });
      return;
    }
    syncEditorContent(editorRef.current, segments, {
      editingChip,
      setChipEditInputElement: (node) => {
        chipEditInputRef.current = node || null;
      },
      onChipActivate: (segment) => {
        emitNamedLookupDebug('chip.event.direct', {
          name: segment.name,
          raw: segment.raw,
        });
        const parsed = parseTokens(segment.raw)[0];
        handleChipClick({
          raw: segment.raw,
          name: segment.name,
          label: parsed?.label || unresolvedChipLabel(segment.name),
          unresolved: parsed?.id === '?',
        });
      },
      onChipEditInput: (nextValue) => {
        setEditingChip((prev) => prev ? { ...prev, value: nextValue, error: '' } : prev);
      },
      onChipEditFocus: () => {
        emitNamedLookupDebug('chip.input.focus', {
          raw: editingChip?.raw,
          name: editingChip?.name,
        });
        chipEditFocusedRef.current = true;
      },
      onChipEditCommit: (nextValue, segment) => {
        if (skipChipBlurRef.current) {
          skipChipBlurRef.current = false;
          return;
        }
        emitNamedLookupDebug('chip.input.commit', {
          raw: segment?.raw,
          name: segment?.name,
          value: nextValue,
        });
        resolveEditingChip({
          valueOverride: nextValue,
          raw: segment?.raw,
          name: segment?.name,
        });
      },
      onChipEditCancel: () => {
        emitNamedLookupDebug('chip.input.cancel', {
          raw: editingChip?.raw,
          name: editingChip?.name,
        });
        chipEditFocusedRef.current = false;
        setEditingChip(null);
      },
      onChipEditLookup: () => {
        skipChipBlurRef.current = true;
        openEditingChipLookup();
      },
    });
    lastSyncedValueRef.current = value;
    if (editingChip && !disabled) {
      requestAnimationFrame(() => {
        try {
          const target = editorRef.current?.querySelector?.('[data-chip-editor="true"] input');
          try {
            editorRef.current?.blur?.();
          } catch (_) {}
          try {
            window.getSelection?.()?.removeAllRanges?.();
          } catch (_) {}
          emitNamedLookupDebug('chip.input.focus-attempt', {
            raw: editingChip.raw,
            name: editingChip.name,
            found: !!target,
          });
          target?.focus?.({ preventScroll: true });
          const nextValue = String(editingChip.value || '');
          const end = nextValue.length;
          target?.setSelectionRange?.(end, end);
          emitNamedLookupDebug('chip.input.focus-state', {
            raw: editingChip.raw,
            name: editingChip.name,
            activeTag: document?.activeElement?.tagName || '',
            activeRole: document?.activeElement?.getAttribute?.('role') || '',
          });
        } catch (_) {}
      });
    }
  }, [disabled, editingChip, openEditingChipLookup, resolveEditingChip, segments, useInlineEditor, value]);

  const popoverContent = (
    <Menu>
      {activeTrigger?.phase === 'namePicker' && nameMenuItems.length === 0 ? (
        <MenuItem disabled text="No matching lookups" />
      ) : null}
      {activeTrigger?.phase === 'namePicker'
        ? nameMenuItems.map((entry) => (
            <MenuItem
              key={entry.name}
              text={`/${entry.name}`}
              label={lookupMenuLabel(entry)}
              onClick={async () => {
                if (entry.dialogId || entry.windowId) {
                  const opened = await openLookupSurface(entry, {
                    start: activeTrigger?.start ?? 0,
                    caret: activeTrigger?.caret ?? activeTrigger?.start ?? 0,
                  });
                  if (opened) return;
                }
                pickName(entry);
              }}
            />
          ))
        : null}
      {activeTrigger?.phase === 'rowPicker' && rowsLoading ? (
        <MenuItem disabled text={<Spinner size={16} />} />
      ) : null}
      {activeTrigger?.phase === 'rowPicker' && !rowsLoading && rows.length === 0 ? (
        <MenuItem disabled text="No matches" />
      ) : null}
      {activeTrigger?.phase === 'rowPicker'
        ? rows.map((row) => {
            const entry = activeTrigger.entry;
            const label = entry?.token?.display
              ? entry.token.display.replace(/\$\{(\w+)\}/g, (_, key) => String(row?.[key] ?? ''))
              : String(row?.name || row?.adOrderName || '');
            const id = entry?.token?.store
              ? entry.token.store.replace(/\$\{(\w+)\}/g, (_, key) => String(row?.[key] ?? ''))
              : String(row?.id ?? row?.adOrderId ?? '');
            return (
              <MenuItem
                key={`${id}:${label}`}
                text={label}
                label={id}
                onClick={() =>
                  activeTrigger?.chipRaw
                    ? handleChipRowPick(entry, row)
                    : pickRow(entry, row)
                }
              />
            );
          })
        : null}
    </Menu>
  );

  const editorStyle = {
    display: 'block',
    width: '100%',
    boxSizing: 'border-box',
    borderRadius: 12,
    resize: 'none',
    minHeight: 40,
    maxHeight: '216px',
    overflowY: 'auto',
    border: '1px solid #ced9e0',
    padding: '10px 12px',
    lineHeight: 1.5,
    whiteSpace: 'pre-wrap',
    outline: 'none',
    background: disabled ? '#f5f8fa' : '#fff',
    color: '#182026',
    ...style,
  };

  return (
    <div className="named-lookup-input" style={{ width: '100%' }}>
      <Popover
        isOpen={!!activeTrigger}
        onClose={() => setActiveTrigger(null)}
        content={popoverContent}
        placement="top-start"
        minimal
        autoFocus={false}
        enforceFocus={false}
      >
        <div style={{ width: '100%', minWidth: 0 }}>
          {useInlineEditor ? (
            <div
              ref={editorRef}
              contentEditable={!disabled}
              suppressContentEditableWarning
              role="textbox"
              aria-multiline="true"
              data-testid={dataTestId || 'chat-composer-input'}
              className={className}
              onInput={handleEditableInput}
              onClick={(event) => {
                if (chipEditorFromTarget(event.target)) {
                  return;
                }
                const caret = caretOffsetWithin(editorRef.current);
                const text = editorRef.current?.innerText || '';
                const display = textBeforeCaret(text, caret);
                const slash = display.lastIndexOf(DEFAULT_TRIGGER);
                if (slash >= 0 && !/\s/.test(display.slice(slash + 1))) {
                  setActiveTrigger({ phase: 'namePicker', start: slash, caret, query: display.slice(slash + 1) });
                }
              }}
              onFocus={(event) => {
                setRetainInlineEditor(true);
                onFocus?.(event);
              }}
              onBlur={(event) => {
                if (!hasInlineChips) {
                  setRetainInlineEditor(false);
                }
                onBlur?.(event);
              }}
              style={editorStyle}
            />
          ) : (
            multiline ? (
              <textarea
                ref={inputRef}
                value={value}
                placeholder={placeholder}
                disabled={disabled}
                onChange={handlePlainInput}
                onFocus={onFocus}
                onBlur={onBlur}
                onSelect={(event) => {
                  const pos = event.target.selectionStart || 0;
                  const display = textBeforeCaret(String(event.target.value || ''), pos);
                  const slash = display.lastIndexOf(DEFAULT_TRIGGER);
                  if (slash >= 0 && !/\s/.test(display.slice(slash + 1))) {
                    setActiveTrigger({ phase: 'namePicker', start: slash, caret: pos, query: display.slice(slash + 1) });
                  }
                }}
                data-testid={dataTestId || 'chat-composer-input'}
                className={className}
                style={editorStyle}
              />
            ) : (
              <>
                {chips.length > 0 && (
                  <div className="named-lookup-chips" style={{ marginBottom: 4 }}>
                    {chips.map((chip) => (
                      editingChip?.raw === chip.raw ? (
                        <div
                          key={chip.raw + ':' + chip.id}
                          style={{ display: 'inline-flex', flexDirection: 'column', marginRight: 6, verticalAlign: 'top' }}
                        >
                          <InputGroup
                            inputRef={chipEditInputRef}
                            value={editingChip?.value || ''}
                            onChange={(event) => {
                              const nextValue = event?.target?.value ?? '';
                              setEditingChip((prev) => prev ? { ...prev, value: nextValue, error: '' } : prev);
                            }}
                            onFocus={() => {
                              chipEditFocusedRef.current = true;
                            }}
                            onBlur={(event) => {
                              if (skipChipBlurRef.current) {
                                skipChipBlurRef.current = false;
                                return;
                              }
                              emitNamedLookupDebug('chip.input.blur', {
                                raw: editingChip?.raw,
                                name: editingChip?.name,
                                value: event?.target?.value ?? '',
                              });
                              resolveEditingChip({ valueOverride: event?.target?.value ?? '' });
                            }}
                            onKeyDown={async (event) => {
                              if (event.key === 'Enter') {
                                event.preventDefault();
                                await resolveEditingChip({ valueOverride: event?.currentTarget?.value });
                              } else if (event.key === 'Escape') {
                                event.preventDefault();
                                chipEditFocusedRef.current = false;
                                setEditingChip(null);
                              }
                            }}
                            rightElement={(
                              <Button
                                icon="caret-down"
                                minimal
                                small
                                tabIndex={-1}
                                data-testid={`named-lookup-chip-browse-${chip.name}`}
                                aria-label={`Browse ${chip.name}`}
                                title={`Browse ${chip.name}`}
                                style={{
                                  borderRadius: 999,
                                  background: 'linear-gradient(180deg, #f8fbff 0%, #eef4fb 100%)',
                                  border: '1px solid #d7e3f0',
                                  color: '#4b6480',
                                  boxShadow: '0 1px 1px rgba(15, 23, 42, 0.06)',
                                  marginRight: 2,
                                }}
                                onMouseDown={(event) => {
                                  event.preventDefault();
                                  skipChipBlurRef.current = true;
                                }}
                                onClick={(event) => {
                                  event.preventDefault();
                                  openEditingChipLookup();
                                }}
                              />
                            )}
                            data-testid={`named-lookup-chip-input-${chip.name}`}
                            intent={editingChip?.error ? 'danger' : undefined}
                            style={{ minWidth: 160 }}
                          />
                          {editingChip?.error ? (
                            <div style={{ fontSize: 11, color: '#c23030', marginTop: 4, paddingLeft: 2 }}>
                              {editingChip.error}
                            </div>
                          ) : null}
                        </div>
                      ) : (
                        <Tag
                          key={chip.raw + ':' + chip.id}
                          minimal
                          interactive
                          round
                          intent={chip.unresolved ? 'warning' : 'success'}
                          style={{ marginRight: 4 }}
                          onClick={() => handleChipClick(chip)}
                        >
                          {chip.unresolved
                            ? unresolvedChipLabel(chip.name)
                            : (chip.label || unresolvedChipLabel(chip.name))}
                        </Tag>
                      )
                    ))}
                  </div>
                )}
                <InputGroup
                  inputRef={inputRef}
                  value={value}
                  placeholder={placeholder}
                  disabled={disabled}
                  onChange={handlePlainInput}
                  onSelect={(event) => {
                    const pos = event.target.selectionStart || 0;
                    const display = textBeforeCaret(String(event.target.value || ''), pos);
                    const slash = display.lastIndexOf(DEFAULT_TRIGGER);
                    if (slash >= 0 && !/\s/.test(display.slice(slash + 1))) {
                      setActiveTrigger({ phase: 'namePicker', start: slash, caret: pos, query: display.slice(slash + 1) });
                    }
                  }}
                  fill
                  data-testid={dataTestId || 'chat-composer-input'}
                  className={className}
                />
              </>
            )
          )}
        </div>
      </Popover>
      {import.meta.env?.DEV && debugEvents.length > 0 ? (
        <div
          data-testid="named-lookup-debug-events"
          style={{
            marginTop: 6,
            padding: '6px 8px',
            borderRadius: 8,
            background: '#f4f7fb',
            border: '1px solid #d8e1ec',
            color: '#4f5f73',
            fontSize: 11,
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace',
            overflowX: 'auto',
            whiteSpace: 'nowrap',
          }}
        >
          {debugEvents.map((event) => {
            const extras = [];
            if (event?.name) extras.push(event.name);
            if (event?.context) extras.push(event.context);
            if (event?.count !== undefined) extras.push(`count=${event.count}`);
            if (event?.hasOrder !== undefined) extras.push(`hasOrder=${event.hasOrder}`);
            if (event?.found !== undefined) extras.push(`found=${event.found}`);
            if (event?.reason) extras.push(event.reason);
            if (event?.value !== undefined) extras.push(`value=${JSON.stringify(event.value)}`);
            return `${event.type}${extras.length ? `(${extras.join(', ')})` : ''}`;
          }).join(' -> ')}
        </div>
      ) : null}
    </div>
  );
}
