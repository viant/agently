import { endpoints } from '../endpoint';

async function loadPreferences({ context }) {
      try {
        const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
        const resp = await fetch(`${base}/v1/api/me/preferences`, { credentials: 'include' });
        const json = await resp.json().catch(() => ({}));
        const data = json?.data || {};
        // Use 'me' DataSource which this window defines
        const ds = context?.Context?.('me')?.handlers?.dataSource;
        ds?.setFormData?.({ values: data });
      } catch (_) {}
}

async function savePreferences({ context }) {
  try {
    const ds = context?.Context?.('me')?.handlers?.dataSource;
    const vals = ds?.peekFormData?.()?.values || ds?.getFormData?.() || ds?.peekFormData?.() || {};
    if (!vals || Object.keys(vals).length === 0) {
      throw new Error('empty form payload');
    }
    const payload = {
      displayName: vals.displayName || undefined,
      timezone: vals.timezone || undefined,
      defaultAgentRef: vals.defaultAgentRef || undefined,
      defaultModelRef: vals.defaultModelRef || undefined,
      defaultEmbedderRef: vals.defaultEmbedderRef || undefined,
    };
    const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
    const resp = await fetch(`${base}/v1/api/me/preferences`, {
      method: 'PATCH',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!resp.ok) throw new Error(await resp.text());
  } catch (e) {
    try { console.error('[preferences.save]', e); } catch (_) {}
  }
}

export const preferencesService = { loadPreferences, savePreferences };
