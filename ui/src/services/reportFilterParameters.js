import {resolveReportRelativeDateRangeSpec} from 'forge/reporting';

const own = (value, key) => Object.prototype.hasOwnProperty.call(value || {}, key);
const alias = (value, fallback) => String(value || fallback || '').replace(/^filters\./, '');
const date = value => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value)
  && !Number.isNaN(Date.parse(value)) && new Date(value).toISOString().slice(0, 10) === value;

// Expand only producer-authored defaults. Explicit inputs, including blank
// values, remain edits; the host never invents an application date policy.
export function initializeReportFilterParameters(definition = {}, parameters = {}, now = new Date()) {
  const next = {...parameters};
  const builder = definition.builderDefinition?.reportBuilder || {};
  const scope = definition.state?.scopeParams || {};
  for (const predicate of builder.predicates || []) {
    const id = String(predicate.id || '');
    if (!id) continue;
    const value = own(scope, id) ? scope[id] : predicate.default;
    if (predicate.kind !== 'dateRange') {
      if (!own(next, id) && (Array.isArray(value) || ['string', 'number', 'boolean'].includes(typeof value))) next[id] = value;
      continue;
    }
    const start = alias(predicate.prefill?.start, predicate.startParamPath);
    const end = alias(predicate.prefill?.end, predicate.endParamPath);
    if (!start || !end || start === end || !value || typeof value !== 'object') continue;
    const range = date(value.start) && date(value.end) ? value : resolveReportRelativeDateRangeSpec(value, now, builder.request);
    if (!range || !date(range.start) || !date(range.end) || range.start > range.end) continue;
    if (!own(next, start)) next[start] = range.start;
    if (!own(next, end)) next[end] = range.end;
  }
  return next;
}
