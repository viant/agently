const SETTINGS_PATH = '/ui/settings';

export function safeUIReturnPath(value = '') {
  const candidate = String(value || '').trim();
  if (!candidate.startsWith('/') || candidate.startsWith('//')) return '/';
  if (candidate === '/ui/workspace' || candidate.startsWith('/ui/workspace?') || candidate.startsWith('/ui/workspace#')) return '/';
  if (candidate === SETTINGS_PATH || candidate.startsWith(`${SETTINGS_PATH}?`) || candidate.startsWith(`${SETTINGS_PATH}#`)) return '/';
  return candidate;
}

export function uiSettingsHref(locationLike = globalThis?.window?.location) {
  const pathname = String(locationLike?.pathname || '/');
  const search = String(locationLike?.search || '');
  const hash = String(locationLike?.hash || '');
  const returnTo = safeUIReturnPath(`${pathname}${search}${hash}`);
  return `${SETTINGS_PATH}?returnTo=${encodeURIComponent(returnTo)}`;
}

export function uiSettingsReturnHref(locationLike = globalThis?.window?.location) {
  const search = String(locationLike?.search || '');
  try {
    return safeUIReturnPath(new URLSearchParams(search).get('returnTo') || '/');
  } catch (_) {
    return '/';
  }
}

export function workspaceSettingsHref(locationLike = globalThis?.window?.location) {
 const source = uiSettingsHref(locationLike);
 return source.replace('/ui/settings?', '/ui/workspace?');
}
