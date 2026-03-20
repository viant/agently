function normalizeEntry(entry) {
  if (entry && typeof entry === 'object') {
    const id = String(entry.id || entry.value || entry.name || '').trim()
    const name = String(entry.label || entry.name || entry.title || id).trim()
    return {
      id,
      name,
      modelRef: String(entry.modelRef || entry.model || '').trim()
    }
  }
  const id = String(entry || '').trim()
  return { id, name: id, modelRef: '' }
}

function humanizeKey(value) {
  const raw = String(value || '').trim()
  if (!raw) return ''
  const spaced = raw
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[._/-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  if (!spaced) return raw
  return spaced
    .split(' ')
    .filter(Boolean)
    .map((word) => {
      const lower = word.toLowerCase()
      if (lower.length <= 3 && lower === word) return lower.toUpperCase()
      return lower.charAt(0).toUpperCase() + lower.slice(1)
    })
    .join(' ')
}

function comparableKey(value) {
  return humanizeKey(value).toLowerCase().replace(/\s+/g, ' ').trim()
}

function displayAgentLabel(entry) {
  const normalized = normalizeEntry(entry)
  if (!normalized.id) return ''
  const label = normalized.name || normalized.id
  if (comparableKey(label) === comparableKey(normalized.id)) {
    return humanizeKey(normalized.id)
  }
  return label
}

function displayModelLabel(entry) {
  const normalized = normalizeEntry(entry)
  if (!normalized.id) return ''
  const rawLabel = normalized.name || normalized.id
  if (!rawLabel) return normalized.id
  if (comparableKey(rawLabel) === comparableKey(normalized.id)) {
    return normalized.id
  }
  const cleaned = rawLabel
    .replace(/\s+\([^)]*\)\s*$/, '')
    .replace(/\s+/g, ' ')
    .trim()
  return cleaned || rawLabel
}

function displayLabel(entry, kind = 'generic') {
  const normalized = normalizeEntry(entry)
  if (!normalized.id) return ''
  if (kind === 'model') return displayModelLabel(entry)
  if (kind === 'agent') return displayAgentLabel(entry)
  if (kind === 'generic') return normalized.name || normalized.id
  return normalized.name || normalized.id
}

export function normalizeWorkspaceAgentInfos(entries = []) {
  return (Array.isArray(entries) ? entries : []).map((entry) => {
    const normalized = normalizeEntry(entry)
    if (!normalized.id) return null
    return {
      ...(entry && typeof entry === 'object' ? entry : {}),
      id: normalized.id,
      name: displayAgentLabel(entry),
      modelRef: normalized.modelRef,
      model: normalized.modelRef
    }
  }).filter(Boolean)
}

export function normalizeWorkspaceModelInfos(entries = []) {
  return (Array.isArray(entries) ? entries : []).map((entry) => {
    const normalized = normalizeEntry(entry)
    if (!normalized.id) return null
    return {
      ...(entry && typeof entry === 'object' ? entry : {}),
      id: normalized.id,
      name: displayModelLabel(entry)
    }
  }).filter(Boolean)
}

export function normalizeWorkspaceAgentOptions(entries = [], defaultAgent = '') {
  return (Array.isArray(entries) ? entries : []).map((entry) => {
    const normalized = normalizeEntry(entry)
    if (!normalized.id) return null
    return {
      ...(entry && typeof entry === 'object' ? entry : {}),
      value: normalized.id,
      label: displayAgentLabel(entry),
      modelRef: normalized.modelRef,
      default: normalized.id === defaultAgent
    }
  }).filter(Boolean)
}

export function normalizeWorkspaceModelOptions(entries = [], defaultModel = '') {
  return (Array.isArray(entries) ? entries : []).map((entry) => {
    const normalized = normalizeEntry(entry)
    if (!normalized.id) return null
    return {
      ...(entry && typeof entry === 'object' ? entry : {}),
      value: normalized.id,
      label: displayModelLabel(entry),
      default: normalized.id === defaultModel
    }
  }).filter(Boolean)
}

export function filterLookupCollection(collection = [], query = '', fields = []) {
  const normalizedQuery = String(query || '').trim().toLowerCase()
  const rows = Array.isArray(collection) ? collection : []
  if (!normalizedQuery) return rows
  const keys = Array.isArray(fields) && fields.length > 0 ? fields : ['id', 'name']
  return rows.filter((entry) => keys.some((field) => String(entry?.[field] || '').toLowerCase().includes(normalizedQuery)))
}

export { displayAgentLabel, displayModelLabel, displayLabel }
