// tokens.js — pure, side-effect-free helpers for the agently named-token
// grammar used by /<name> hotkey (Activation b) and authored /<name> in
// starting prompts (Activation c).
//
// Storage format:   @{name:id "label"}
//   - name:  the lookup name from GET /v1/api/lookups/registry
//   - id:    the canonical value (default: row.id) — what the form persists
//   - label: user-facing display string (from the row's display template)
//
// Two representations:
//   1. Stored (in form state, transcript, persistence)  — rich token
//   2. Sent (to LLM or tool call)                       — flattened to modelForm
//
// Nothing in this file imports React; the module is trivial to unit test
// and is identical to the algorithm exercised by lookups-test.mjs (T12-T17).

// Grammar: @{name:id "label"}. The id slot can be "?" as an authored-token
// sentinel meaning "unresolved" — used by NamedLookupInput when rewriting
// literal /<name> placeholders from a template body into clickable chips
// before the user has picked a row.
const TOKEN_RE =
  /@\{([a-zA-Z][a-zA-Z0-9_-]*):([^\s"]+)\s+"((?:[^"\\]|\\.)*)"\}/g;

/** True when the parsed token is an authored placeholder (id sentinel "?"). */
export function isUnresolvedToken(parsedOrId) {
  const id = typeof parsedOrId === 'string' ? parsedOrId : parsedOrId?.id;
  return id === '?' || id === '' || id == null;
}

/**
 * Serialize a resolved row into the stored token form.
 *
 * @param {{name: string, token?: {store?: string, display?: string, modelForm?: string}}} entry registry entry
 * @param {object} resolved  the selected row (row[k] referenced by ${k} templates)
 * @returns {string} `@{name:id "label"}`
 */
export function serializeToken(entry, resolved) {
  const tok = entry.token || {};
  const storeTpl = tok.store || '${id}';
  const displayTpl = tok.display || '${name}';
  const id = applyTemplate(storeTpl, resolved);
  const labelRaw = applyTemplate(displayTpl, resolved);
  const label = String(labelRaw).replace(/"/g, '\\"');
  return `@{${entry.name}:${id} "${label}"}`;
}

/**
 * Serialize a direct user-entered identity when we do not yet have a resolved row.
 * This keeps the typed id explicit in persisted state without inventing row data.
 *
 * @param {string} name lookup name
 * @param {string|number} value canonical identity entered by the user
 * @returns {string} `@{name:id "id"}`
 */
export function serializeManualToken(name, value) {
  const lookupName = String(name || '').trim();
  const raw = String(value ?? '').trim();
  const label = raw.replace(/"/g, '\\"');
  return `@{${lookupName}:${raw} "${label}"}`;
}

/**
 * Parse a string and return every occurrence of `@{…}`.
 *
 * @param {string} text
 * @returns {Array<{raw: string, index: number, name: string, id: string, label: string}>}
 */
export function parseTokens(text) {
  const out = [];
  if (!text) return out;
  // Reset global regex lastIndex across calls.
  const re = new RegExp(TOKEN_RE.source, 'g');
  let m;
  while ((m = re.exec(text)) !== null) {
    out.push({
      raw: m[0],
      index: m.index,
      name: m[1],
      id: m[2],
      // Un-escape internal \" sequences.
      label: m[3].replace(/\\"/g, '"'),
    });
  }
  return out;
}

/**
 * Parse authored prompt text containing literal `/<name>` occurrences and
 * return a segment list mixing plain text with unresolved picker slots.
 *
 * @param {string} text
 * @param {Array<{name: string}>} registry  output of GET /v1/api/lookups/registry
 * @returns {Array<{kind: 'text', value: string} | {kind: 'picker', entry: object, occ: number, resolved: null}>}
 */
export function parseAuthored(text, registry) {
  const parts = [];
  if (!text) return [{ kind: 'text', value: '' }];
  const names = new Set(registry.map((e) => e.name));
  const re = /\/([a-zA-Z][a-zA-Z0-9_-]*)\b/g;
  let lastIdx = 0;
  let m;
  let occ = 0;
  while ((m = re.exec(text)) !== null) {
    const name = m[1];
    if (!names.has(name)) continue;
    const entry = registry.find((e) => e.name === name);
    parts.push({ kind: 'text', value: text.slice(lastIdx, m.index) });
    parts.push({ kind: 'picker', entry, occ: occ++, resolved: null });
    lastIdx = m.index + m[0].length;
  }
  parts.push({ kind: 'text', value: text.slice(lastIdx) });
  return parts;
}

/**
 * Flatten a segmented prompt into the text that leaves for the LLM.
 *
 * Resolved pickers become their `token.modelForm` template (default `${id}`).
 * Unresolved picker segments either pass through as `/<name>` or throw when
 * the binding is `required`.
 *
 * @param {Array} parts  output of parseAuthored
 * @returns {string}
 */
export function flatten(parts) {
  return parts
    .map((p) => {
      if (p.kind === 'text') return p.value;
      if (!p.resolved) {
        if (p.entry && p.entry.required) {
          throw new Error(`unresolved required: /${p.entry.name}`);
        }
        return '/' + p.entry.name;
      }
      const tpl =
        (p.entry.token && p.entry.token.modelForm) || '${id}';
      return applyTemplate(tpl, p.resolved);
    })
    .join('');
}

/**
 * Flatten a text containing stored `@{…}` tokens for LLM submission.
 * Uses each token's matching registry entry for its modelForm template;
 * when the name is unknown, the raw token is replaced with the id (safe
 * default that never leaks labels).
 *
 * @param {string} storedText
 * @param {Array<object>} registry
 * @returns {string}
 */
export function flattenStored(storedText, registry, options = {}) {
  const allowUnresolvedRequired = options?.allowUnresolvedRequired === true;
  if (!storedText) return '';
  const byName = new Map();
  for (const e of registry) byName.set(e.name, e);
  return storedText.replace(TOKEN_RE, (raw, name, id, label) => {
    if (isUnresolvedToken(id)) {
      const entry = byName.get(name);
      if (entry && entry.required && !allowUnresolvedRequired) {
        throw new Error(`unresolved required: /${name}`);
      }
      // Non-required unresolved → pass the literal /<name> through so the
      // LLM still sees the user intent even without a concrete id.
      return '/' + name;
    }
    const entry = byName.get(name);
    const tpl = (entry && entry.token && entry.token.modelForm) || '${id}';
    return applyTemplate(tpl, { id, name: label, label });
  });
}

/**
 * Rehydrate a stored string into segments suitable for chip rendering on
 * conversation reload. No network call.
 */
export function rehydrate(storedText) {
  const parts = [];
  if (!storedText) return [{ kind: 'text', value: '' }];
  const tokens = parseTokens(storedText);
  let cursor = 0;
  for (const t of tokens) {
    if (t.index > cursor) {
      parts.push({ kind: 'text', value: storedText.slice(cursor, t.index) });
    }
    parts.push({
      kind: 'chip',
      name: t.name,
      id: t.id,
      label: t.label,
      raw: t.raw,
      unresolved: isUnresolvedToken(t.id),
    });
    cursor = t.index + t.raw.length;
  }
  if (cursor < storedText.length) {
    parts.push({ kind: 'text', value: storedText.slice(cursor) });
  }
  return parts;
}

// applyTemplate expands `${k}` placeholders in tpl using row[k].
function applyTemplate(tpl, row) {
  if (!tpl) return '';
  return String(tpl).replace(/\$\{(\w+)\}/g, (_, k) => {
    const v = row && row[k];
    return v == null ? '' : String(v);
  });
}
