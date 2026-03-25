import { client } from './agentlyClient';
import { getLogger } from 'forge/utils/logger';
import { registerDynamicEvaluator } from 'forge/runtime/binding';
import { getBusSignal } from 'forge/core';
import { showToast } from './httpClient';
import {
  filterLookupCollection,
  normalizeWorkspaceAgentInfos,
  normalizeWorkspaceModelInfos
} from './workspaceMetadata';

const log = getLogger('agently');

const DEFAULT_TIMEZONE = 'UTC';
const DEFAULT_CALENDAR_TIME = '09:00 AM';
const DEFAULT_CALENDAR_INTERVAL_HOURS = 2;
const DEFAULT_ELAPSED_INTERVAL_VALUE = 24;
const DEFAULT_ELAPSED_INTERVAL_UNIT = 'hours';
const DEFAULT_TIMEOUT_SECONDS = 300;
const SCHEDULE_TOAST_TTL_MS = 20000;
const ALL_WEEKDAYS = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
const CALENDAR_BUILDER_FIELDS = new Set(['calendarPattern', 'calendarTime', 'calendarIntervalHours', 'weekdays']);
const ELAPSED_BUILDER_FIELDS = new Set(['elapsedIntervalValue', 'elapsedIntervalUnit']);
const MODE_VALIDATION_FIELDS = new Set(['calendarTime', 'calendarIntervalHours', 'elapsedIntervalValue', 'elapsedIntervalUnit', 'cronExpr']);
const VALIDATION_FIELD_TAB = {
  name: 'general',
  agentRef: 'general',
  taskPrompt: 'general',
  taskPromptUri: 'execution',
  calendarTime: 'schedule',
  calendarIntervalHours: 'schedule',
  elapsedIntervalValue: 'schedule',
  elapsedIntervalUnit: 'schedule',
  cronExpr: 'schedule',
  startAt: 'schedule',
  endAt: 'schedule'
};

function newScheduleID() {
  const generated = globalThis?.crypto?.randomUUID?.();
  if (generated) return generated;
  return `sched_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`;
}

function normalizeVisibility(raw) {
  const value = String(raw || '').trim().toLowerCase();
  return value || undefined;
}

function normalizeScheduleRow(r = {}) {
  const row = {
    _rawScheduleType: firstDefined(r, ['scheduleType', 'schedule_type']),
    _rawCronExpr: firstDefined(r, ['cronExpr', 'cron_expr']),
    _rawIntervalSeconds: firstDefined(r, ['intervalSeconds', 'interval_seconds']),
    id: firstDefined(r, ['id']),
    name: firstDefined(r, ['name']),
    description: firstDefined(r, ['description']),
    visibility: firstDefined(r, ['visibility']) || 'private',
    agentRef: firstDefined(r, ['agentRef', 'agent_ref']),
    modelOverride: firstDefined(r, ['modelOverride', 'model_override']),
    enabled: asBoolean(firstDefined(r, ['enabled'])),
    startAt: firstDefined(r, ['startAt', 'start_at']),
    endAt: firstDefined(r, ['endAt', 'end_at']),
    scheduleType: firstDefined(r, ['scheduleType', 'schedule_type']),
    cronExpr: firstDefined(r, ['cronExpr', 'cron_expr']),
    intervalSeconds: firstDefined(r, ['intervalSeconds', 'interval_seconds']),
    timezone: firstDefined(r, ['timezone']),
    timeoutSeconds: firstDefined(r, ['timeoutSeconds', 'timeout_seconds']),
    taskPromptUri: firstDefined(r, ['taskPromptUri', 'task_prompt_uri']),
    taskPrompt: firstDefined(r, ['taskPrompt', 'task_prompt']),
    nextRunAt: firstDefined(r, ['nextRunAt', 'next_run_at']),
    lastStatus: firstDefined(r, ['lastStatus', 'last_status']),
    lastRunAt: firstDefined(r, ['lastRunAt', 'last_run_at']),
    createdAt: firstDefined(r, ['createdAt', 'created_at']),
    updatedAt: firstDefined(r, ['updatedAt', 'updated_at']),
    userCredUrl: firstDefined(r, ['userCredUrl', 'user_cred_url'])
  };
  return ensureEditorState(row);
}

function unwrapSchedulePayload(payload) {
  if (payload && typeof payload === 'object' && payload.data && typeof payload.data === 'object' && !Array.isArray(payload.data)) {
    return payload.data;
  }
  return payload && typeof payload === 'object' ? payload : null;
}

function syncSavedSchedule(ds, payload) {
  const raw = unwrapSchedulePayload(payload);
  if (!raw || typeof raw !== 'object') return null;
  const normalized = normalizeScheduleRow(raw);
  updateFormIfChanged(ds, normalized);

  const currentCollection = ds?.peekCollection?.() || ds?.getCollection?.();
  let nextRowIndex = -1;
  if (Array.isArray(currentCollection) && typeof ds?.setCollection === 'function') {
    const nextCollection = currentCollection.slice();
    const index = nextCollection.findIndex((item) => String(item?.id || '') === String(normalized.id || ''));
    if (index >= 0) {
      nextCollection[index] = { ...nextCollection[index], ...normalized };
      nextRowIndex = index;
    } else {
      nextCollection.push(normalized);
      nextRowIndex = nextCollection.length - 1;
    }
    ds.setCollection(nextCollection);
  }

  if (nextRowIndex < 0) {
    const selection = ds?.peekSelection?.() || ds?.getSelection?.() || {};
    nextRowIndex = Number.isInteger(selection?.rowIndex) ? selection.rowIndex : -1;
  }

  if (typeof ds?.setSelected === 'function' && nextRowIndex >= 0) {
    ds.setSelected({ selected: normalized, rowIndex: nextRowIndex });
  }

  return normalized;
}

function joinURL(base, path) {
  const b = (base || '').replace(/\/+$/, '');
  const p = (path || '').replace(/^\/+/, '');
  return `${b}/${p}`;
}

const WEEKDAY_TO_CRON = {
  sun: 0,
  mon: 1,
  tue: 2,
  wed: 3,
  thu: 4,
  fri: 5,
  sat: 6
};

const CRON_TO_WEEKDAY = {
  0: 'sun',
  1: 'mon',
  2: 'tue',
  3: 'wed',
  4: 'thu',
  5: 'fri',
  6: 'sat',
  7: 'sun'
};

const WEEKDAY_ORDER = ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];

function asBoolean(value) {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value === 1;
  if (typeof value === 'string') {
    const v = value.trim().toLowerCase();
    if (v === 'true' || v === '1' || v === 'yes' || v === 'on' || v === 'enabled') return true;
    if (v === 'false' || v === '0' || v === 'no' || v === 'off' || v === 'disabled') return false;
  }
  return !!value;
}

function firstDefined(obj, keys = []) {
  for (const k of keys) {
    if (obj && Object.prototype.hasOwnProperty.call(obj, k) && obj[k] !== undefined) {
      return obj[k];
    }
  }
  return undefined;
}

function parseTimeTo24h(value) {
  const raw = String(value || '').trim();
  if (!raw) return null;
  let match = raw.match(/^(\d{1,2}):(\d{2})$/);
  if (match) {
    const hh = Number(match[1]);
    const mm = Number(match[2]);
    if (Number.isFinite(hh) && Number.isFinite(mm) && hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59) {
      return { hh, mm };
    }
  }
  match = raw.match(/^(\d{1,2}):(\d{2})\s*([aApP][mM])$/);
  if (match) {
    let hh = Number(match[1]);
    const mm = Number(match[2]);
    const ap = String(match[3]).toLowerCase();
    if (!(Number.isFinite(hh) && Number.isFinite(mm) && hh >= 1 && hh <= 12 && mm >= 0 && mm <= 59)) return null;
    if (ap === 'pm' && hh !== 12) hh += 12;
    if (ap === 'am' && hh === 12) hh = 0;
    return { hh, mm };
  }
  return null;
}

