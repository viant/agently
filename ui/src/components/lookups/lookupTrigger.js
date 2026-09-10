export const DEFAULT_LOOKUP_TRIGGER = '/';

const normalize = (value = '') => String(value || '').trim().toLowerCase();
const compact = (value = '') => normalize(value).replace(/[^a-z0-9]+/g, '');

export function findLookupTriggerStart(value = '', trigger = DEFAULT_LOOKUP_TRIGGER) {
  const text = String(value || '');
  for (let index = text.length - 1; index >= 0; index -= 1) {
    if (text[index] !== trigger) continue;
    if (index === 0 || /[\s([{]/.test(text[index - 1])) return index;
  }
  return -1;
}

export function filterLookupRegistry(registry = [], query = '') {
  const q = normalize(query);
  if (!q) return Array.isArray(registry) ? registry : [];
  const compactQuery = compact(q);
  return (Array.isArray(registry) ? registry : [])
    .map((entry, index) => {
      const name = normalize(entry?.name);
      const fields = [entry?.name, entry?.title, entry?.label, entry?.description]
        .map(normalize)
        .filter(Boolean);
      const compactFields = fields.map(compact);
      let score = Number.POSITIVE_INFINITY;
      if (compact(name) === compactQuery) score = 0;
      else if (compact(name).startsWith(compactQuery)) score = 1;
      else if (fields.some((field) => field.split(/[^a-z0-9]+/).some((word) => word.startsWith(q)))) score = 2;
      else if (fields.some((field) => field.includes(q)) || compactFields.some((field) => field.includes(compactQuery))) score = 3;
      return {entry, index, score};
    })
    .filter((candidate) => Number.isFinite(candidate.score))
    .sort((left, right) => left.score - right.score || left.index - right.index)
    .map((candidate) => candidate.entry);
}
