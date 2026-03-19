/**
 * Creates a minimal Forge-compatible context for rendering tool feed containers.
 * This avoids the full Forge window lifecycle (no fetching, no connectors)
 * since feed data is pre-wired into signals by wireFeedSignals().
 */
import { getCollectionSignal, getControlSignal, getSelectionSignal, getFormSignal } from 'forge/core';

export function createFeedContext(feedId, dataSources = {}) {
  const windowId = `feed-${feedId}`;
  const dsNames = Object.keys(dataSources);
  const firstDS = dsNames[0] || '';

  const getDataSourceId = (ref) => `${windowId}DS${ref}`;

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
    const dsId = getDataSourceId(dsRef);
    return {
      identity: { ...identity, dataSourceRef: dsRef },
      metadata,
      handlers: {
        dataSource: {
          collection: getCollectionSignal(dsId),
          form: getFormSignal(dsId),
          control: getControlSignal(dsId),
          selection: getSelectionSignal(dsId, { selected: null, rowIndex: -1 }),
          peekFormData: () => {
            try { return getFormSignal(dsId).value || {}; } catch (_) { return {}; }
          },
          setFormData: ({ values }) => {
            try { getFormSignal(dsId).value = values; } catch (_) {}
          },
          setCollection: (data) => {
            try { getCollectionSignal(dsId).value = Array.isArray(data) ? data : []; } catch (_) {}
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