function buildUTCDate(year, month, day, hours = 0, minutes = 0, seconds = 0, milliseconds = 0) {
  const d = new Date(Date.UTC(year, month - 1, day, hours, minutes, seconds, milliseconds));
  if (Number.isNaN(d.getTime())) return null;
  if (
    d.getUTCFullYear() !== year ||
    d.getUTCMonth() !== month - 1 ||
    d.getUTCDate() !== day ||
    d.getUTCHours() !== hours ||
    d.getUTCMinutes() !== minutes ||
    d.getUTCSeconds() !== seconds
  ) {
    return null;
  }
  return d;
}

function toUTCISOStringFromLocalParts(value) {
  if (!(value instanceof Date) || Number.isNaN(value.getTime())) return null;
  return buildUTCDate(
    value.getFullYear(),
    value.getMonth() + 1,
    value.getDate(),
    value.getHours(),
    value.getMinutes(),
    value.getSeconds(),
    value.getMilliseconds()
  )?.toISOString() || null;
}

function parseScheduleDateTimeString(raw) {
  const value = String(raw || '').trim();
  if (!value) return null;

  if (/[zZ]$|[+-]\d{2}:\d{2}$|[+-]\d{4}$/.test(value)) {
    const explicit = new Date(value);
    return Number.isNaN(explicit.getTime()) ? null : explicit;
  }

  let match = value.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (match) {
    return buildUTCDate(Number(match[1]), Number(match[2]), Number(match[3]));
  }

  match = value.match(/^(\d{4})-(\d{2})-(\d{2})[T\s](\d{2}):(\d{2})(?::(\d{2}))?(?:\.(\d{1,3}))?$/);
  if (match) {
    return buildUTCDate(
      Number(match[1]),
      Number(match[2]),
      Number(match[3]),
      Number(match[4]),
      Number(match[5]),
      Number(match[6] || 0),
      Number(String(match[7] || '0').padEnd(3, '0'))
    );
  }

  match = value.match(/^(\d{1,2})\/(\d{1,2})\/(\d{4})(?:,\s*|\s+)?(?:(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([aApP][mM])?)?$/);
  if (match) {
    let hours = Number(match[4] || 0);
    const minutes = Number(match[5] || 0);
    const seconds = Number(match[6] || 0);
    const meridiem = String(match[7] || '').toLowerCase();
    if (meridiem) {
      if (hours < 1 || hours > 12) return null;
      if (meridiem === 'pm' && hours !== 12) hours += 12;
      if (meridiem === 'am' && hours === 12) hours = 0;
    }
    return buildUTCDate(
      Number(match[3]),
      Number(match[1]),
      Number(match[2]),
      hours,
      minutes,
      seconds
    );
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return null;
  return buildUTCDate(
    parsed.getFullYear(),
    parsed.getMonth() + 1,
    parsed.getDate(),
    parsed.getHours(),
    parsed.getMinutes(),
    parsed.getSeconds(),
    parsed.getMilliseconds()
  );
}

function normalizeScheduleDateTime(value, label) {
  if (value === undefined || value === null) return undefined;
  if (typeof value === 'string' && !value.trim()) return undefined;
  if (value instanceof Date) {
    const normalized = toUTCISOStringFromLocalParts(value);
    if (normalized) return normalized;
    throw new Error(`${label} must be a valid date/time`);
  }
  if (typeof value === 'number') {
    const d = new Date(value);
    if (!Number.isNaN(d.getTime())) return d.toISOString();
    throw new Error(`${label} must be a valid date/time`);
  }
  if (typeof value === 'string') {
    const parsed = parseScheduleDateTimeString(value);
    if (parsed) return parsed.toISOString();
    throw new Error(`${label} must be a valid date/time`);
  }
  if (typeof value === 'object') {
    if ('date' in value) return normalizeScheduleDateTime(value.date, label);
    if ('value' in value) return normalizeScheduleDateTime(value.value, label);
    if (typeof value.toISOString === 'function') {
      const result = value.toISOString();
      if (typeof result === 'string' && result.trim()) return result;
    }
  }
  throw new Error(`${label} must be a valid date/time`);
}

function toAmPm(hh, mm) {
  const safeH = Number.isFinite(hh) ? hh : 9;
  const safeM = Number.isFinite(mm) ? mm : 0;
  const ampm = safeH >= 12 ? 'PM' : 'AM';
  const h12 = (safeH % 12) || 12;
  const mmPadded = String(safeM).padStart(2, '0');
  return `${String(h12).padStart(2, '0')}:${mmPadded} ${ampm}`;
}

function toDailyCron(timeValue, weekdays = []) {
  const parsed = parseTimeTo24h(timeValue) || { hh: 9, mm: 0 };
  const mapped = (Array.isArray(weekdays) ? weekdays : [])
    .map((d) => WEEKDAY_TO_CRON[String(d || '').toLowerCase().trim()])
    .filter((v) => v !== undefined);
  const uniq = [...new Set(mapped)];
  const dow = uniq.length === 0 || uniq.length === ALL_WEEKDAYS.length ? '*' : uniq.join(',');
  return `${parsed.mm} ${parsed.hh} * * ${dow}`;
}

function toIntervalCron(intervalHoursValue) {
  const raw = Number(intervalHoursValue);
  const every = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : DEFAULT_ELAPSED_INTERVAL_VALUE;
  return `0 */${every} * * *`;
}

function parseCronWeekdays(dowExpr) {
  const weekdays = String(dowExpr || '')
    .split(',')
    .map((v) => CRON_TO_WEEKDAY[String(v).trim()])
    .filter(Boolean);
  return weekdays.length ? [...new Set(weekdays)] : [...ALL_WEEKDAYS];
}

function parseDailyCron(cronExpr) {
  const parts = String(cronExpr || '').trim().split(/\s+/).filter(Boolean);
  if (parts.length < 5) return null;
  const mm = Number(parts[0]);
  const hh = Number(parts[1]);
  const dom = parts[2];
  const mon = parts[3];
  const dow = parts[4];
  if (!Number.isFinite(mm) || !Number.isFinite(hh) || dom !== '*' || mon !== '*') return null;
  return {
    dailyTime: toAmPm(hh, mm),
    weekdays: parseCronWeekdays(dow)
  };
}

function parseCalendarEveryCron(cronExpr) {
  const parts = String(cronExpr || '').trim().split(/\s+/).filter(Boolean);
  if (parts.length < 5) return null;
  const [min, hour, dom, mon, dow] = parts;
  if (dom !== '*' || mon !== '*') return null;
  const mm = Number(min);
  if (!Number.isFinite(mm) || mm < 0 || mm > 59) return null;
  if (hour === '*') {
    return {
      everyHours: 1,
      calendarTime: toAmPm(0, mm),
      weekdays: parseCronWeekdays(dow)
    };
  }
  let match = hour.match(/^\*\/(\d{1,2})$/);
  if (match) {
    const everyHours = Number(match[1]);
    if (Number.isFinite(everyHours) && everyHours > 0) {
      return {
        everyHours,
        calendarTime: toAmPm(0, mm),
        weekdays: parseCronWeekdays(dow)
      };
    }
  }
  match = hour.match(/^(\d{1,2})-23\/(\d{1,2})$/);
  if (match) {
    const startHH = Number(match[1]);
    const everyHours = Number(match[2]);
    if (Number.isFinite(startHH) && Number.isFinite(everyHours) && startHH >= 0 && startHH <= 23 && everyHours > 0) {
      return {
        everyHours,
        calendarTime: toAmPm(startHH, mm),
        weekdays: parseCronWeekdays(dow)
      };
    }
  }
  return null;
}

function toCalendarEveryCron(timeValue, everyHoursValue, weekdays = []) {
  const parsed = parseTimeTo24h(timeValue) || { hh: 0, mm: 0 };
  const rawEvery = Number(everyHoursValue);
  const everyHours = Number.isFinite(rawEvery) && rawEvery > 0 ? Math.round(rawEvery) : DEFAULT_CALENDAR_INTERVAL_HOURS;
  const safeEveryHours = Math.min(Math.max(everyHours, 1), 23);
  const mapped = (Array.isArray(weekdays) ? weekdays : [])
    .map((d) => WEEKDAY_TO_CRON[String(d || '').toLowerCase().trim()])
    .filter((v) => v !== undefined);
  const uniq = [...new Set(mapped)];
  const dow = uniq.length === 0 || uniq.length === ALL_WEEKDAYS.length ? '*' : uniq.join(',');
  let hourExpr = '*';
  if (safeEveryHours > 1) {
    hourExpr = parsed.hh === 0 ? `*/${safeEveryHours}` : `${parsed.hh}-23/${safeEveryHours}`;
  }
  return `${parsed.mm} ${hourExpr} * * ${dow}`;
}

function normalizeWeekdays(value) {
  if (Array.isArray(value)) return value.map((v) => String(v || '').trim().toLowerCase()).filter(Boolean);
  if (typeof value === 'string') {
    return value.split(',').map((v) => String(v || '').trim().toLowerCase()).filter(Boolean);
  }
  if (value && typeof value === 'object') {
    return Object.entries(value)
      .filter(([, selected]) => !!selected)
      .map(([k]) => String(k || '').trim().toLowerCase())
      .filter(Boolean);
  }
  return [];
}

function normalizeWeekdaySelection(value) {
  const weekdays = normalizeWeekdays(value);
  return weekdays.length ? [...new Set(weekdays)] : [...ALL_WEEKDAYS];
}

function normalizeEditorKind(form = {}) {
  const direct = String(form.scheduleEditorKind || '').trim().toLowerCase();
  if (direct === 'calendar' || direct === 'elapsed' || direct === 'advanced') return direct;
  const legacyMode = String(form.scheduleMode || '').trim().toLowerCase();
  if (legacyMode === 'custom') return 'advanced';
  if (legacyMode === 'interval') return 'elapsed';
  if (legacyMode === 'daily') return 'calendar';
  const rawType = String(firstDefined(form, ['scheduleType', 'schedule_type']) || '').trim().toLowerCase();
  if (rawType === 'interval') return 'elapsed';
  return 'calendar';
}

function normalizeCalendarPattern(form = {}) {
  const direct = String(form.calendarPattern || '').trim().toLowerCase();
  if (direct === 'once' || direct === 'every') return direct;
  const legacyMode = String(form.scheduleMode || '').trim().toLowerCase();
  if (legacyMode === 'interval') return 'every';
  return 'once';
}

function normalizeElapsedUnit(form = {}) {
  const unit = String(form.elapsedIntervalUnit || '').trim().toLowerCase();
  if (unit === 'minutes' || unit === 'hours' || unit === 'days') return unit;
  return DEFAULT_ELAPSED_INTERVAL_UNIT;
}

function intervalUnitMultiplier(unit) {
  if (unit === 'minutes') return 60;
  if (unit === 'days') return 86400;
  return 3600;
}

function toElapsedSeconds(value, unit) {
  const raw = Number(value);
  const amount = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : DEFAULT_ELAPSED_INTERVAL_VALUE;
  return amount * intervalUnitMultiplier(unit);
}

function fromIntervalSeconds(value) {
  const seconds = Number(value);
  if (!(Number.isFinite(seconds) && seconds > 0)) {
    return { elapsedIntervalValue: DEFAULT_ELAPSED_INTERVAL_VALUE, elapsedIntervalUnit: DEFAULT_ELAPSED_INTERVAL_UNIT };
  }
  if (seconds % 86400 === 0) {
    return { elapsedIntervalValue: seconds / 86400, elapsedIntervalUnit: 'days' };
  }
  if (seconds % 3600 === 0) {
    return { elapsedIntervalValue: seconds / 3600, elapsedIntervalUnit: 'hours' };
  }
  if (seconds % 60 === 0) {
    return { elapsedIntervalValue: seconds / 60, elapsedIntervalUnit: 'minutes' };
  }
  return { elapsedIntervalValue: Math.max(1, Math.round(seconds / 60)), elapsedIntervalUnit: 'minutes' };
}

function toElapsedPseudoCron(value, unit) {
  const raw = Number(value);
  const amount = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : DEFAULT_ELAPSED_INTERVAL_VALUE;
  if (unit === 'minutes') return `*/${amount} * * * *`;
  if (unit === 'days') return `0 0 */${amount} * *`;
  return toIntervalCron(amount);
}

function sortWeekdays(value) {
  const weekdays = normalizeWeekdaySelection(value);
  return weekdays.slice().sort((a, b) => WEEKDAY_ORDER.indexOf(a) - WEEKDAY_ORDER.indexOf(b));
}

function formatWeekdaySummary(value) {
  const weekdays = sortWeekdays(value);
  const key = weekdays.join(',');
  if (weekdays.length === ALL_WEEKDAYS.length) return 'every day';
  if (key === 'mon,tue,wed,thu,fri') return 'weekdays';
  return weekdays.map((day) => day.charAt(0).toUpperCase() + day.slice(1, 3)).join(', ');
}

function formatPreciseWeekdaySummary(value) {
  const weekdays = sortWeekdays(value);
  const key = weekdays.join(',');
  if (weekdays.length === ALL_WEEKDAYS.length) return 'every day';
  if (key === 'mon,tue,wed,thu,fri') return 'Mon-Fri';

  const labels = {
    mon: 'Mon',
    tue: 'Tue',
    wed: 'Wed',
    thu: 'Thu',
    fri: 'Fri',
    sat: 'Sat',
    sun: 'Sun'
  };

  const ranges = [];
  let start = weekdays[0];
  let prevIndex = WEEKDAY_ORDER.indexOf(start);
  for (let i = 1; i < weekdays.length; i += 1) {
    const current = weekdays[i];
    const currentIndex = WEEKDAY_ORDER.indexOf(current);
    if (currentIndex !== prevIndex + 1) {
      ranges.push(start === weekdays[i - 1] ? labels[start] : `${labels[start]}-${labels[weekdays[i - 1]]}`);
      start = current;
    }
    prevIndex = currentIndex;
  }
  if (weekdays.length) {
    const last = weekdays[weekdays.length - 1];
    ranges.push(start === last ? labels[start] : `${labels[start]}-${labels[last]}`);
  }
  return ranges.join(', ');
}

function formatTime24hLabel(hh, mm) {
  return `${String(hh).padStart(2, '0')}:${String(mm).padStart(2, '0')}`;
}

function formatOrdinal(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return String(value);
  const abs = Math.abs(Math.trunc(n));
  const mod100 = abs % 100;
  if (mod100 >= 11 && mod100 <= 13) return `${abs}th`;
  switch (abs % 10) {
    case 1: return `${abs}st`;
    case 2: return `${abs}nd`;
    case 3: return `${abs}rd`;
    default: return `${abs}th`;
  }
}

function buildCalendarEverySummary(calendarTime, everyHours, weekdays, timezone) {
  const parsed = parseTimeTo24h(calendarTime);
  const safeEveryHours = Number.isFinite(Number(everyHours)) && Number(everyHours) > 0 ? Math.min(Math.round(Number(everyHours)), 23) : DEFAULT_CALENDAR_INTERVAL_HOURS;
  const daySummary = formatPreciseWeekdaySummary(weekdays);
  if (!parsed) {
    return `Every ${safeEveryHours} hour${safeEveryHours === 1 ? '' : 's'} on ${daySummary} (${timezone})`;
  }
  const minuteLabel = String(parsed.mm).padStart(2, '0');
  if (safeEveryHours === 1) {
    return `At ${minuteLabel} minutes past every hour on ${daySummary} (${timezone})`;
  }
  const endHour = parsed.hh + Math.floor((23 - parsed.hh) / safeEveryHours) * safeEveryHours;
  return `At ${minuteLabel} minutes past every ${formatOrdinal(safeEveryHours)} hour from ${formatTime24hLabel(parsed.hh, parsed.mm)} through ${formatTime24hLabel(endHour, parsed.mm)} on ${daySummary} (${timezone})`;
}

function buildScheduleSummary(form = {}, derived = {}) {
  const kind = normalizeEditorKind(form);
  const timezone = String(form.timezone || DEFAULT_TIMEZONE).trim() || DEFAULT_TIMEZONE;
  if (kind === 'advanced') {
    const cron = String(derived.cronExpr || form.cronExpr || '').trim();
    const daily = parseDailyCron(cron);
    if (daily) {
      return `${daily.dailyTime} on ${formatWeekdaySummary(daily.weekdays)} (${timezone})`;
    }
    const calendar = parseCalendarEveryCron(cron);
    if (calendar) {
      return buildCalendarEverySummary(calendar.calendarTime, calendar.everyHours, calendar.weekdays, timezone);
    }
    return cron ? `Custom cron in ${timezone}` : 'Custom cron schedule';
  }
  if (kind === 'elapsed') {
    const unit = normalizeElapsedUnit(form);
    const raw = Number(form.elapsedIntervalValue);
    const value = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : DEFAULT_ELAPSED_INTERVAL_VALUE;
    return `Every ${value} ${unit} from previous run`;
  }
  const pattern = normalizeCalendarPattern(form);
  const weekdays = normalizeWeekdaySelection(form.weekdays);
  const calendarTime = String(form.calendarTime || form.dailyTime || DEFAULT_CALENDAR_TIME).trim() || DEFAULT_CALENDAR_TIME;
  if (pattern === 'every') {
    const raw = Number(form.calendarIntervalHours);
    const everyHours = Number.isFinite(raw) && raw > 0 ? Math.min(Math.round(raw), 23) : DEFAULT_CALENDAR_INTERVAL_HOURS;
    return buildCalendarEverySummary(calendarTime, everyHours, weekdays, timezone);
  }
  return `${calendarTime} on ${formatWeekdaySummary(weekdays)} (${timezone})`;
}

function deriveScheduleFields(form = {}) {
  const kind = normalizeEditorKind(form);
  if (kind === 'advanced') {
    const cronExpr = String(form.cronExpr || '').trim();
    return {
      scheduleType: 'cron',
      cronExpr,
      intervalSeconds: null,
      scheduleSummary: buildScheduleSummary(form, { cronExpr })
    };
  }
  if (kind === 'elapsed') {
    const unit = normalizeElapsedUnit(form);
    const raw = Number(form.elapsedIntervalValue);
    const elapsedIntervalValue = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : DEFAULT_ELAPSED_INTERVAL_VALUE;
    const intervalSeconds = toElapsedSeconds(elapsedIntervalValue, unit);
    const cronExpr = toElapsedPseudoCron(elapsedIntervalValue, unit);
    return {
      scheduleType: 'interval',
      cronExpr,
      intervalSeconds,
      scheduleSummary: buildScheduleSummary({ ...form, elapsedIntervalValue, elapsedIntervalUnit: unit }, { cronExpr, intervalSeconds })
    };
  }
  const pattern = normalizeCalendarPattern(form);
  const calendarTime = String(form.calendarTime || form.dailyTime || DEFAULT_CALENDAR_TIME).trim() || DEFAULT_CALENDAR_TIME;
  const weekdays = normalizeWeekdaySelection(form.weekdays);
  if (pattern === 'every') {
    const raw = Number(form.calendarIntervalHours || form.intervalHours);
    const calendarIntervalHours = Number.isFinite(raw) && raw > 0 ? Math.min(Math.round(raw), 23) : DEFAULT_CALENDAR_INTERVAL_HOURS;
    const cronExpr = toCalendarEveryCron(calendarTime, calendarIntervalHours, weekdays);
    return {
      scheduleType: 'cron',
      cronExpr,
      intervalSeconds: null,
      scheduleSummary: buildScheduleSummary({ ...form, calendarTime, calendarIntervalHours, weekdays }, { cronExpr })
    };
  }
  const cronExpr = toDailyCron(calendarTime, weekdays);
  return {
    scheduleType: 'cron',
    cronExpr,
    intervalSeconds: null,
    scheduleSummary: buildScheduleSummary({ ...form, calendarTime, weekdays }, { cronExpr })
  };
}

function inferEditorState(form = {}) {
  const rawType = String(firstDefined(form, ['scheduleType', 'schedule_type']) || '').trim().toLowerCase();
  const rawCron = String(firstDefined(form, ['cronExpr', 'cron_expr']) || '').trim();
  const intervalSeconds = firstDefined(form, ['intervalSeconds', 'interval_seconds']);

  if (rawType === 'interval') {
    return {
      scheduleEditorKind: 'elapsed',
      calendarPattern: 'once',
      calendarTime: DEFAULT_CALENDAR_TIME,
      weekdays: [...ALL_WEEKDAYS],
      calendarIntervalHours: DEFAULT_CALENDAR_INTERVAL_HOURS,
      ...fromIntervalSeconds(intervalSeconds)
    };
  }

  const daily = parseDailyCron(rawCron);
  if (daily) {
    return {
      scheduleEditorKind: 'calendar',
      calendarPattern: 'once',
      calendarTime: daily.dailyTime,
      weekdays: daily.weekdays,
      calendarIntervalHours: DEFAULT_CALENDAR_INTERVAL_HOURS,
      ...fromIntervalSeconds(intervalSeconds)
    };
  }

  const calendarEvery = parseCalendarEveryCron(rawCron);
  if (calendarEvery) {
    return {
      scheduleEditorKind: 'calendar',
      calendarPattern: 'every',
      calendarTime: calendarEvery.calendarTime,
      weekdays: calendarEvery.weekdays,
      calendarIntervalHours: calendarEvery.everyHours,
      ...fromIntervalSeconds(intervalSeconds)
    };
  }

  if (rawType === 'cron' || rawCron) {
    return {
      scheduleEditorKind: 'advanced',
      calendarPattern: 'once',
      calendarTime: DEFAULT_CALENDAR_TIME,
      weekdays: [...ALL_WEEKDAYS],
      calendarIntervalHours: DEFAULT_CALENDAR_INTERVAL_HOURS,
      ...fromIntervalSeconds(intervalSeconds)
    };
  }

  return {
    scheduleEditorKind: 'calendar',
    calendarPattern: 'once',
    calendarTime: DEFAULT_CALENDAR_TIME,
    weekdays: [...ALL_WEEKDAYS],
    calendarIntervalHours: DEFAULT_CALENDAR_INTERVAL_HOURS,
    ...fromIntervalSeconds(intervalSeconds)
  };
}

function hasEditorState(form = {}) {
  return form.scheduleEditorKind !== undefined ||
    form.calendarPattern !== undefined ||
    form.calendarTime !== undefined ||
    form.calendarIntervalHours !== undefined ||
    form.elapsedIntervalValue !== undefined ||
    form.elapsedIntervalUnit !== undefined ||
    form.scheduleSummary !== undefined;
}

function applyLegacyAliases(next = {}) {
  const kind = normalizeEditorKind(next);
  const pattern = normalizeCalendarPattern(next);
  if (kind === 'advanced') {
    next.scheduleMode = 'custom';
  } else if (kind === 'elapsed') {
    next.scheduleMode = 'interval';
    next.intervalHours = Math.max(1, Math.round((Number(next.intervalSeconds) || 0) / 3600)) || DEFAULT_ELAPSED_INTERVAL_VALUE;
  } else if (pattern === 'every') {
    next.scheduleMode = 'interval';
    next.dailyTime = next.calendarTime;
    next.intervalHours = next.calendarIntervalHours;
  } else {
    next.scheduleMode = 'daily';
    next.dailyTime = next.calendarTime;
  }
}

function ensureEditorState(form = {}) {
  const next = { ...form };
  const inferred = hasEditorState(next) ? {} : inferEditorState(next);
  Object.assign(next, inferred);

  next.scheduleEditorKind = normalizeEditorKind(next);
  next.calendarPattern = normalizeCalendarPattern(next);
  next.calendarTime = String(next.calendarTime || next.dailyTime || DEFAULT_CALENDAR_TIME).trim() || DEFAULT_CALENDAR_TIME;
  next.weekdays = normalizeWeekdaySelection(next.weekdays);
  const rawCalendarEvery = Number(next.calendarIntervalHours || next.intervalHours);
  next.calendarIntervalHours = Number.isFinite(rawCalendarEvery) && rawCalendarEvery > 0
    ? Math.min(Math.round(rawCalendarEvery), 23)
    : DEFAULT_CALENDAR_INTERVAL_HOURS;
  const normalizedElapsed = fromIntervalSeconds(next.intervalSeconds);
  next.elapsedIntervalValue = Number.isFinite(Number(next.elapsedIntervalValue)) && Number(next.elapsedIntervalValue) > 0
    ? Math.round(Number(next.elapsedIntervalValue))
    : normalizedElapsed.elapsedIntervalValue;
  next.elapsedIntervalUnit = normalizeElapsedUnit(next);
  const timeoutCandidate = firstDefined(next, ['timeoutSeconds', 'timeout_seconds']);
  const timeoutNumber = Number(timeoutCandidate);
  next.timeoutSeconds =
    timeoutCandidate !== undefined &&
    timeoutCandidate !== null &&
    String(timeoutCandidate).trim() !== '' &&
    Number.isFinite(timeoutNumber) &&
    timeoutNumber >= 0
      ? Math.round(timeoutNumber)
      : DEFAULT_TIMEOUT_SECONDS;
  if (!String(next.visibility || '').trim()) {
    next.visibility = 'private';
  }
  if (!String(next.timezone || '').trim()) {
    next.timezone = DEFAULT_TIMEZONE;
  }

  const derived = deriveScheduleFields(next);
  next.scheduleType = derived.scheduleType;
  next.cronExpr = derived.cronExpr;
  next.intervalSeconds = derived.intervalSeconds;
  next.scheduleSummary = derived.scheduleSummary;

  applyLegacyAliases(next);
  return next;
}

function applyScheduleSync(form = {}) {
  return ensureEditorState(form);
}

function getSchedulesDataSource(context) {
  return context?.Context?.('schedules')?.handlers?.dataSource;
}

function ensureRunsBoundToScheduleForm(context, collection = []) {
  const incoming = Array.isArray(collection) ? collection : [];
  if (incoming.length > 0) return;
  const schedulesDS = getSchedulesDataSource(context);
  const formID = String(getFormState(schedulesDS)?.id || '').trim();
  if (!formID) return;
  const runsDS = context?.handlers?.dataSource;
  const currentInputID = String(runsDS?.peekInput?.()?.parameters?.scheduleId || '').trim();
  if (currentInputID === formID) return;
  schedulesDS?.pushFormDependencies?.();
}

function getFormState(ds) {
  const peek = ds?.peekFormData?.();
  const peekValues = (peek && typeof peek === 'object' && peek.values && typeof peek.values === 'object') ? peek.values : {};
  const get = ds?.getFormData?.() || {};
  const selected = ds?.peekSelection?.()?.selected || ds?.getSelection?.()?.selected || {};
  return { ...selected, ...get, ...peekValues };
}

function hasNonEmptyText(value) {
  return String(value || '').trim().length > 0;
}

function hasPositiveNumber(value) {
  const raw = Number(value);
  return Number.isFinite(raw) && raw > 0;
}

function getValidationErrors(form = {}) {
  return form?.validationErrors && typeof form.validationErrors === 'object' && !Array.isArray(form.validationErrors)
    ? form.validationErrors
    : {};
}

function clearLegacyFormError(next = {}) {
  if (!Object.prototype.hasOwnProperty.call(next, 'formError')) return next;
  return { ...next, formError: '' };
}

function validateScheduleForm(rawForm = {}) {
  const fieldErrors = {};
  const orderedFields = [];
  const kind = normalizeEditorKind(rawForm);
  const addError = (field, message) => {
    const key = String(field || '').trim();
    const text = String(message || '').trim();
    if (!key || !text || fieldErrors[key]) return;
    fieldErrors[key] = text;
    orderedFields.push(key);
  };

  if (!hasNonEmptyText(rawForm.name)) {
    addError('name', 'Schedule Name is required');
  }

  if (!hasNonEmptyText(rawForm.agentRef || rawForm.agent)) {
    addError('agentRef', 'Agent is required');
  }

  if (!hasNonEmptyText(rawForm.taskPrompt) && !hasNonEmptyText(rawForm.taskPromptUri)) {
    addError('taskPrompt', 'Task Prompt or Task Prompt URI is required');
  }

  if (kind === 'advanced' && !hasNonEmptyText(rawForm.cronExpr)) {
    addError('cronExpr', 'Cron Expression is required');
  }

  if (kind === 'calendar' && !hasNonEmptyText(rawForm.calendarTime || rawForm.dailyTime)) {
    addError('calendarTime', 'At / Starting At is required');
  }

  if (kind === 'calendar' && normalizeCalendarPattern(rawForm) === 'every' && !hasPositiveNumber(rawForm.calendarIntervalHours || rawForm.intervalHours)) {
    addError('calendarIntervalHours', 'Repeat Every (Hours Within The Day) is required');
  }

  if (kind === 'elapsed' && !hasPositiveNumber(firstDefined(rawForm, ['elapsedIntervalValue', 'intervalHours', 'intervalSeconds', 'interval_seconds']))) {
    addError('elapsedIntervalValue', 'Repeat Every is required');
  }

  if (kind === 'elapsed' && !hasNonEmptyText(rawForm.elapsedIntervalUnit) && !hasPositiveNumber(firstDefined(rawForm, ['intervalSeconds', 'interval_seconds']))) {
    addError('elapsedIntervalUnit', 'Interval Unit is required');
  }

  try {
    normalizeScheduleDateTime(rawForm.startAt, 'Start Date');
  } catch (err) {
    addError('startAt', err?.message || 'Start Date must be a valid date/time');
  }

  try {
    normalizeScheduleDateTime(rawForm.endAt, 'End Date');
  } catch (err) {
    addError('endAt', err?.message || 'End Date must be a valid date/time');
  }

  return {
    fieldErrors,
    orderedFields,
    messages: orderedFields.map((field) => fieldErrors[field]).filter(Boolean)
  };
}

function setScheduleValidation(ds, validation = {}) {
  const current = getFormState(ds);
  const next = clearLegacyFormError({ ...current, validationErrors: { ...(validation?.fieldErrors || {}) } });
  if (JSON.stringify(current) === JSON.stringify(next)) return false;
  ds?.setFormData?.({ values: next });
  return true;
}

function clearScheduleValidation(ds, { fieldKey = '', baseForm = null } = {}) {
  const current = baseForm && typeof baseForm === 'object' ? baseForm : getFormState(ds);
  const existing = getValidationErrors(current);
  const keys = Object.keys(existing);
  const clearKey = String(fieldKey || '').trim();
  if (keys.length === 0 && !hasNonEmptyText(current?.formError)) return false;

  let nextErrors = existing;
  if (clearKey) {
    const related = new Set([clearKey]);
    if (clearKey === 'taskPrompt' || clearKey === 'taskPromptUri') {
      related.add('taskPrompt');
      related.add('taskPromptUri');
    }
    if (clearKey === 'scheduleEditorKind') {
      MODE_VALIDATION_FIELDS.forEach((field) => related.add(field));
    }
    if (clearKey === 'calendarPattern') {
      related.add('calendarIntervalHours');
    }
    nextErrors = { ...existing };
    related.forEach((field) => {
      delete nextErrors[field];
    });
  } else {
    nextErrors = {};
  }

  const next = clearLegacyFormError({ ...current, validationErrors: nextErrors });
  if (JSON.stringify(current) === JSON.stringify(next)) return false;
  ds?.setFormData?.({ values: next });
  return true;
}

function summarizeValidationMessages(messages = []) {
  const cleaned = (Array.isArray(messages) ? messages : []).map((entry) => String(entry || '').trim()).filter(Boolean);
  if (cleaned.length === 0) return "Schedule can't be saved.";
  if (cleaned.length === 1) return `Schedule can't be saved. ${cleaned[0]}.`;
  return `Schedule can't be saved. ${cleaned[0]}. ${cleaned.length - 1} more issue${cleaned.length > 2 ? 's' : ''}.`;
}

function pushWindowTabSelection(windowId, tabId) {
  if (!windowId || !tabId) return false;
  try {
    const bus = getBusSignal(windowId);
    const current = bus?.peek?.() || bus?.value || [];
    bus.value = [...(Array.isArray(current) ? current : []), { type: 'selectTab', tabId }];
    return true;
  } catch (_) {
    return false;
  }
}

function focusScheduleField(fieldId) {
  if (typeof document === 'undefined' || !fieldId) return false;
  const wrapper = document.querySelector(`[data-forge-control-id="${String(fieldId).replace(/"/g, '\\"')}"]`);
  if (!wrapper) return false;
  const target = wrapper.querySelector('input:not([disabled]), textarea:not([disabled]), select:not([disabled]), button:not([disabled]), [tabindex]:not([tabindex="-1"])') || wrapper;
  try {
    wrapper.scrollIntoView?.({ block: 'center', behavior: 'smooth' });
  } catch (_) {}
  try {
    target.focus({ preventScroll: true });
    return true;
  } catch (_) {
    try {
      target.focus();
      return true;
    } catch (_) {
      return false;
    }
  }
}

function revealScheduleValidation(context, fieldId = '') {
  const windowId = context?.identity?.windowId || context?.Context?.('schedules')?.identity?.windowId;
  if (!windowId) return;
  pushWindowTabSelection(windowId, 'scheduleEditor');
  const subtabId = VALIDATION_FIELD_TAB[String(fieldId || '').trim()] || 'general';
  setTimeout(() => pushWindowTabSelection(windowId, subtabId), 20);
  setTimeout(() => focusScheduleField(fieldId), 100);
}

function resetScheduleEditorTab(context) {
  const windowId = context?.identity?.windowId || context?.Context?.('schedules')?.identity?.windowId;
  if (!windowId) return;
  setTimeout(() => pushWindowTabSelection(windowId, 'general'), 20);
}

function applyIncomingFieldValue(form = {}, item, value) {
  const fieldKey = item?.dataField || item?.bindingPath || item?.id;
  if (!fieldKey) return form;
  return {
    ...form,
    [fieldKey]: value
  };
}

function resolveIncomingFieldValue(params = {}) {
  if (Object.prototype.hasOwnProperty.call(params, 'value') && params.value !== undefined) {
    return { hasValue: true, value: params.value };
  }
  if (Object.prototype.hasOwnProperty.call(params, 'selected') && params.selected !== undefined) {
    const selected = params.selected;
    return {
      hasValue: true,
      value: selected && selected.value !== undefined ? selected.value : selected
    };
  }
  const event = params.event;
  if (event && typeof event === 'object') {
    if (event.target && typeof event.target === 'object') {
      if (Object.prototype.hasOwnProperty.call(event.target, 'checked')) {
        return { hasValue: true, value: event.target.checked };
      }
      if (Object.prototype.hasOwnProperty.call(event.target, 'value')) {
        return { hasValue: true, value: event.target.value };
      }
    }
    if (Object.prototype.hasOwnProperty.call(event, 'value')) {
      return { hasValue: true, value: event.value };
    }
  }
  if (event !== undefined) {
    return { hasValue: true, value: event };
  }
  return { hasValue: false, value: undefined };
}

function deriveCronExprForKind(form = {}, kind) {
  const source = ensureEditorState({ ...form, scheduleEditorKind: kind });
  return String(deriveScheduleFields(source).cronExpr || '').trim();
}

function seedAdvancedCronExpr(current = {}, base = {}, fieldKey = '') {
  const targetKind = normalizeEditorKind(current);
  if (fieldKey === 'scheduleEditorKind' && targetKind === 'advanced') {
    const sourceKind = normalizeEditorKind(base);
    if (sourceKind !== 'advanced') {
      return {
        ...current,
        cronExpr: deriveCronExprForKind(base, sourceKind)
      };
    }
    return current;
  }

  if (targetKind !== 'advanced') return current;

  if (ELAPSED_BUILDER_FIELDS.has(fieldKey)) {
    return {
      ...current,
      cronExpr: deriveCronExprForKind(current, 'elapsed')
    };
  }

  if (CALENDAR_BUILDER_FIELDS.has(fieldKey)) {
    return {
      ...current,
      cronExpr: deriveCronExprForKind(current, 'calendar')
    };
  }

  return current;
}

function updateFormIfChanged(ds, values) {
  const current = getFormState(ds);
  if (JSON.stringify(current) === JSON.stringify(values)) return false;
  ds?.setFormData?.({ values });
  return true;
}

function scheduleEmptyStateConfig(panelId) {
  if (panelId === 'bp6-tab-panel_window-manager-tabs_schedule/history') {
    return {
      title: 'No scheduled runs yet',
      body: 'Trigger a schedule to populate execution history and inspect outcomes here.'
    };
  }
  return {
    title: 'No schedules yet',
    body: 'Create an automation to run an agent on a cadence, then review recent runs from the Runs tab.'
  };
}

export function panelHasRenderableRows(wrapper) {
  const body = wrapper?.querySelector?.('tbody');
  if (!body) return false;
  const rows = Array.from(body.querySelectorAll('tr'));
  if (rows.length === 0) return false;
  return rows.some((row) => {
    const cells = Array.from(row.querySelectorAll?.('td') || []);
    if (cells.length === 0) return false;
    return cells.some((cell) => !cell.classList?.contains?.('empty-row'));
  });
}

function decorateScheduleEmptyStates() {
  if (typeof document === 'undefined') return;
  const panelIds = [
    'bp6-tab-panel_form-tabs_schedulesCatalogue',
    'bp6-tab-panel_window-manager-tabs_schedule/history'
  ];
  panelIds.forEach((panelId) => {
    const panel = document.getElementById(panelId);
    if (!panel) return;
    const wrapper = panel.querySelector('.basic-table-wrapper');
    if (!wrapper) return;
    let empty = wrapper.querySelector('.app-automation-empty-state');
    if (panelId === 'bp6-tab-panel_window-manager-tabs_schedule/history') {
      empty?.remove();
      wrapper.classList.remove('is-empty');
      return;
    }
    if (panelHasRenderableRows(wrapper)) {
      empty?.remove();
      wrapper.classList.remove('is-empty');
      return;
    }
    wrapper.classList.add('is-empty');
    if (!empty) {
      const cfg = scheduleEmptyStateConfig(panelId);
      empty = document.createElement('div');
      empty.className = 'app-automation-empty-state';
      empty.innerHTML = `<div class="app-automation-empty-title">${cfg.title}</div><div class="app-automation-empty-body">${cfg.body}</div>`;
      wrapper.appendChild(empty);
    }
  });
}

function installScheduleEmptyStateObserver() {
  if (typeof document === 'undefined' || scheduleService._emptyStateObserverInstalled) return;
  const observer = new MutationObserver(() => {
    clearTimeout(scheduleService._emptyStateObserverTimer);
    scheduleService._emptyStateObserverTimer = setTimeout(decorateScheduleEmptyStates, 20);
  });
  observer.observe(document.body, { childList: true, subtree: true });
  scheduleService._emptyStateObserverInstalled = true;
}

function ensureScheduleRuntimeHooks() {
  try {
    if (!scheduleService._visibilityHookInstalled) {
      registerDynamicEvaluator('onVisible', ({ item, context: ctx }) => {
        try {
          const ds = ctx?.Context?.('schedules')?.handlers?.dataSource;
          const form = ensureEditorState(getFormState(ds));
          const kind = normalizeEditorKind(form);
          const calendarPattern = normalizeCalendarPattern(form);
          if (item?.id === 'calendarPattern') return kind === 'calendar';
          if (item?.id === 'calendarTime') return kind === 'calendar';
          if (item?.id === 'calendarIntervalHours') return kind === 'calendar' && calendarPattern === 'every';
          if (item?.id === 'weekdays') return kind === 'calendar';
          if (item?.id === 'elapsedIntervalValue' || item?.id === 'elapsedIntervalUnit') return kind === 'elapsed';
          if (item?.id === 'cronExpr') return kind === 'advanced';
          if (item?.id === 'scheduleType' || item?.id === 'intervalSeconds') return false;
        } catch (_) {}
        return undefined;
      });
      scheduleService._visibilityHookInstalled = true;
    }
    if (!scheduleService._validationHookInstalled) {
      registerDynamicEvaluator('onValidate', ({ item, context: ctx }) => {
        try {
          const ds = getSchedulesDataSource(ctx) || ctx?.handlers?.dataSource;
          const form = getFormState(ds);
          return getValidationErrors(form)?.[item?.id] || undefined;
        } catch (_) {
          return undefined;
        }
      });
      scheduleService._validationHookInstalled = true;
    }
  } catch (e) {
    log.warn('scheduleService.ensureScheduleRuntimeHooks error', e);
  }
}

async function saveSchedule({ context }) {
  ensureScheduleRuntimeHooks();
  if (scheduleService._saveScheduleInFlight) return true;
  const ctx = context?.Context('schedules');
  if (!ctx) {
    log.error('scheduleService.saveSchedule: schedules context not found');
    return false;
  }
  const ds = ctx.handlers?.dataSource;
  const rawForm = getFormState(ds);
  const form = ensureEditorState(rawForm);
  if (!Object.keys(form).length) {
    log.warn('scheduleService.saveSchedule: no form data');
    return false;
  }
  const validation = validateScheduleForm(rawForm);
  if (validation.orderedFields.length > 0) {
    setScheduleValidation(ds, validation);
    showToast(summarizeValidationMessages(validation.messages), {
      intent: 'danger',
      key: `schedule-validation:${validation.orderedFields.join(',')}`,
      ttlMs: SCHEDULE_TOAST_TTL_MS
    });
    revealScheduleValidation(context, validation.orderedFields[0]);
    return false;
  }
  const enabledRaw = firstDefined(form, ['enabled']);
  const existingID = String(form.id || '').trim();
  const scheduleID = existingID || newScheduleID();
  const visibility = normalizeVisibility(form.visibility);
  scheduleService._saveScheduleInFlight = true;
  try {
    const payload = {
      id: scheduleID,
      name: form.name,
      description: form.description,
      agentRef: form.agentRef || form.agent,
      modelOverride: form.modelOverride,
      enabled: enabledRaw === undefined ? false : asBoolean(enabledRaw),
      startAt: normalizeScheduleDateTime(form.startAt, 'Start Date'),
      endAt: normalizeScheduleDateTime(form.endAt, 'End Date'),
      scheduleType: form.scheduleType,
      cronExpr: String(form.cronExpr || '').trim(),
      intervalSeconds: form.scheduleType === 'interval' ? form.intervalSeconds : null,
      timezone: form.timezone,
      timeoutSeconds: form.timeoutSeconds,
      taskPromptUri: form.taskPromptUri,
      taskPrompt: form.taskPrompt,
      userCredUrl: form.userCredUrl
    };
    if (visibility) {
      payload.visibility = visibility;
    }
    if (!existingID) {
      updateFormIfChanged(ds, { ...form, id: scheduleID });
    }
    ds?.setLoading?.(true);
    await client.upsertSchedules([payload]);
    clearScheduleValidation(ds);
    let synced = null;
    try {
      synced = syncSavedSchedule(ds, await client.getSchedule(scheduleID));
    } catch (refreshErr) {
      log.warn('scheduleService.saveSchedule refresh error', refreshErr);
    }
    if (!existingID || !synced) {
      ds?.fetchCollection?.();
    }
    resetScheduleEditorTab(context);
    return true;
  } catch (err) {
    log.error('scheduleService.saveSchedule error', err);
    clearScheduleValidation(ds);
    showToast(String(err?.message || err || 'Failed to save schedule'), {
      intent: 'danger',
      key: 'schedule-save-error',
      ttlMs: SCHEDULE_TOAST_TTL_MS
    });
    return false;
  } finally {
    scheduleService._saveScheduleInFlight = false;
    ds?.setLoading?.(false);
  }
}

async function deleteSchedule({ context }) {
  const ctx = context?.Context('schedules');
  if (!ctx) {
    log.error('scheduleService.deleteSchedule: schedules context not found');
    return false;
  }
  const ds = ctx.handlers?.dataSource;
  const selected = ds?.peekSelection?.()?.selected || ds?.getSelection?.()?.selected || {};
  const id = selected?.id;
  if (!id) {
    log.warn('scheduleService.deleteSchedule: no schedule selected');
    return false;
  }
  // Soft-delete: disable the schedule via PATCH (no DELETE endpoint available)
  ds?.setLoading?.(true);
  try {
    await client.upsertSchedules([{ id, name: selected.name, enabled: false }]);
    ds?.fetchCollection?.();
    return true;
  } catch (err) {
    log.error('scheduleService.deleteSchedule error', err);
    ds?.setError?.(err);
    return false;
  } finally {
    ds?.setLoading?.(false);
  }
}

/* ─── display formatters ────────────────────────────────────────── */

function fmtRelativeDate(raw) {
  if (!raw) return '—';
  const d = new Date(raw);
  if (isNaN(d.getTime())) return String(raw);
  const now = new Date();
  const diffMs = d.getTime() - now.getTime();
  const absDiff = Math.abs(diffMs);
  const mins = Math.round(absDiff / 60000);
  const hrs = Math.round(absDiff / 3600000);
  const days = Math.round(absDiff / 86400000);
  const isFuture = diffMs > 0;

  if (mins < 1) return 'just now';
  if (mins < 60) return isFuture ? `in ${mins}m` : `${mins}m ago`;
  if (hrs < 24) return isFuture ? `in ${hrs}h` : `${hrs}h ago`;
  if (days < 7) return isFuture ? `in ${days}d` : `${days}d ago`;

  const month = d.toLocaleString('en-US', { month: 'short' });
  const day = d.getDate();
  const hour = d.toLocaleString('en-US', { hour: 'numeric', minute: '2-digit', hour12: true });
  return `${month} ${day}, ${hour}`;
}

function fmtStatus(raw) {
  if (!raw) return '—';
  const s = String(raw).trim().toLowerCase();
  const labels = {
    success: 'Success', completed: 'Success', ok: 'Success',
    failed: 'Failed', error: 'Failed', failure: 'Failed',
    running: 'Running', in_progress: 'Running', pending: 'Pending',
    queued: 'Queued', cancelled: 'Cancelled', canceled: 'Cancelled',
    skipped: 'Skipped', timeout: 'Timed Out'
  };
  return labels[s] || String(raw).charAt(0).toUpperCase() + String(raw).slice(1);
}

function fmtScheduleType(raw) {
  if (!raw) return '—';
  const s = String(raw).trim().toLowerCase();
  if (s === 'cron') return 'Cron';
  if (s === 'interval') return 'Interval';
  if (s === 'adhoc') return 'Adhoc';
  return String(raw).charAt(0).toUpperCase() + String(raw).slice(1);
}

function fmtCronSummary(row = {}) {
  const normalized = ensureEditorState(row);
  return normalized.scheduleSummary || String(normalized.cronExpr || '').trim() || '—';
}

function fmtDuration(row) {
  const start = row?.startedAt || row?.started_at || row?.createdAt || row?.created_at;
  const end = row?.completedAt || row?.completed_at;
  if (!start || !end) return '—';
  const diffMs = new Date(end).getTime() - new Date(start).getTime();
  if (isNaN(diffMs) || diffMs < 0) return '—';
  const secs = Math.round(diffMs / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  const remSecs = secs % 60;
  if (mins < 60) return remSecs > 0 ? `${mins}m ${remSecs}s` : `${mins}m`;
  const hrs = Math.floor(mins / 60);
  const remMins = mins % 60;
  return remMins > 0 ? `${hrs}h ${remMins}m` : `${hrs}h`;
}

export const scheduleService = {
  onInit({ context }) {
    installScheduleEmptyStateObserver();
    setTimeout(decorateScheduleEmptyStates, 0);
    ensureScheduleRuntimeHooks();
    try {
      const ds = getSchedulesDataSource(context);
      if (ds) {
        const synced = applyScheduleSync(getFormState(ds));
        updateFormIfChanged(ds, synced);
        const selectedID = String(ds?.peekSelection?.()?.selected?.id || '').trim();
        if (!selectedID && String(synced?.id || '').trim()) {
          ds?.pushFormDependencies?.();
        }
      }
    } catch (_) {}
  },
  applyLookupFilter({ context }) {
    try {
      const ds = context?.handlers?.dataSource;
      const args = ds?.peekInput?.()?.args || {};
      const q = args.name ?? args.agentRef ?? args.query;
      setTimeout(() => {
        try {
          if (q === undefined || q === null) return;
          const current = ds?.peekFilter?.() || {};
          if (current.name === q) return;
          ds?.setSilentFilterValues?.({ filter: { ...current, name: q } });
          ds?.fetchCollection?.();
        } catch (e) {
          console.warn('[schedule.applyLookupFilter][deferred] error', e);
        }
      }, 0);
    } catch (e) {
      console.warn('[schedule.applyLookupFilter] error', e);
    }
  },
  saveSchedule,
  deleteSchedule,
  async runSelected({ context }) {
    if (scheduleService._runSelectedInFlight) return true;
    const ctx = context?.Context('schedules');
    if (!ctx) {
      log.error('scheduleService.runSelected: schedules context not found');
      return false;
    }
    const id = ctx.handlers?.dataSource?.peekSelection?.()?.selected?.id;
    if (!id) {
      ctx.handlers?.setError?.('Select a schedule first');
      return false;
    }
    ctx.handlers?.setLoading?.(true);
    scheduleService._runSelectedInFlight = true;
    try {
      await client.runScheduleNow(id);
      try { context?.Context('runs')?.connector?.get?.({}); } catch (_) {}
      return true;
    } catch (err) {
      log.error('scheduleService.runSelected error', err);
      ctx.handlers?.setError?.(err);
      return false;
    } finally {
      scheduleService._runSelectedInFlight = false;
      ctx.handlers?.setLoading?.(false);
    }
  },
  onFetchSchedules({ collection = [] }) {
    try {
      setTimeout(decorateScheduleEmptyStates, 0);
      const incoming = Array.isArray(collection) ? collection : [];
      return incoming.map((r) => normalizeScheduleRow(r));
    } catch (e) {
      log.error('schedule.onFetchSchedules error', e);
      return collection;
    }
  },
  onFetchRuns({ context, collection = [] }) {
    try {
      setTimeout(decorateScheduleEmptyStates, 0);
      const incoming = Array.isArray(collection) ? collection : [];
      ensureRunsBoundToScheduleForm(context, incoming);
      return incoming.map((r) => {
        const id = firstDefined(r, ['ScheduleRunId', 'scheduleRunId', 'schedule_run_id', 'Id', 'id']);
        const conversationId = firstDefined(r, ['ConversationId', 'conversationId', 'conversation_id', 'Id', 'id']);
        const legacyActivity = firstDefined(r, ['LastActivity', 'UpdatedAt', 'updatedAt', 'updated_at']);
        return {
          id,
          scheduleId: firstDefined(r, ['ScheduleId', 'scheduleId', 'schedule_id']),
          scheduleName: firstDefined(r, ['ScheduleName', 'scheduleName', 'schedule_name']),
          conversationId,
          status: firstDefined(r, ['Status', 'status']),
          createdAt: firstDefined(r, ['CreatedAt', 'createdAt', 'created_at']),
          startedAt: firstDefined(r, ['StartedAt', 'startedAt', 'started_at']) || (id === conversationId ? legacyActivity : null),
          completedAt: firstDefined(r, ['CompletedAt', 'completedAt', 'completed_at']) || (id === conversationId ? legacyActivity : null),
          errorMessage: firstDefined(r, ['ErrorMessage', 'errorMessage', 'error_message', 'StatusMessage', 'statusMessage', 'status_message'])
        };
      });
    } catch (e) {
      log.error('schedule.onFetchRuns error', e);
      return collection;
    }
  },
  onFetchAgentsLov({ context, collection = [] }) {
    try {
      const ds = context?.handlers?.dataSource;
      const filter = ds?.peekFilter?.() || {};
      const query = String(filter.name || filter.query || '').trim();
      const normalized = normalizeWorkspaceAgentInfos(collection);
      return filterLookupCollection(normalized, query, ['id', 'name', 'modelRef']);
    } catch (e) {
      log.error('schedule.onFetchAgentsLov error', e);
      return collection;
    }
  },
  onFetchModelsLov({ context, collection = [] }) {
    try {
      const ds = context?.handlers?.dataSource;
      const filter = ds?.peekFilter?.() || {};
      const query = String(filter.name || filter.query || '').trim();
      const normalized = normalizeWorkspaceModelInfos(collection);
      return filterLookupCollection(normalized, query, ['id', 'name']);
    } catch (e) {
      log.error('schedule.onFetchModelsLov error', e);
      return collection;
    }
  },
  showIfCalendar({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ensureEditorState(getFormState(ds));
      return normalizeEditorKind(form) === 'calendar';
    } catch (_) {
      return undefined;
    }
  },
  showIfCalendarEvery({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ensureEditorState(getFormState(ds));
      return normalizeEditorKind(form) === 'calendar' && normalizeCalendarPattern(form) === 'every';
    } catch (_) {
      return undefined;
    }
  },
  showIfElapsed({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ensureEditorState(getFormState(ds));
      return normalizeEditorKind(form) === 'elapsed';
    } catch (_) {
      return undefined;
    }
  },
  showIfAdvanced({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ensureEditorState(getFormState(ds));
      return normalizeEditorKind(form) === 'advanced';
    } catch (_) {
      return undefined;
    }
  },
  showIfEdit({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ds?.getFormData?.() || {};
      return String(form?.id || '').trim().length > 0;
    } catch (_) {
      return undefined;
    }
  },
  syncScheduleFields(params = {}) {
    try {
      ensureScheduleRuntimeHooks();
      const { context, item } = params;
      const ds = getSchedulesDataSource(context) || context?.handlers?.dataSource;
      if (!ds) return;
      const { hasValue, value } = resolveIncomingFieldValue(params);
      const base = getFormState(ds);
      const fieldKey = item?.dataField || item?.bindingPath || item?.id || '';
      let current = hasValue ? applyIncomingFieldValue(base, item, value) : clearLegacyFormError(base);
      if (fieldKey) {
        current = clearLegacyFormError({ ...current, validationErrors: { ...getValidationErrors(current) } });
        const existingErrors = getValidationErrors(current);
        if (Object.keys(existingErrors).length > 0) {
          const nextErrors = { ...existingErrors };
          const related = new Set([fieldKey]);
          if (fieldKey === 'taskPrompt' || fieldKey === 'taskPromptUri') {
            related.add('taskPrompt');
            related.add('taskPromptUri');
          }
          if (fieldKey === 'scheduleEditorKind') {
            MODE_VALIDATION_FIELDS.forEach((field) => related.add(field));
          }
          if (fieldKey === 'calendarPattern') {
            related.add('calendarIntervalHours');
          }
          related.forEach((field) => delete nextErrors[field]);
          current.validationErrors = nextErrors;
        }
      }
      current = seedAdvancedCronExpr(current, base, fieldKey);
      const synced = applyScheduleSync(current);
      updateFormIfChanged(ds, synced);
    } catch (e) {
      log.warn('schedule.syncScheduleFields error', e);
    }
  },
  addNewSchedule({ context }) {
    try {
      ensureScheduleRuntimeHooks();
      const ds = getSchedulesDataSource(context);
      if (ds) {
        ds.handleAddNew();
        updateFormIfChanged(ds, ensureEditorState({ ...getFormState(ds), validationErrors: {}, formError: '' }));
      }
      // Switch to Editor tab via bus message
      const windowId = context?.identity?.windowId;
      if (windowId) {
        pushWindowTabSelection(windowId, 'scheduleEditor');
        setTimeout(() => pushWindowTabSelection(windowId, 'general'), 20);
      }
    } catch (e) {
      log.warn('schedule.addNewSchedule error', e);
    }
  },
  hideField() {
    return false;
  },
  /* ── column value formatters ── */
  formatStatus({ col, row }) {
    try {
      const raw = row?.[col?.id];
      return fmtStatus(raw);
    } catch (_) { return undefined; }
  },
  formatDate({ col, row }) {
    try {
      const raw = row?.[col?.id];
      return fmtRelativeDate(raw);
    } catch (_) { return undefined; }
  },
  formatScheduleType({ col, row }) {
    try {
      const raw = row?.[col?.id];
      return fmtScheduleType(raw);
    } catch (_) { return undefined; }
  },
  formatCronSummary({ row }) {
    try {
      return fmtCronSummary(row);
    } catch (_) { return undefined; }
  },
  formatDuration({ row }) {
    try {
      return fmtDuration(row);
    } catch (_) { return undefined; }
  }
};
