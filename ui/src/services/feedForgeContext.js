/**
 * Creates a minimal Forge-compatible context for rendering tool feed containers.
 * This avoids the full Forge window lifecycle (no fetching, no connectors)
 * since feed data is pre-wired into signals by wireFeedSignals().
 */
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal, getFormStatusSignal, getInputSignal } from 'forge/core';
import { chatService } from './chatService';
import { reportingHostServices } from './reportingHostServices';
import { dispatchForgeUIAction } from './forgeUIActions';
import { fetchDatasource } from '../components/lookups/client';
import { showToast } from './httpClient';

function normalizeSelectionMode(dataSource = {}) {
  return String(dataSource?.selectionMode || 'single').trim().toLowerCase() === 'multi' ? 'multi' : 'single';
}

function uniqueKeyFields(dataSource = {}) {
  return (Array.isArray(dataSource?.uniqueKey) ? dataSource.uniqueKey : [])
    .map((entry) => String(entry?.field || entry || '').trim())
    .filter(Boolean);
}

export function feedRowKey(row = null, dataSource = {}) {
  if (!row || typeof row !== 'object') return '';
  const fields = uniqueKeyFields(dataSource);
  if (fields.length === 0) return '';
  return fields.map((field) => JSON.stringify(row?.[field] ?? null)).join('|');
}

function rowsEqual(left, right, dataSource = {}) {
  if (left === right) return true;
  const leftKey = feedRowKey(left, dataSource);
  const rightKey = feedRowKey(right, dataSource);
  return !!leftKey && leftKey === rightKey;
}

function isRowInSelection(row, selection = [], dataSource = {}) {
  return (Array.isArray(selection) ? selection : []).some((candidate) => rowsEqual(candidate, row, dataSource));
}

function rowsWithSelectionState(rows = [], selectedRows = [], dataSource = {}) {
  const field = String(dataSource?.selection?.field || 'selected').trim() || 'selected';
  return (Array.isArray(rows) ? rows : []).map((row) => ({
    ...(row || {}),
    [field]: isRowInSelection(row, selectedRows, dataSource),
  }));
}

export function buildFeedSelectionChangePayload({
  feedId = '',
  conversationId = '',
  dataSourceRef = '',
  dataSource = {},
  rows = [],
  selectedRows = [],
  initialSelectedRows = [],
  changedRow = null,
} = {}) {
  const snapshot = rowsWithSelectionState(rows, selectedRows, dataSource);
  const selectedSnapshot = snapshot.filter((row) => row?.[String(dataSource?.selection?.field || 'selected').trim() || 'selected']);
  const unselectedSnapshot = snapshot.filter((row) => !row?.[String(dataSource?.selection?.field || 'selected').trim() || 'selected']);
  const changedRows = snapshot.filter((row) => (
    isRowInSelection(row, selectedRows, dataSource) !== isRowInSelection(row, initialSelectedRows, dataSource)
  ));
  const changedSelected = changedRow ? isRowInSelection(changedRow, selectedRows, dataSource) : false;
  const selectionConfig = dataSource?.selection && typeof dataSource.selection === 'object' ? dataSource.selection : {};
  const callback = selectionConfig?.callback && typeof selectionConfig.callback === 'object' ? selectionConfig.callback : null;
  return {
    feedId: String(feedId || '').trim(),
    conversationId: String(conversationId || '').trim(),
    dataSourceRef: String(dataSourceRef || '').trim(),
    eventName: String(callback?.eventName || selectionConfig?.eventName || 'feed_selection_changed').trim(),
    action: changedSelected ? 'selected' : 'unselected',
    row: changedRow ? { ...changedRow, [String(selectionConfig?.field || 'selected').trim() || 'selected']: changedSelected } : null,
    selectedRows: selectedSnapshot,
    unselectedRows: unselectedSnapshot,
    changedRows,
    finalDataSourceSnapshot: snapshot,
    callback,
  };
}

