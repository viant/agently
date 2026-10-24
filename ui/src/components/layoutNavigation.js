// Navigation URLs come from trusted catalogs; still reject executable schemes
// and credentials, and allow plain HTTP only for local development hosts.
export function resolveLayoutURL(action = {}) {
  const raw = action?.type === 'url' ? action.url : '';
  if (typeof raw !== 'string' || !raw || raw.trim() !== raw || /[\\\s]/.test(raw)) return '';
  try {
    const url = new URL(raw);
    if (url.username || url.password) return '';
    const loopback = url.hostname === 'localhost' || url.hostname === '[::1]' || /^127(?:\.\d{1,3}){3}$/.test(url.hostname);
    return url.protocol === 'https:' || (url.protocol === 'http:' && loopback) ? url.href : '';
  } catch (_) { return ''; }
}

export function openLayoutURL(action) {
  const url = resolveLayoutURL(action);
  if (!url) return false;
  window.open(url, '_blank', 'noopener,noreferrer');
  return true;
}

// Route identities come from catalog IDs, never executable window keys or inputs.
export function layoutWindowDestinations(layout = {}) {
  const destinations = [];
  for (const app of layout.applications || []) {
    const visit = (menus, path = [], disabled = app.disabled) => {
      for (const menu of menus || []) {
        const ids = [...path, menu.id];
        if (menu.children?.length) visit(menu.children, ids, disabled || menu.disabled);
        else if (menu.action?.type === 'window') destinations.push({...menu, applicationId: app.id,
          destination: JSON.stringify([app.id, ...ids]), disabled: !!(disabled || menu.disabled)});
      }
    };
    visit(app.menus);
  }
  return destinations;
}
