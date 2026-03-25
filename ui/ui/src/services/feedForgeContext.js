/**
 * Creates a minimal Forge-compatible context for rendering tool feed containers.
 * This avoids the full Forge window lifecycle (no fetching, no connectors)
 * since feed data is pre-wired into signals by wireFeedSignals().
 */
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal, getInputSignal } from 'forge/core';

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

export function createFeedContext(feedId, dataSources = {}, conversationId = '') {
  const windowId = conversationId ? `feed-${feedId}-${conversationId}` : `feed-${feedId}`;
  const dsNames = Object.keys(dataSources);
  const firstDS = dsNames[0] || '';
  const dsRuntime = new Map();

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
    const state = { page: 1, filter: {}, activeFilter: '' };
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
    return {
      identity: { ...identity, dataSourceRef: dsRef },
      metadata,
      signals,
      handlers: {
        dataSource: {
          collection: signals.collection,
          form: signals.form,
          control: signals.control,
          selection: signals.selection,
          input: signals.input,
          peekInput: () => {
            try { return signals.input.peek?.() || signals.input.value || {}; } catch (_) { return {}; }
          },
          getCollection: () => {
            try { return signals.collection.value || []; } catch (_) { return []; }
          },
          peekCollection: () => {
            try { return signals.collection.peek?.() || signals.collection.value || []; } catch (_) { return []; }
          },
          getSelection: () => {
            try { return signals.selection.value || { selected: null, rowIndex: -1 }; } catch (_) { return { selected: null, rowIndex: -1 }; }
          },
          peekSelection: () => {
            try { return signals.selection.peek?.() || signals.selection.value || { selected: null, rowIndex: -1 }; } catch (_) { return { selected: null, rowIndex: -1 }; }
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
            try { signals.collection.value = Array.isArray(data) ? data : []; } catch (_) {}
          },
          setSelection: ({ selected = null, rowIndex = -1 } = {}) => {
            try { signals.selection.value = { selected, rowIndex }; } catch (_) {}
            return true;
          },
          setSelected: ({ selected = null, rowIndex = -1 } = {}) => {
            try { signals.selection.value = { selected, rowIndex }; } catch (_) {}
            return true;
          },
          getPage: () => Number(runtimeState.page || 1),
          setPage: (page) => {
            runtimeState.page = Math.max(1, Number(page || 1));
            updateInput({ page: runtimeState.page, fetch: true });
            return true;
          },
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
        },
        on: () => () => {},
        emit: () => {},
      },
      Context: makeSubContext,
      tableSettingKey: (id) => `tf-${feedId}-${id}`,
      lookupHandler: () => null,
    };
  }

  return {
    identity,
    metadata,
    signals: getSignals(firstDS),
    handlers: {
      dataSource: makeSubContext(firstDS).handlers.dataSource,
      on: () => () => {},
      emit: () => {},
    },
    Context: makeSubContext,
    tableSettingKey: (id) => `tf-${feedId}-${id}`,
    lookupHandler: () => null,
  };
}
