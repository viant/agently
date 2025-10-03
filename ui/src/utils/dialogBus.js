import { signal } from '@preact/signals-react';

// Global dialog state for a simple code viewer/diff dialog
export const codeDiffDialogState = signal({ open: false, title: '', hasPrev: false, current: '', prev: '', diff: '', loading: false, currentUri: '', prevUri: '' });

export function openCodeDiffDialog({ title = 'Changed File', current = '', prev = '', diff = '', hasPrev = false, loading = false, currentUri = '', prevUri = '' }) {
  codeDiffDialogState.value = { open: true, title, hasPrev: hasPrev || !!prev, current, prev, diff, loading, currentUri, prevUri };
}

export function closeCodeDiffDialog() {
  const v = codeDiffDialogState.peek?.() || codeDiffDialogState.value;
  codeDiffDialogState.value = { ...v, open: false };
}

export function updateCodeDiffDialog(patch) {
  const v = codeDiffDialogState.peek?.() || codeDiffDialogState.value;
  codeDiffDialogState.value = { ...v, ...(patch || {}) };
}
