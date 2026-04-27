import { parseTokens } from './tokens.js';

export function createEditingChipState(chip = {}) {
  return {
    raw: String(chip?.raw || ''),
    name: String(chip?.name || ''),
    value: chip?.unresolved ? '' : String(chip?.id || ''),
    error: '',
  };
}

export function applyResolvedChipToken(currentValue = '', currentRaw = '', token = '') {
  const parsed = parseTokens(token)[0];
  if (!String(parsed?.label || '').trim()) {
    return {
      ok: false,
      error: 'The lookup resolved, but no display label was emitted.',
    };
  }
  const source = String(currentValue || '');
  const raw = String(currentRaw || '');
  const idx = source.indexOf(raw);
  if (idx < 0) {
    return {
      ok: true,
      nextStored: token,
    };
  }
  return {
    ok: true,
    nextStored: source.slice(0, idx) + token + source.slice(idx + raw.length),
  };
}

export function shouldSkipEditorSync({
  editingChip = null,
  currentStored = '',
  lastSyncedValue = '',
  nextValue = '',
  hasChipEditor = false,
  activeChipRaw = '',
} = {}) {
  if (editingChip) {
    return (
      hasChipEditor &&
      String(activeChipRaw || '') === String(editingChip.raw || '') &&
      currentStored === nextValue &&
      lastSyncedValue === nextValue
    );
  }
  if (hasChipEditor) return false;
  return currentStored === nextValue && lastSyncedValue === nextValue;
}
