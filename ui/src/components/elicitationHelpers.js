export function prepareRequestedSchema(requestedSchema = null) {
  try {
    if (!requestedSchema || typeof requestedSchema !== 'object') return requestedSchema;
    const clone = JSON.parse(JSON.stringify(requestedSchema));
    const props = (clone.properties = clone.properties || {});
    Object.keys(props).forEach((key) => {
      const p = props[key];
      if (!p || typeof p !== 'object') return;
      const t = (p.type || '').toLowerCase();
      if (t === 'array') {
        if (p.default === undefined) p.default = [];
        if (p.default && !Array.isArray(p.default)) p.default = [];
      } else if (t === 'object') {
        if (p.default === undefined) p.default = {};
      }
    });
    return clone;
  } catch (_) {
    return requestedSchema;
  }
}

export function elicitationDataBindingKey(elicitationId = '') {
  return `window.state.answers.elic_${String(elicitationId || 'local').trim() || 'local'}`;
}

export function collectElicitationFormValues({ dataBindingKey = '', formWrapperId = '', schema = null, trackedValues = {} } = {}) {
  try {
    const path = String(dataBindingKey || '').split('.');
    let obj = window;
    for (const seg of path.slice(1)) {
      obj = obj?.[seg];
    }
    if (obj && typeof obj === 'object') {
      const values = obj?.values || obj?.data || obj;
      if (Object.keys(values).length > 0) return values;
    }
  } catch (_) {}

  if (trackedValues && typeof trackedValues === 'object' && Object.keys(trackedValues).length > 0) {
    return trackedValues;
  }

  try {
    const root = typeof document !== 'undefined' ? document.getElementById(formWrapperId) : null;
    if (!root) return {};
    const out = {};
    const fields = root.querySelectorAll('input, select, textarea');
    fields.forEach((el) => {
      const name = el.name || el.getAttribute('data-field') || el.id || '';
      if (!name) return;
      const type = (el.getAttribute('type') || '').toLowerCase();
      if (type === 'checkbox') out[name] = !!el.checked;
      else out[name] = el.value;
    });
    if (schema?.properties) {
      for (const key of Object.keys(schema.properties)) {
        if (out[key] !== undefined) continue;
        const sel = [`[name="${key}"]`, `[id="${key}"]`, `[data-field="${key}"]`].join(',');
        const el = root.querySelector(sel);
        if (el) out[key] = el.value;
      }
    }
    return out;
  } catch (_) {
    return {};
  }
}

export function triggerElicitationFormSubmit(formWrapperId = '') {
  try {
    const root = typeof document !== 'undefined' ? document.getElementById(formWrapperId) : null;
    if (!root) return false;
    const btn = root.querySelector('button[type="submit"], input[type="submit"]');
    if (btn) {
      btn.click();
      return true;
    }
    const form = root.querySelector('form');
    if (form) {
      if (typeof form.requestSubmit === 'function') {
        form.requestSubmit();
        return true;
      }
      if (typeof form.submit === 'function') {
        form.submit();
        return true;
      }
    }
  } catch (_) {}
  return false;
}
