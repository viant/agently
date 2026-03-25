import { signal } from '@preact/signals-react';

export const codeDiffDialogState = signal({
  open: false,
  title: '',
  hasPrev: false,
  current: '',
  prev: '',
  diff: '',
  loading: false,
  currentUri: '',
  prevUri: ''
});

export const fileViewDialogState = signal({
  open: false,
  title: '',
  uri: '',
  content: '',
  loading: false
});

export function openCodeDiffDialog({ title = 'Changed File', current = '', prev = '', diff = '', hasPrev = false, loading = false, currentUri = '', prevUri = '' }) {
  codeDiffDialogState.value = { open: true, title, hasPrev: hasPrev || !!prev, current, prev, diff, loading, currentUri, prevUri };
}

export function closeCodeDiffDialog() {
  const value = codeDiffDialogState.peek?.() || codeDiffDialogState.value;
  codeDiffDialogState.value = { ...value, open: false };
}

export function updateCodeDiffDialog(patch) {
  const value = codeDiffDialogState.peek?.() || codeDiffDialogState.value;
  codeDiffDialogState.value = { ...value, ...(patch || {}) };
}

export function openFileViewDialog({ title = 'File', uri = '', content = '', loading = false }) {
  fileViewDialogState.value = { open: true, title, uri, content, loading };
}

export function closeFileViewDialog() {
  const value = fileViewDialogState.peek?.() || fileViewDialogState.value;
  fileViewDialogState.value = { ...value, open: false };
}

export function updateFileViewDialog(patch) {
  const value = fileViewDialogState.peek?.() || fileViewDialogState.value;
  fileViewDialogState.value = { ...value, ...(patch || {}) };
}
