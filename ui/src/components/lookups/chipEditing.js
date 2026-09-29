import { parseTokens, serializeManualToken } from './tokens.js';

export function createEditingChipState(chip = {}) {
  const parsed = parseTokens(String(chip.raw || ''))[0];
  const id = chip.id ?? parsed?.id;
  return {
    raw: String(chip?.raw || ''),
    name: String(chip?.name || ''),
    value: chip?.unresolved || id === '?' ? '' : String(id || ''),
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
      ok: false,
      error: 'The original chip is no longer in the draft. Your text was preserved.',
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
  chipCountMatches = true,
} = {}) {
  if (editingChip) {
    return (
      hasChipEditor &&
      String(activeChipRaw || '') === String(editingChip.raw || '') &&
      currentStored === nextValue &&
      lastSyncedValue === nextValue
    );
  }
  if (hasChipEditor || !chipCountMatches) return false;
  return currentStored === nextValue && lastSyncedValue === nextValue;
}

export function unwrapLookupSelection(record) {
  if (record?.canceled || ['canceled', 'busy'].includes(record?.status)) return null;
  if (Array.isArray(record)) return record.length ? unwrapLookupSelection(record[0]) : null;
  if (!record || typeof record !== 'object') return record;
  if (record.selected) return record.selected;
  if (Array.isArray(record.selection) && record.selection.length) return unwrapLookupSelection(record.selection[0]);
  return record;
}

export function draftWithEditedChip(draft, chip, inputValue) {
  const value = String(inputValue ?? '').trim();
  if (!chip || !value || value === parseTokens(chip.raw)[0]?.id) return draft;
  const result = applyResolvedChipToken(draft, chip.raw, serializeManualToken(chip.name, value));
  return result.ok ? result.nextStored : draft;
}
