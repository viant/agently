export function resolveWorkspaceWindowTitle(windowEntry = null, metadata = null) {
  const explicitTitle = String(windowEntry?.windowTitle || '').trim();
  const windowKey = String(windowEntry?.windowKey || '').trim();
  const resourceKey = /^window:\/\//i.test(windowKey);
  const placeholder = !explicitTitle || explicitTitle === windowKey
    || /^window:\/\//i.test(explicitTitle)
    || (resourceKey && /^Window: /i.test(explicitTitle));
  if (!placeholder) return explicitTitle;
  const authoredTitle = String(metadata?.window?.title || metadata?.title || '').trim();
  const chipName = String(windowEntry?.navigation?.chipName || metadata?.window?.chipName || metadata?.chipName || '').trim();
  const navigationLabel = String(windowEntry?.navigation?.label || '').trim();
  if (authoredTitle || chipName || navigationLabel) return authoredTitle || chipName || navigationLabel;
  if (resourceKey) {
    return windowKey.split('/').filter(Boolean).pop().replace(/([a-z0-9])([A-Z])/g, '$1 $2')
      .replace(/[_-]+/g, ' ').replace(/\b\w/g, character => character.toUpperCase());
  }
  return explicitTitle || windowKey;
}
