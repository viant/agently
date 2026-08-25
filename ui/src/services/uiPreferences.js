import { useSyncExternalStore } from 'react';

export const DEVELOPER_MODE_KEY = 'agently.developerMode';

const listeners = new Set();

function readDeveloperMode() {
  if (typeof window === 'undefined') return false;
  try {
    const value = String(window.localStorage?.getItem(DEVELOPER_MODE_KEY) || '').trim().toLowerCase();
    return value === 'true' || value === '1' || value === 'on';
  } catch (_) {
    return false;
  }
}

let developerMode = readDeveloperMode();

function notify() {
  for (const listener of listeners) listener();
}

export function getDeveloperMode() {
  return developerMode;
}

export function setDeveloperMode(enabled) {
  const next = enabled === true;
  if (developerMode === next) return;
  developerMode = next;
  if (typeof window !== 'undefined') {
    try { window.localStorage?.setItem(DEVELOPER_MODE_KEY, next ? 'true' : 'false'); } catch (_) {}
  }
  notify();
}

export function resetUIPreferences() {
  developerMode = false;
  if (typeof window !== 'undefined') {
    try { window.localStorage?.removeItem(DEVELOPER_MODE_KEY); } catch (_) {}
  }
  notify();
}

export function subscribeUIPreferences(listener) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useDeveloperMode() {
  return useSyncExternalStore(subscribeUIPreferences, getDeveloperMode, () => false);
}

if (typeof window !== 'undefined') {
  window.addEventListener('storage', (event) => {
    if (event.key !== DEVELOPER_MODE_KEY) return;
    const next = readDeveloperMode();
    if (next === developerMode) return;
    developerMode = next;
    notify();
  });
}