function setPathValue(target = {}, path = '', value) {
  const key = String(path || '').trim();
  if (!key) return { ...target };
  const parts = key.split('.').filter(Boolean);
  if (parts.length === 0) return { ...target };
  const clone = { ...target };
  let current = clone;
  for (let index = 0; index < parts.length - 1; index += 1) {
    const part = parts[index];
    const next = current?.[part];
    current[part] = next && typeof next === 'object' && !Array.isArray(next) ? { ...next } : {};
    current = current[part];
  }
  current[parts[parts.length - 1]] = value;
  return clone;
}

function getPathValue(target = null, path = '') {
  const parts = String(path || '').trim().split('.').filter(Boolean);
  let current = target;
  for (const part of parts) {
    if (current == null || typeof current !== 'object') return undefined;
    current = current?.[part];
  }
  return current;
}

function sameNodePath(left, right) {
  const a = Array.isArray(left) ? left : [];
  const b = Array.isArray(right) ? right : [];
  if (a.length !== b.length) return false;
  for (let index = 0; index < a.length; index += 1) {
    if (a[index] !== b[index]) return false;
  }
  return true;
}

export function createFeedContext(feedId, dataSources = {}, conversationId = '', options = {}) {
  const windowId = conversationId ? `feed-${feedId}-${conversationId}` : `feed-${feedId}`;
  const dsNames = Object.keys(dataSources);
  const firstDS = dsNames[0] || '';
  const dsRuntime = new Map();
  const DEFAULT_PAGE_SIZE = 3;

  const getDataSourceId = (ref) => `${windowId}DS${ref}`;
  const getSignals = (dsRef) => {
    const dsId = getDataSourceId(dsRef);
    const dataSource = dataSources?.[dsRef] || {};
    const selectionDefault = normalizeSelectionMode(dataSource) === 'multi'
      ? { selection: [] }
      : { selected: null, rowIndex: -1 };
    return {
      collection: getCollectionSignal(dsId),
      control: getControlSignal(dsId),
      selection: getSelectionSignal(dsId, selectionDefault),
      form: getFormSignal(dsId),
      formStatus: getFormStatusSignal(dsId),
      input: getInputSignal(dsId),
    };
  };
  const getRuntimeState = (dsRef) => {
    if (dsRuntime.has(dsRef)) return dsRuntime.get(dsRef);
    const configuredPageSize = Number(dataSources?.[dsRef]?.paging?.size || 0);
    const state = {
      page: 1,
      pageSize: Number.isFinite(configuredPageSize) && configuredPageSize > 0 ? configuredPageSize : 3,
      filter: {},
      activeFilter: '',
      fullCollection: [],
      initialSelectedRows: null,
    };
    dsRuntime.set(dsRef, state);
    return state;
  };

  const identity = {
    windowId,
    dataSourceRef: firstDS,
    getDataSourceId,
  };

  const metadata = {
    dataSource: { ...dataSources },
    actions: { import: () => ({}) },
    view: {},
  };

  const resolveLookupBindings = (bindings = {}) => Object.fromEntries(Object.entries(bindings || {}).map(([targetPath, binding]) => {
    const ref = String(binding?.dataSourceRef || '').trim();
    const sourceContext = ref ? makeSubContext(ref) : null;
    const source = sourceContext?.handlers?.dataSource?.peekFormData?.() || {};
    return [targetPath, getPathValue(source, binding?.path || binding?.field || targetPath)];
  }).filter(([, value]) => value !== undefined));

  const lookupHandlers = {
    search: async ({ dataSourceRef = '', query = '', queryInput = '', inputs = {}, inputBindings = {}, timeoutMs = 15000 } = {}) => {
      const ref = String(dataSourceRef || '').trim();
      if (!ref) throw new Error('Lookup datasource is required.');
      let requestInputs = { ...(inputs && typeof inputs === 'object' ? inputs : {}) };
      for (const [path, value] of Object.entries(resolveLookupBindings(inputBindings))) {
        requestInputs = setPathValue(requestInputs, path, value);
      }
      if (String(queryInput || '').trim()) requestInputs = setPathValue(requestInputs, queryInput, String(query || '').trim());
      const response = await fetchDatasource(ref, requestInputs, { timeoutMs });
      return Array.isArray(response?.rows) ? response.rows : (Array.isArray(response?.data) ? response.data : []);
    },
  };

  const resolveHandler = (name = '', localDataSource = null) => {
    const key = String(name || '').trim();
    if (!key) return null;
    if (key.startsWith('dataSource.')) {
      const method = key.slice('dataSource.'.length);
      const fn = localDataSource?.[method];
      return typeof fn === 'function' ? fn : null;
    }
    if (key.startsWith('chat.')) {
      const fn = chatService?.[key.slice('chat.'.length)];
      return typeof fn === 'function' ? fn : null;
    }
    if (key === 'feed.print') {
      return () => {
        if (typeof options?.exportPDF !== 'function') return false;
        showToast('Generating PDF from the current feed…', { intent: 'primary' });
        Promise.resolve().then(() => options.exportPDF())
          .then(() => showToast('PDF generated from the current feed.', { intent: 'success' }))
          .catch((error) => {
            console.error('Tool Feed PDF export failed', error);
            showToast(String(error?.message || error || 'PDF export failed.'), { intent: 'danger' });
          });
        return true;
      };
    }
    if (key === 'feed.submitDraft') {
      return ({ execution = {} } = {}) => {
        const state = execution?.state && typeof execution.state === 'object' ? execution.state : {};
        const formRef = String(state?.formDataSourceRef || '').trim();
        const selectionRefs = [
          ...(Array.isArray(state?.selectionDataSourceRefs) ? state.selectionDataSourceRefs : []),
          state?.selectionDataSourceRef,
        ].map((value) => String(value || '').trim()).filter((value, index, values) => value && values.indexOf(value) === index);
        const formData = formRef ? (makeSubContext(formRef)?.handlers?.dataSource?.getFormData?.() || {}) : {};
        const selections = Object.fromEntries(selectionRefs.map((ref) => [
          ref,
          makeSubContext(ref)?.handlers?.dataSource?.buildSelectionPayload?.() || {},
        ]));
        const collections = Object.fromEntries(selectionRefs.map((ref) => {
          const target = makeSubContext(ref)?.handlers?.dataSource;
          const selection = target?.getSelection?.() || {};
          return [ref, {
            rows: target?.peekFullCollection?.() || target?.peekCollection?.() || [],
            selectedRows: Array.isArray(selection?.selection) ? selection.selection : [],
          }];
        }));
        options?.onDraftSubmit?.({
          formDataSourceRef: formRef,
          formData,
          collections,
        });
        const selectionPayload = selectionRefs.length > 0 ? selections[selectionRefs[0]] : {};
        const callback = state?.callback && typeof state.callback === 'object' ? state.callback : null;
        dispatchForgeUIAction({
          ...selectionPayload,
          feedId,
          conversationId,
          eventName: String(callback?.eventName || state?.eventName || 'feed_draft_submit').trim(),
          callback,
          plannerSubmit: state?.plannerSubmit && typeof state.plannerSubmit === 'object' ? state.plannerSubmit : undefined,
          callbackContext: {
            ...(state?.context && typeof state.context === 'object' ? state.context : {}),
            ...(localDataSource?.resolveBindings?.(state?.contextBindings) || {}),
          },
          formData,
          selections,
        });
        return true;
      };
    }
    if (key === 'feed.submitSelection') {
      return ({ execution = {} } = {}) => {
        const state = execution?.state && typeof execution.state === 'object' ? execution.state : {};
        const payload = localDataSource?.buildSelectionPayload?.() || {};
        const callback = state?.callback && typeof state.callback === 'object' ? state.callback : null;
        dispatchForgeUIAction({
          ...payload,
          eventName: String(callback?.eventName || state?.eventName || payload?.eventName || 'feed_selection_submit').trim(),
          callback,
          plannerSubmit: state?.plannerSubmit && typeof state.plannerSubmit === 'object' ? state.plannerSubmit : undefined,
          callbackContext: {
            ...(state?.context && typeof state.context === 'object' ? state.context : {}),
            ...(localDataSource?.resolveBindings?.(state?.contextBindings) || {}),
          },
        });
        return true;
      };
    }
    if (key === 'feed.submitAction') {
      return ({ execution = {} } = {}) => {
        const state = execution?.state && typeof execution.state === 'object' ? execution.state : {};
        const formRef = String(state?.formDataSourceRef || '').trim();
        const selectionRefs = (Array.isArray(state?.selectionDataSourceRefs) ? state.selectionDataSourceRefs : [])
          .map((value) => String(value || '').trim()).filter(Boolean);
        const formData = formRef ? (makeSubContext(formRef)?.handlers?.dataSource?.getFormData?.() || {}) : {};
        const selections = Object.fromEntries(selectionRefs.map((ref) => [
          ref,
          makeSubContext(ref)?.handlers?.dataSource?.buildSelectionPayload?.() || {},
        ]));
        const callback = state?.callback && typeof state.callback === 'object' ? state.callback : null;
        dispatchForgeUIAction({
          feedId,
          conversationId,
          eventName: String(callback?.eventName || state?.eventName || 'feed_action').trim(),
          callback,
          plannerSubmit: state?.plannerSubmit && typeof state.plannerSubmit === 'object' ? state.plannerSubmit : undefined,
          callbackContext: {
            ...(state?.context && typeof state.context === 'object' ? state.context : {}),
            ...(localDataSource?.resolveBindings?.(state?.contextBindings) || {}),
          },
          formData,
          selections,
          selectedRows: [],
          unselectedRows: [],
          changedRows: [],
          finalDataSourceSnapshot: [],
        });
        return true;
      };
    }
    if (key === 'feed.submitInstructions') {
      return ({ execution = {} } = {}) => {
        const state = execution?.state && typeof execution.state === 'object' ? execution.state : {};
        const formData = localDataSource?.getFormData?.() || {};
        const callback = state?.callback && typeof state.callback === 'object' ? state.callback : null;
        dispatchForgeUIAction({
          feedId,
          conversationId,
          eventName: String(callback?.eventName || state?.eventName || 'feed_instructions_submit').trim(),
          callback,
          plannerSubmit: state?.plannerSubmit && typeof state.plannerSubmit === 'object' ? state.plannerSubmit : undefined,
          callbackContext: {
            ...(state?.context && typeof state.context === 'object' ? state.context : {}),
            ...(localDataSource?.resolveBindings?.(state?.contextBindings) || {}),
          },
          formData,
          selectedRows: [],
          unselectedRows: [],
          changedRows: [],
          finalDataSourceSnapshot: [],
        });
        return true;
      };
    }
    if (key === 'tool.execute') return chatService?.executeDeclaredTool || null;
    const fn = chatService?.[key];
    return typeof fn === 'function' ? fn : null;
  };

  // Build sub-context for each data source ref.
  function makeSubContext(dsRef) {
    const signals = getSignals(dsRef);
    const runtimeState = getRuntimeState(dsRef);
    const dsConfig = metadata.dataSource?.[dsRef] || {};
    const selectionMode = normalizeSelectionMode(dsConfig);
    const defaultSelection = () => (selectionMode === 'multi' ? { selection: [] } : { selected: null, rowIndex: -1 });
    const markDirty = () => {
      try {
        signals.formStatus.value = { ...(signals.formStatus.peek?.() || signals.formStatus.value || {}), dirty: true };
      } catch (_) {}
    };
    const updateInput = (next = {}) => {
      try {
        signals.input.value = {
          ...(signals.input.peek?.() || signals.input.value || {}),
          ...next,
        };
      } catch (_) {}
    };
    const resolveRows = () => {
      const runtimeRows = Array.isArray(runtimeState.fullCollection) ? runtimeState.fullCollection : [];
      if (runtimeRows.length > 0) return runtimeRows;
      try {
        const currentRows = signals.collection.peek?.() || signals.collection.value || [];
        return Array.isArray(currentRows) ? currentRows : [];
      } catch (_) {
        return [];
      }
    };
    const applyPagedCollection = () => {
      const rows = resolveRows();
      const pageSize = runtimeState.pageSize || DEFAULT_PAGE_SIZE;
      const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
      runtimeState.page = Math.min(pageCount, Math.max(1, Number(runtimeState.page || 1)));
      const start = (runtimeState.page - 1) * pageSize;
      const visible = rows.slice(start, start + pageSize);
      try { signals.collection.value = visible; } catch (_) {}
    };

    const selectedRows = () => {
      if (selectionMode !== 'multi') return [];
      try {
        const current = signals.selection.peek?.() || signals.selection.value || defaultSelection();
        return Array.isArray(current?.selection) ? current.selection : [];
      } catch (_) {
        return [];
      }
    };

    const publishSelectionChange = (changedRow = null) => {
      if (selectionMode !== 'multi') return;
      const rows = resolveRows();
      const selected = selectedRows();
      if (runtimeState.initialSelectedRows == null) {
        runtimeState.initialSelectedRows = [...selected];
      }
      const detail = buildFeedSelectionChangePayload({
        feedId,
        conversationId,
        dataSourceRef: dsRef,
        dataSource: dsConfig,
        rows,
        selectedRows: selected,
        initialSelectedRows: runtimeState.initialSelectedRows,
        changedRow,
      });
      const feedbackRef = String(dsConfig?.selection?.feedbackDataSourceRef || '').trim();
      if (feedbackRef) {
        const feedbackSignals = getSignals(feedbackRef);
        const label = String(
          detail?.row?.channel || detail?.row?.Channel
          || detail?.row?.name || detail?.row?.Name
          || detail?.row?.label || detail?.row?.Label
          || detail?.row?.id || detail?.row?.ID
          || 'Item'
        ).trim();
        // Collection replacement, select-all, and reset do not identify one
        // toggled row. Calling those events "Item excluded" is misleading (and
        // previously produced "Item excluded; 5 selected" immediately after an
        // add). Reserve included/excluded wording for an actual row toggle.
        const message = changedRow
          ? `${label} ${detail.action === 'selected' ? 'included' : 'excluded'}; ${detail.selectedRows.length} selected.`
          : `Selection updated; ${detail.selectedRows.length} selected.`;
        const feedback = { message, action: detail.action, selectedCount: detail.selectedRows.length, changedCount: detail.changedRows.length };
        try { feedbackSignals.form.value = feedback; } catch (_) {}
        try { feedbackSignals.collection.value = [feedback]; } catch (_) {}
      }
      if (detail.callback) dispatchForgeUIAction(detail);
    };
    const dataSourceHandlers = {
      collection: signals.collection,
      form: signals.form,
      control: signals.control,
      selection: signals.selection,
      input: signals.input,
      peekInput: () => {
        try { return signals.input.peek?.() || signals.input.value || {}; } catch (_) { return {}; }
      },
      getCollection: () => {
        try {
          return Array.isArray(signals.collection.value) ? signals.collection.value : [];
        } catch (_) { return []; }
      },
      peekCollection: () => {
        try {
          const rows = signals.collection.peek?.() || signals.collection.value || [];
          return Array.isArray(rows) ? rows : [];
        } catch (_) { return []; }
      },
      peekFullCollection: () => resolveRows(),
      getSelection: () => {
        try { return signals.selection.value || defaultSelection(); } catch (_) { return defaultSelection(); }
      },
      peekSelection: () => {
        try { return signals.selection.peek?.() || signals.selection.value || defaultSelection(); } catch (_) { return defaultSelection(); }
      },
      isSelected: ({ row = null, rowIndex = -1, nodePath = null } = {}) => {
        try {
          const selection = signals.selection.peek?.() || signals.selection.value || defaultSelection();
          if (selectionMode === 'multi') {
            return isRowInSelection(row, selection?.selection || [], dsConfig);
          }
          if (Array.isArray(nodePath)) {
            return sameNodePath(selection.nodePath, nodePath);
          }
          if (row && selection.selected) {
            return selection.selected === row;
          }
          return Number(selection.rowIndex) === Number(rowIndex) && rowIndex >= 0;
        } catch (_) {
          return false;
        }
      },
      peekFormData: () => {
        try { return signals.form.value || {}; } catch (_) { return {}; }
      },
      getFormData: () => {
        try { return signals.form.value || {}; } catch (_) { return {}; }
      },
      setFormData: ({ values }) => {
        try { signals.form.value = values; } catch (_) {}
      },
      setFormField: ({ item, value }) => {
        const fieldKey = item?.dataField || item?.bindingPath || item?.id || '';
        try { signals.form.value = setPathValue(signals.form.peek?.() || signals.form.value || {}, fieldKey, value); } catch (_) {}
        markDirty();
        return true;
      },
      setCollection: (data) => {
        runtimeState.fullCollection = Array.isArray(data) ? data : [];
        applyPagedCollection();
      },
      replaceCollection: ({ rows = [], selectAll = false } = {}) => {
        const nextRows = Array.isArray(rows) ? rows : [];
        if (selectionMode === 'multi' && runtimeState.initialSelectedRows == null) {
          runtimeState.initialSelectedRows = [...selectedRows()];
        }
        runtimeState.fullCollection = nextRows;
        applyPagedCollection();
        if (selectionMode === 'multi' && selectAll) {
          try { signals.selection.value = { selection: [...nextRows] }; } catch (_) {}
        }
        markDirty();
        publishSelectionChange(null);
        return true;
      },
      setSelection: ({ selected = null, rowIndex = -1, nodePath = null } = {}) => {
        try {
          signals.selection.value = selectionMode === 'multi'
            ? { selection: Array.isArray(selected) ? selected : (selected ? [selected] : []) }
            : { selected, rowIndex, nodePath };
        } catch (_) {}
        return true;
      },
      setSelected: ({ selected = null, rowIndex = -1, nodePath = null } = {}) => {
        try {
          signals.selection.value = selectionMode === 'multi'
            ? { selection: Array.isArray(selected) ? selected : (selected ? [selected] : []) }
            : { selected, rowIndex, nodePath };
        } catch (_) {}
        return true;
      },
      toggleSelection: ({ selected = null, row = null, rowIndex = -1, node = null, nodePath = null } = {}) => {
        const nextSelected = selected || row || node || null;
        try {
          const current = signals.selection.peek?.() || signals.selection.value || defaultSelection();
          if (selectionMode === 'multi') {
            const currentRows = Array.isArray(current?.selection) ? current.selection : [];
            if (runtimeState.initialSelectedRows == null) runtimeState.initialSelectedRows = [...currentRows];
            const isPresent = isRowInSelection(nextSelected, currentRows, dsConfig);
            signals.selection.value = {
              selection: isPresent
                ? currentRows.filter((candidate) => !rowsEqual(candidate, nextSelected, dsConfig))
                : [...currentRows, nextSelected],
            };
            markDirty();
            publishSelectionChange(nextSelected);
            return true;
          }
          if (Array.isArray(nodePath)) {
            signals.selection.value = sameNodePath(current.nodePath, nodePath)
              ? { selected: null, rowIndex: -1, nodePath: null }
              : { selected: nextSelected, rowIndex, nodePath };
            return true;
          }
          if (current.selected === nextSelected && Number(current.rowIndex) === Number(rowIndex)) {
            signals.selection.value = { selected: null, rowIndex: -1, nodePath: null };
          } else {
            signals.selection.value = { selected: nextSelected, rowIndex, nodePath: null };
          }
        } catch (_) {}
        return true;
      },
      setAllSelection: () => {
        if (selectionMode !== 'multi') return false;
        const rows = resolveRows();
        if (runtimeState.initialSelectedRows == null) runtimeState.initialSelectedRows = [...selectedRows()];
        try { signals.selection.value = { selection: [...rows] }; } catch (_) {}
        markDirty();
        publishSelectionChange(null);
        return true;
      },
      resetSelection: () => {
        if (selectionMode !== 'multi') return false;
        if (runtimeState.initialSelectedRows == null) runtimeState.initialSelectedRows = [...selectedRows()];
        try { signals.selection.value = { selection: [] }; } catch (_) {}
        markDirty();
        publishSelectionChange(null);
        return true;
      },
      selectIntoForm: ({ selected = null, row = null, rowIndex = -1, node = null, nodePath = null } = {}) => {
        const nextSelected = selected || row || node || null;
        try {
          signals.selection.value = {
            selected: nextSelected,
            rowIndex,
            nodePath: Array.isArray(nodePath) ? nodePath : null,
          };
        } catch (_) {}
        try {
          signals.form.value = nextSelected && typeof nextSelected === 'object' ? { ...nextSelected } : {};
        } catch (_) {}
        return true;
      },
      getPage: () => Number(runtimeState.page || 1),
      setPage: (page) => {
        const rows = resolveRows();
        const pageSize = runtimeState.pageSize || DEFAULT_PAGE_SIZE;
        const pageCount = Math.max(1, Math.ceil(rows.length / pageSize));
        runtimeState.page = Math.min(pageCount, Math.max(1, Number(page || 1)));
        applyPagedCollection();
        updateInput({ page: runtimeState.page });
        return true;
      },
      getCollectionInfo: () => {
        try { signals.collection.value; } catch (_) {}
        const rows = resolveRows();
        const totalCount = rows.length;
        const pageSize = runtimeState.pageSize || DEFAULT_PAGE_SIZE;
        const pageCount = Math.max(1, Math.ceil(totalCount / pageSize));
        return {
          pageCount,
          totalCount,
          page: Math.min(pageCount, Math.max(1, Number(runtimeState.page || 1))),
          pageSize,
        };
      },
      isInactive: () => false,
      getFilter: () => ({ ...(runtimeState.filter || {}) }),
      peekFilter: () => ({ ...(runtimeState.filter || {}) }),
      setFilter: ({ filter = {} } = {}) => {
        runtimeState.filter = { ...(filter || {}) };
        updateInput({ filter: runtimeState.filter, fetch: true });
        return true;
      },
      setFilterValue: ({ item, value } = {}) => {
        const fieldKey = item?.dataField || item?.bindingPath || item?.id || '';
        runtimeState.filter = setPathValue(runtimeState.filter || {}, fieldKey, value);
        updateInput({ filter: runtimeState.filter });
        return true;
      },
      setSilentFilterValue: ({ item, value } = {}) => {
        const fieldKey = item?.dataField || item?.bindingPath || item?.id || '';
        runtimeState.filter = setPathValue(runtimeState.filter || {}, fieldKey, value);
        updateInput({ filter: runtimeState.filter });
        return true;
      },
      getFilterSet: () => Array.isArray(dsConfig?.filterSet) ? dsConfig.filterSet : [],
      getFilterSets: () => Array.isArray(dsConfig?.filterSet) ? dsConfig.filterSet : [],
      getActiveFilter: () => {
        const filterSets = Array.isArray(dsConfig?.filterSet) ? dsConfig.filterSet : [];
        return filterSets.find((entry) => entry?.default) || null;
      },
      setActiveFilter: ({ execution } = {}) => {
        runtimeState.activeFilter = String(execution?.args?.[0] || '').trim();
        return true;
      },
      fetchCollection: () => true,
      refreshSelection: ({ filter = {} } = {}) => {
        runtimeState.filter = { ...(filter || {}) };
        updateInput({ filter: runtimeState.filter, refresh: true });
        return true;
      },
      peekLoading: () => {
        try { return !!(signals.control.peek?.() || signals.control.value || {}).loading; } catch (_) { return false; }
      },
      getLoading: () => {
        try { return !!(signals.control.value || {}).loading; } catch (_) { return false; }
      },
      setLoading: (loading) => {
        try { signals.control.value = { ...(signals.control.peek?.() || signals.control.value || {}), loading: !!loading }; } catch (_) {}
      },
      peekError: () => {
        try { return (signals.control.peek?.() || signals.control.value || {}).error || null; } catch (_) { return null; }
      },
      getError: () => {
        try { return (signals.control.value || {}).error || null; } catch (_) { return null; }
      },
      setError: (error) => {
        try { signals.control.value = { ...(signals.control.peek?.() || signals.control.value || {}), error: error ? String(error) : null, loading: false }; } catch (_) {}
      },
      buildSelectionPayload: () => buildFeedSelectionChangePayload({
        feedId,
        conversationId,
        dataSourceRef: dsRef,
        dataSource: dsConfig,
        rows: resolveRows(),
        selectedRows: selectedRows(),
        initialSelectedRows: runtimeState.initialSelectedRows || selectedRows(),
      }),
      resolveBindings: (bindings = {}) => {
        const result = {};
        for (const [key, raw] of Object.entries(bindings && typeof bindings === 'object' ? bindings : {})) {
          const binding = typeof raw === 'string' ? { dataSourceRef: dsRef, path: raw } : (raw || {});
          const ref = String(binding?.dataSourceRef || dsRef).trim();
          const refSignals = getSignals(ref);
          const form = refSignals.form.peek?.() || refSignals.form.value || {};
          const collection = refSignals.collection.peek?.() || refSignals.collection.value || [];
          const root = form && Object.keys(form).length > 0 ? form : (Array.isArray(collection) ? collection[0] : collection);
          const value = getPathValue(root, binding?.path || binding?.field || key);
          if (value !== undefined) result[key] = value;
        }
        return result;
      },
      peekDataSourceValue: (scope) => {
        if (scope === 'collection') return signals.collection.peek?.() || signals.collection.value || [];
        if (scope === 'filter') return { ...(runtimeState.filter || {}) };
        return signals.form.peek?.() || signals.form.value || {};
      },
    };
    return {
      identity: { ...identity, dataSourceRef: dsRef },
      dataSource: dsConfig,
      dataSources: metadata.dataSource,
      metadata,
      signals,
      handlers: {
        dataSource: dataSourceHandlers,
        filePreview: { open: (payload) => chatService?.onChangedFileSelect?.({
          uri: payload?.currentUri,
          origUri: payload?.previousUri,
          diff: payload?.diff,
          modes: payload?.modes,
          defaultMode: payload?.preview?.defaultMode,
          previewTool: payload?.tool,
          conversationId,
          item: payload?.row,
        }) },
        ...reportingHostServices,
        lookup: lookupHandlers,
        on: () => () => {},
        emit: () => {},
      },
      Context: makeSubContext,
      tableSettingKey: (id) => `tf-${feedId}-${id}`,
      lookupHandler: (name) => resolveHandler(name, dataSourceHandlers),
    };
  }

  const rootSubContext = makeSubContext(firstDS);

  return {
    identity,
    dataSource: metadata.dataSource?.[firstDS] || {},
    dataSources: metadata.dataSource,
    metadata,
    signals: getSignals(firstDS),
    handlers: {
      dataSource: rootSubContext.handlers.dataSource,
      filePreview: rootSubContext.handlers.filePreview,
      ...reportingHostServices,
      lookup: lookupHandlers,
      on: () => () => {},
      emit: () => {},
    },
    Context: makeSubContext,
    tableSettingKey: (id) => `tf-${feedId}-${id}`,
    lookupHandler: (name) => resolveHandler(name, rootSubContext.handlers.dataSource),
  };
}
