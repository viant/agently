/**
 * Creates a minimal Forge-compatible context for rendering tool feed containers.
 * This avoids the full Forge window lifecycle (no fetching, no connectors)
 * since feed data is pre-wired into signals by wireFeedSignals().
 */
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal, getInputSignal } from 'forge/core';
import { chatService } from './chatService';

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

function sameNodePath(left, right) {
  const a = Array.isArray(left) ? left : [];
  const b = Array.isArray(right) ? right : [];
  if (a.length !== b.length) return false;
  for (let index = 0; index < a.length; index += 1) {
    if (a[index] !== b[index]) return false;
  }
  return true;
}

export function createFeedContext(feedId, dataSources = {}, conversationId = '') {
  const windowId = conversationId ? `feed-${feedId}-${conversationId}` : `feed-${feedId}`;
  const dsNames = Object.keys(dataSources);
  const firstDS = dsNames[0] || '';
  const dsRuntime = new Map();
  const DEFAULT_PAGE_SIZE = 3;

  const getDataSourceId = (ref) => `${windowId}DS${ref}`;
  const getSignals = (dsRef) => {
    const dsId = getDataSourceId(dsRef);
    return {
      collection: getCollectionSignal(dsId),
      control: getControlSignal(dsId),
      selection: getSelectionSignal(dsId, { selected: null, rowIndex: -1 }),
      form: getFormSignal(dsId),
      input: getInputSignal(dsId),
    };
  };
  const getRuntimeState = (dsRef) => {
    if (dsRuntime.has(dsRef)) return dsRuntime.get(dsRef);
    const state = { page: 1, filter: {}, activeFilter: '', fullCollection: [] };
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
    const fn = chatService?.[key];
    return typeof fn === 'function' ? fn : null;
  };

  // Build sub-context for each data source ref.
  function makeSubContext(dsRef) {
    const signals = getSignals(dsRef);
    const runtimeState = getRuntimeState(dsRef);
    const dsConfig = metadata.dataSource?.[dsRef] || {};
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
      const pageCount = Math.max(1, Math.ceil(rows.length / DEFAULT_PAGE_SIZE));
      runtimeState.page = Math.min(pageCount, Math.max(1, Number(runtimeState.page || 1)));
      const start = (runtimeState.page - 1) * DEFAULT_PAGE_SIZE;
      const visible = rows.slice(start, start + DEFAULT_PAGE_SIZE);
      try { signals.collection.value = visible; } catch (_) {}
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
      getSelection: () => {
        try { return signals.selection.value || { selected: null, rowIndex: -1 }; } catch (_) { return { selected: null, rowIndex: -1 }; }
      },
      peekSelection: () => {
        try { return signals.selection.peek?.() || signals.selection.value || { selected: null, rowIndex: -1 }; } catch (_) { return { selected: null, rowIndex: -1 }; }
      },
      isSelected: ({ row = null, rowIndex = -1, nodePath = null } = {}) => {
        try {
          const selection = signals.selection.peek?.() || signals.selection.value || { selected: null, rowIndex: -1, nodePath: null };
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
        return true;
      },
      setCollection: (data) => {
        runtimeState.fullCollection = Array.isArray(data) ? data : [];
        applyPagedCollection();
      },
      setSelection: ({ selected = null, rowIndex = -1, nodePath = null } = {}) => {
        try { signals.selection.value = { selected, rowIndex, nodePath }; } catch (_) {}
        return true;
      },
      setSelected: ({ selected = null, rowIndex = -1, nodePath = null } = {}) => {
        try { signals.selection.value = { selected, rowIndex, nodePath }; } catch (_) {}
        return true;
      },
      toggleSelection: ({ selected = null, row = null, rowIndex = -1, node = null, nodePath = null } = {}) => {
        const nextSelected = selected || row || node || null;
        try {
          const current = signals.selection.peek?.() || signals.selection.value || { selected: null, rowIndex: -1, nodePath: null };
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
      getPage: () => Number(runtimeState.page || 1),
      setPage: (page) => {
        const rows = resolveRows();
        const pageCount = Math.max(1, Math.ceil(rows.length / DEFAULT_PAGE_SIZE));
        runtimeState.page = Math.min(pageCount, Math.max(1, Number(page || 1)));
        applyPagedCollection();
        updateInput({ page: runtimeState.page });
        return true;
      },
      getCollectionInfo: () => {
        try { signals.collection.value; } catch (_) {}
        const rows = resolveRows();
        const totalCount = rows.length;
        const pageCount = Math.max(1, Math.ceil(totalCount / DEFAULT_PAGE_SIZE));
        return {
          pageCount,
          totalCount,
          page: Math.min(pageCount, Math.max(1, Number(runtimeState.page || 1))),
          pageSize: DEFAULT_PAGE_SIZE,
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
      on: () => () => {},
      emit: () => {},
    },
    Context: makeSubContext,
    tableSettingKey: (id) => `tf-${feedId}-${id}`,
    lookupHandler: (name) => resolveHandler(name, rootSubContext.handlers.dataSource),
  };
}
