export const workspaceApplicationKey = app => JSON.stringify([app.providerIdentity || '', app.id]);
export const workspaceMenuKey = (app, path) => JSON.stringify([app.providerIdentity || '', app.id, ...path]);

// Display names are a projection of the authorized layout. Theme defaults are
// workspace-owned metadata; an explicit personal name has the final say.
export function resolveLayoutLabels(layout, themeId = '', preferences = {}) {
  if (!layout) return layout;
  const defaults = layout.left?.navigation?.labelsByTheme?.[themeId] || {};
  const applications = (layout.applications || []).map(app => {
    const visit = (items, parents = []) => {
      if (!items) return items;
      const result = items.map(item => {
        const path = [...parents, item.id];
        const children = visit(item.children, path);
        const key = workspaceMenuKey(app, path);
        const title = preferences.menuLabels?.[key] || defaults[key] || item.title;
        return title === item.title && children === item.children ? item : {...item, title, ...(children ? {children} : {})};
      });
      return result.every((item, index) => item === items[index]) ? items : result;
    };
    const menus = visit(app.menus);
    const title = preferences.applicationLabels?.[workspaceApplicationKey(app)] || app.title;
    return menus === app.menus && title === app.title ? app : {...app, title, menus};
  });
  return applications.every((app, index) => app === layout.applications[index]) ? layout : {...layout, applications};
}

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
        else if (['window', 'report'].includes(menu.action?.type)) destinations.push({...menu, applicationId: app.id,
          destination: JSON.stringify([app.id, ...ids]), disabled: !!(disabled || menu.disabled)});
      }
    };
    visit(app.menus);
  }
  return destinations;
}
