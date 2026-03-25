import { useSyncExternalStore } from 'react';

const defaultCodeDiffState = {
  open: false,
  title: '',
  hasPrev: false,
  current: '',
  prev: '',
  diff: '',
  loading: false,
  currentUri: '',
  prevUri: ''
};

const defaultFileViewState = {
  open: false,
  title: '',
  uri: '',
  content: '',
  loading: false
};

function createStore(initialState) {
  let state = { ...initialState };
  const listeners = new Set();
  return {
    get() {
      return state;
    },
    set(next) {
      state = { ...next };
      for (const listener of listeners) listener();
    },
    patch(patch) {
      state = { ...state, ...(patch || {}) };
      for (const listener of listeners) listener();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    }
  };
}

const codeDiffStore = createStore(defaultCodeDiffState);
const fileViewStore = createStore(defaultFileViewState);

export function useCodeDiffDialogState() {
  return useSyncExternalStore(codeDiffStore.subscribe, codeDiffStore.get, codeDiffStore.get);
}

export function useFileViewDialogState() {
  return useSyncExternalStore(fileViewStore.subscribe, fileViewStore.get, fileViewStore.get);
}

export function openCodeDiffDialog({ title = 'Changed File', current = '', prev = '', diff = '', hasPrev = false, loading = false, currentUri = '', prevUri = '' }) {
  codeDiffStore.set({ open: true, title, hasPrev: hasPrev || !!prev, current, prev, diff, loading, currentUri, prevUri });
}

export function closeCodeDiffDialog() {
  codeDiffStore.patch({ open: false });
}

export function updateCodeDiffDialog(patch) {
  codeDiffStore.patch(patch);
}

export function openFileViewDialog({ title = 'File', uri = '', content = '', loading = false }) {
  fileViewStore.set({ open: true, title, uri, content, loading });
}

export function closeFileViewDialog() {
  fileViewStore.patch({ open: false });
}

export function updateFileViewDialog(patch) {
  fileViewStore.patch(patch);
}
