import { endpoints } from '../endpoint';
import { getLogger } from 'forge/utils/logger';
import { registerDynamicEvaluator } from 'forge/runtime/binding';
const log = getLogger('agently');
const liveScheduleDraft = { dailyTime: '', intervalHours: '', cronExpr: '', lastMode: '' };

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
  sat: 6,
};

const CRON_TO_WEEKDAY = {
  0: 'sun',
  1: 'mon',
  2: 'tue',
  3: 'wed',
  4: 'thu',
  5: 'fri',
  6: 'sat',
  7: 'sun',
};

// Convert 0/1 or truthy/falsy to strict boolean
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

// Prefer the first defined field from provided keys
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
  // HH:MM
  let match = raw.match(/^(\d{1,2}):(\d{2})$/);
  if (match) {
    const hh = Number(match[1]);
    const mm = Number(match[2]);
    if (Number.isFinite(hh) && Number.isFinite(mm) && hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59) {
      return { hh, mm };
    }
  }
  // HH:MM AM/PM
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
  const days = Array.isArray(weekdays) ? weekdays : [];
  const mapped = days
      .map((d) => WEEKDAY_TO_CRON[String(d || '').toLowerCase().trim()])
      .filter((v) => v !== undefined);
  const uniq = [...new Set(mapped)];
  const dow = uniq.length ? uniq.join(',') : '*';
  return `${parsed.mm} ${parsed.hh} * * ${dow}`;
}

function toIntervalCron(intervalHoursValue) {
  const raw = Number(intervalHoursValue);
  const every = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : 24;
  return `0 */${every} * * *`;
}

function parseDailyCron(cronExpr) {
  const cron = String(cronExpr || '').trim();
  const parts = cron.split(/\s+/).filter(Boolean);
  if (parts.length < 5) return null;
  const mm = Number(parts[0]);
  const hh = Number(parts[1]);
  const dom = parts[2];
  const mon = parts[3];
  const dow = parts[4];
  if (!Number.isFinite(mm) || !Number.isFinite(hh) || dom !== '*' || mon !== '*') return null;
  const weekdays = String(dow || '')
      .split(',')
      .map((v) => CRON_TO_WEEKDAY[String(v).trim()])
      .filter(Boolean);
  return {
    dailyTime: toAmPm(hh, mm),
    weekdays: weekdays.length ? [...new Set(weekdays)] : ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'],
  };
}

function parseIntervalCron(cronExpr) {
  const cron = String(cronExpr || '').trim();
  const parts = cron.split(/\s+/).filter(Boolean);
  if (parts.length < 5) return null;
  const [min, hour, dom, mon, dow] = parts;
  if (dom !== '*' || mon !== '*' || dow !== '*') return null;
  // top of every N hours
  let m = min.match(/^0$/);
  let h = hour.match(/^\*\/(\d{1,2})$/);
  if (m && h) {
    const every = Number(h[1]);
    if (Number.isFinite(every) && every > 0) return { intervalHours: every };
  }
  // every hour on fixed minute
  m = min.match(/^(\d{1,2})$/);
  h = hour.match(/^\*$/);
  if (m && h) {
    // UI only supports hour granularity; map to 1 hour
    return { intervalHours: 1 };
  }
  return null;
}

// Save or update a schedule via datly PATCH API
export async function saveSchedule({ context }) {
  const ctx = context?.Context('schedules');
  if (!ctx) {
    log.error('scheduleService.saveSchedule: schedules context not found');
    return false;
  }
  const ds = ctx.handlers?.dataSource;
  const editedValues = (ds?.peekFormData?.()?.values) || {};
  const formData = ds?.getFormData?.() || {};
  const selected = ds?.peekSelection?.()?.selected || ds?.getSelection?.()?.selected || {};
  // Some data source implementations expose only changed fields in peekFormData().values.
  // Merge with selected/full row so save payload preserves untouched fields.
  const form = { ...selected, ...formData, ...editedValues };
  if (!form || Object.keys(form).length === 0) {
    log.warn('scheduleService.saveSchedule: no form data');
    return false;
  }
  // Normalize model: ensure boolean enabled even if backend key shape differs.
  const enabledRaw = firstDefined(form, ['enabled']);
  const enabled = enabledRaw === undefined ? false : asBoolean(enabledRaw);
  const mode = normalizeMode(form);
  const weekdays = normalizeWeekdays(form.weekdays);
  const intervalHoursRaw = Number(form.intervalHours);
  const intervalHours = Number.isFinite(intervalHoursRaw) && intervalHoursRaw > 0 ? intervalHoursRaw : 24;
  const intervalSeconds = Math.round(intervalHours * 3600);
  const computedScheduleType = mode === 'interval' ? 'interval' : 'cron';
  const computedDailyCronExpr = toDailyCron(form.dailyTime, weekdays);
  const computedIntervalCronExpr = toIntervalCron(intervalHours);
  const customCronExpr = String(form.cronExpr || '').trim();
  // UI uses lowerCamel; map to write API (TitleCase)
  const payload = {
    id: form.id || '',
    name: form.name,
    description: form.description,
    visibility: String(form.visibility || 'private').trim().toLowerCase(),
    agentRef: form.agentRef || form.agent,
    modelOverride: form.modelOverride,
    enabled: enabled,
    startAt: form.startAt,
    endAt: form.endAt,
    scheduleType: computedScheduleType,
    cronExpr: mode === 'custom' ? customCronExpr : (mode === 'interval' ? computedIntervalCronExpr : computedDailyCronExpr),
    intervalSeconds: computedScheduleType === 'interval' ? intervalSeconds : null,
    timezone: form.timezone,
    timeoutSeconds: form.timeoutSeconds,
    taskPromptUri: form.taskPromptUri,
    taskPrompt: form.taskPrompt,
    userCredUrl: form.userCredUrl,
  };
  // Datly write API expects { Schedules: [ {...} ] } at /v1/api/agently/schedule (PATCH)
  const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
  const url = joinURL(base, '/v1/api/agently/schedule');
  ds?.setLoading?.(true);
  try {
    const resp = await fetch(url, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ data: [ payload ] }),
      credentials: 'include',
    });
    if (!resp.ok) {
      const txt = await resp.text().catch(() => '');
      throw new Error(`PATCH ${url} failed: ${resp.status} ${txt}`);
    }
    // Refresh collection so the schedules grid reflects saved changes
    ds?.fetchCollection?.();
    return true;
  } catch (err) {
    log.error('scheduleService.saveSchedule error', err);
    ds?.setError?.(err);
    return false;
  } finally {
    ds?.setLoading?.(false);
  }
}

function getSchedulesDataSource(context) {
  return context?.Context?.('schedules')?.handlers?.dataSource;
}

function getFormState(ds) {
  const peek = ds?.peekFormData?.();
  const peekValues = (peek && typeof peek === 'object' && peek.values && typeof peek.values === 'object') ? peek.values : {};
  const get = ds?.getFormData?.() || {};
  const selected = ds?.peekSelection?.()?.selected || ds?.getSelection?.()?.selected || {};
  // Merge all known sources to avoid dropping fields when one API returns a partial snapshot.
  return { ...selected, ...get, ...peekValues };
}

function normalizeWeekdays(value) {
  if (Array.isArray(value)) return value;
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

function normalizeMode(form = {}) {
  const mode = String(form.scheduleMode || form.scheduleType || '').trim().toLowerCase();
  if (mode === 'cron' || mode === 'adhoc') return 'custom';
  if (mode === 'daily' || mode === 'interval' || mode === 'custom') return mode;
  return 'daily';
}

function applyScheduleSync(form = {}) {
  const scheduleTypeRaw = firstDefined(form, ['scheduleType', 'schedule_type']);
  const cronRaw = firstDefined(form, ['cronExpr', 'cron_expr']);
  const intervalSecondsRaw = firstDefined(form, ['intervalSeconds', 'interval_seconds']);
  const seed = {
    ...form,
    scheduleType: scheduleTypeRaw !== undefined ? scheduleTypeRaw : form.scheduleType,
    cronExpr: cronRaw !== undefined ? cronRaw : form.cronExpr,
    intervalSeconds: intervalSecondsRaw !== undefined ? intervalSecondsRaw : form.intervalSeconds,
  };

  const mode = normalizeMode(seed);
  const next = { ...seed, scheduleMode: mode };
  const cronExpr = String(next.cronExpr || '').trim();
  const dailyParsed = parseDailyCron(cronExpr);
  const intervalParsed = parseIntervalCron(cronExpr);
  const intervalFromSeconds = Number(next.intervalSeconds);
  const fallbackIntervalHours = Number.isFinite(intervalFromSeconds) && intervalFromSeconds > 0
      ? Math.max(1, Math.round(intervalFromSeconds / 3600))
      : 24;

  if (dailyParsed) {
    next.dailyTime = dailyParsed.dailyTime;
    next.weekdays = dailyParsed.weekdays;
  }
  if (intervalParsed?.intervalHours) {
    next.intervalHours = intervalParsed.intervalHours;
  } else if (!(Number(next.intervalHours) > 0)) {
    next.intervalHours = fallbackIntervalHours;
  }

  if (mode === 'daily') {
    const timeValue = String(next.dailyTime || '').trim() || '09:00 AM';
    const weekdays = normalizeWeekdays(next.weekdays).length
        ? normalizeWeekdays(next.weekdays)
        : ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
    const cron = toDailyCron(timeValue, weekdays);
    next.scheduleType = 'cron';
    next.dailyTime = timeValue;
    next.weekdays = weekdays;
    next.cronExpr = cron;
    next.intervalSeconds = null;
  } else if (mode === 'interval') {
    const intervalHours = Number(next.intervalHours) > 0 ? Math.round(Number(next.intervalHours)) : fallbackIntervalHours;
    next.scheduleType = 'interval';
    next.intervalHours = intervalHours;
    next.intervalSeconds = intervalHours * 3600;
    next.cronExpr = toIntervalCron(intervalHours);
  } else {
    next.scheduleType = 'cron';
    next.intervalSeconds = null;
  }

  return next;
}

function setupLiveDraftListeners() {
  if (typeof document === 'undefined') return;
  if (setupLiveDraftListeners._installed) return;
  const capture = (event) => {
    try {
      const el = event?.target;
      if (!el || typeof el.closest !== 'function') return;
      if (!el.closest('#bp6-tab-panel_form-tabs_schedule')) return;
      if (el.matches?.("input[placeholder='09:00 AM']")) {
        liveScheduleDraft.dailyTime = String(el.value || '').trim();
      } else if (el.matches?.("input[placeholder='0 9 * * 1-5']")) {
        liveScheduleDraft.cronExpr = String(el.value || '').trim();
      } else if (el.matches?.("input[type='number']")) {
        liveScheduleDraft.intervalHours = String(el.value || '').trim();
      }
    } catch (_) { /* ignore */ }
  };
  document.addEventListener('input', capture, true);
  document.addEventListener('change', capture, true);
  document.addEventListener('keyup', capture, true);
  setupLiveDraftListeners._installed = true;
}

function updateFormIfChanged(ds, values) {
  const current = getFormState(ds);
  if (JSON.stringify(current) === JSON.stringify(values)) return false;
  ds?.setFormData?.({ values });
  return true;
}

function applyLiveScheduleInputs(form = {}) {
  if (typeof document === 'undefined') return form;
  const panel = document.querySelector('#bp6-tab-panel_form-tabs_schedule');
  if (!panel) return form;
  const next = { ...form };
  const dailyInput = panel.querySelector("input[placeholder='09:00 AM']");
  if (dailyInput && typeof dailyInput.value === 'string' && dailyInput.value.trim()) {
    next.dailyTime = dailyInput.value.trim();
    liveScheduleDraft.dailyTime = next.dailyTime;
  }
  const cronInput = panel.querySelector("input[placeholder='0 9 * * 1-5']");
  if (cronInput && typeof cronInput.value === 'string' && cronInput.value.trim()) {
    next.cronExpr = cronInput.value.trim();
    liveScheduleDraft.cronExpr = next.cronExpr;
  }
  const intervalInput = panel.querySelector("input[type='number']");
  if (intervalInput && typeof intervalInput.value === 'string' && intervalInput.value.trim()) {
    const n = Number(intervalInput.value);
    if (Number.isFinite(n) && n > 0) {
      next.intervalHours = Math.round(n);
      liveScheduleDraft.intervalHours = String(next.intervalHours);
    }
  }
  if (!String(next.dailyTime || '').trim() && liveScheduleDraft.dailyTime) {
    next.dailyTime = liveScheduleDraft.dailyTime;
  }
  if (!String(next.cronExpr || '').trim() && liveScheduleDraft.cronExpr) {
    next.cronExpr = liveScheduleDraft.cronExpr;
  }
  if (!(Number(next.intervalHours) > 0) && liveScheduleDraft.intervalHours) {
    const n = Number(liveScheduleDraft.intervalHours);
    if (Number.isFinite(n) && n > 0) next.intervalHours = Math.round(n);
  }
  return next;
}

function syncOnModeTransition(ds, form = {}) {
  const withLive = applyLiveScheduleInputs({ ...getFormState(ds), ...form });
  const mode = normalizeMode(withLive);
  const prevMode = String(withLive._lastScheduleMode || liveScheduleDraft.lastMode || 'daily').toLowerCase();
  const next = {
    ...withLive,
    scheduleMode: mode,
    _lastScheduleMode: mode,
  };
  if (mode !== prevMode) {
    next.cronExpr = transitionCronExpr(withLive, mode, prevMode);
  }
  liveScheduleDraft.lastMode = mode;
  updateFormIfChanged(ds, next);
}

function transitionCronExpr(form = {}, mode, prevMode) {
  const currentMode = String(mode || '').toLowerCase();
  const previousMode = String(prevMode || '').toLowerCase();
  if (currentMode === 'daily' || (currentMode === 'custom' && previousMode === 'daily')) {
    const dailyTime = String(form.dailyTime || liveScheduleDraft.dailyTime || '09:00 AM').trim() || '09:00 AM';
    const weekdays = normalizeWeekdays(form.weekdays).length
        ? normalizeWeekdays(form.weekdays)
        : ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
    return toDailyCron(dailyTime, weekdays);
  }
  if (currentMode === 'interval' || (currentMode === 'custom' && previousMode === 'interval')) {
    const raw = Number(form.intervalHours || liveScheduleDraft.intervalHours);
    const intervalHours = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : 24;
    return toIntervalCron(intervalHours);
  }
  return String(form.cronExpr || liveScheduleDraft.cronExpr || '').trim();
}

export const scheduleService = {
  onInit({ context }) {
    setupLiveDraftListeners();
    try {
      if (scheduleService._visibilityHookInstalled) return;
      registerDynamicEvaluator('onVisible', ({ item, context: ctx }) => {
        try {
          const ds = ctx?.Context?.('schedules')?.handlers?.dataSource;
          const form = ds?.getFormData?.() || {};
          const t = normalizeMode(form);
          if (item?.id === 'dailyTime') return t === 'daily';
          if (item?.id === 'weekdays') return t === 'daily' || t === 'interval';
          if (item?.id === 'intervalHours') return t === 'interval';
          if (item?.id === 'cronExpr') return t === 'custom';
          if (item?.id === 'scheduleType' || item?.id === 'intervalSeconds') return false;
        } catch (_) { /* ignore */ }
        return undefined;
      });
      scheduleService._visibilityHookInstalled = true;
    } catch (e) {
      log.warn('scheduleService.onInit visibility hook error', e);
    }
    // Normalize selected/new form once on window init so mode/cron stay aligned.
    try {
      const ds = getSchedulesDataSource(context);
      if (ds) {
        const synced = applyScheduleSync(applyLiveScheduleInputs(getFormState(ds)));
        synced._lastScheduleMode = normalizeMode(synced);
        updateFormIfChanged(ds, synced);
      }
    } catch (_) { /* ignore */ }
  },
  // Apply lookup filter for dialog (agentLov) using resolved args
  applyLookupFilter({ context }) {
    try {
      const ds = context?.handlers?.dataSource;
      const args = ds?.peekInput?.()?.args || {};
      const q = args.name ?? args.agentRef ?? args.query;
      // Defer to next tick to avoid signal write during dialog open cycle
      setTimeout(() => {
        try {
          if (q === undefined || q === null) return;
          const current = ds?.peekFilter?.() || {};
          if (current.name === q) return; // no change
          ds?.setSilentFilterValues?.({ filter: { ...current, name: q } });
          ds?.fetchCollection?.();
        } catch (e) {
          try { console.warn('[schedule.applyLookupFilter][deferred] error', e); } catch (_) {}
        }
      }, 0);
    } catch (e) {
      try { console.warn('[schedule.applyLookupFilter] error', e); } catch (_) {}
    }
  },
  saveSchedule,
  // Trigger a run for the currently selected schedule
  async runSelected({ context }) {
    if (scheduleService._runSelectedInFlight) {
      return true;
    }
    const ctx = context?.Context('schedules');
    if (!ctx) {
      log.error('scheduleService.runSelected: schedules context not found');
      return false;
    }
    const sel = ctx.handlers?.dataSource?.peekSelection?.();
    const id = sel?.selected?.id;
    if (!id) {
      ctx.handlers?.setError?.('Select a schedule first');
      return false;
    }
    const base = (endpoints?.agentlyAPI?.baseURL || '').replace(/\/+$/, '');
    const url = joinURL(base, `/v1/api/agently/scheduler/run-now/`);
    ctx.handlers?.setLoading?.(true);
    scheduleService._runSelectedInFlight = true;
    try {
      const resp = await fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ scheduleId: id, status: 'pending' }), credentials: 'include' });
      if (!resp.ok) {
        const txt = await resp.text().catch(() => '');
        throw new Error(`POST run-now failed: ${resp.status} ${txt}`);
      }
      // Optionally refresh runs datasource if present
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
  // Forge onFetch: transform raw records and return them for the datasource
  onFetchSchedules({ context, collection = [] }) {
    try {
      const incoming = Array.isArray(collection) ? collection : [];
      log.debug('schedule.onFetchSchedules: incoming size', incoming.length);
      const mapped = incoming.map((r) => ({
        _rawScheduleType: firstDefined(r, ['scheduleType', 'schedule_type']),
        _rawCronExpr: firstDefined(r, ['cronExpr', 'cron_expr']),
        _rawIntervalSeconds: firstDefined(r, ['intervalSeconds', 'interval_seconds']),
        id: firstDefined(r, ['id']),
        name: firstDefined(r, ['name']),
        description: firstDefined(r, ['description']),
        visibility: (firstDefined(r, ['visibility']) || 'private'),
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
        userCredUrl:  firstDefined(r, ['userCredUrl', 'user_cred_url']),
      }));
      mapped.forEach((row) => {
        const rawType = String(row._rawScheduleType || row.scheduleType || '').toLowerCase();
        const rawCron = row._rawCronExpr || row.cronExpr || '';
        const dailyParsed = parseDailyCron(rawCron);
        const intervalParsed = parseIntervalCron(rawCron);
        row.intervalHours = row._rawIntervalSeconds ? Math.max(1, Math.round(Number(row._rawIntervalSeconds) / 3600)) : (intervalParsed?.intervalHours || 24);
        row.dailyTime = dailyParsed?.dailyTime || '09:00 AM';
        row.weekdays = dailyParsed?.weekdays || ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
        if (rawType === 'interval') {
          row.scheduleMode = 'interval';
        } else if (dailyParsed) {
          row.scheduleMode = 'daily';
        } else if (rawType === 'cron') {
          row.scheduleMode = 'custom';
        } else if (intervalParsed) {
          row.scheduleMode = 'interval';
        } else if (String(rawCron || '').trim()) {
          row.scheduleMode = 'custom';
        } else {
          row.scheduleMode = 'daily';
        }
        row._lastScheduleMode = row.scheduleMode;
        // Keep cron text aligned with mode for editor round-trips.
        if (!String(row.cronExpr || '').trim()) {
          if (row.scheduleMode === 'daily') {
            row.cronExpr = toDailyCron(row.dailyTime, row.weekdays);
          } else if (row.scheduleMode === 'interval') {
            row.cronExpr = toIntervalCron(row.intervalHours);
          }
        }
      });
      log.debug('schedule.onFetchSchedules: mapped size', mapped.length);
      return mapped;
    } catch (e) {
      log.error('schedule.onFetchSchedules error', e);
      return collection;
    }
  },
  onFetchRuns({ context, collection = [] }) {
    try {
      const incoming = Array.isArray(collection) ? collection : [];
      log.debug('schedule.onFetchRuns: incoming size', incoming.length);
      const mapped = incoming.map((r) => ({
        id: firstDefined(r, ['id']),
        status: firstDefined(r, ['status']),
        createdAt: firstDefined(r, ['createdAt', 'created_at']),
        startedAt: firstDefined(r, ['startedAt', 'started_at']),
        completedAt: firstDefined(r, ['completedAt', 'completed_at']),
        errorMessage: firstDefined(r, ['errorMessage', 'error_message']),
      }));
      log.debug('schedule.onFetchRuns: mapped size', mapped.length);
      return mapped;
    } catch (e) {
      log.error('schedule.onFetchRuns error', e);
      return collection;
    }
  },
  // Field visibility handlers used by metadata item.on → onVisible
  showIfCron({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ds?.getFormData?.() || {};
      return String(form?.scheduleType || '').toLowerCase() === 'cron';
    } catch (_) {
      return undefined;
    }
  },
  showIfInterval({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ds?.getFormData?.() || {};
      syncOnModeTransition(ds, form);
      return normalizeMode(form) === 'interval';
    } catch (_) {
      return undefined;
    }
  },
  showIfDaily({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ds?.getFormData?.() || {};
      syncOnModeTransition(ds, form);
      return normalizeMode(form) === 'daily';
    } catch (_) {
      return undefined;
    }
  },
  showIfCustom({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ds?.getFormData?.() || {};
      syncOnModeTransition(ds, form);
      return normalizeMode(form) === 'custom';
    } catch (_) {
      return undefined;
    }
  },
  showIfNonCustom({ context }) {
    try {
      const ds = context?.Context?.('schedules')?.handlers?.dataSource;
      const form = ds?.getFormData?.() || {};
      const mode = normalizeMode(form);
      return mode === 'daily' || mode === 'interval';
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
  syncScheduleFields({ context }) {
    try {
      const ds = getSchedulesDataSource(context) || context?.handlers?.dataSource;
      if (!ds) return;
      const current = getFormState(ds);
      const withLiveInputs = applyLiveScheduleInputs(current);
      const mode = normalizeMode(withLiveInputs);
      const prevMode = String(withLiveInputs._lastScheduleMode || normalizeMode(current)).toLowerCase();
      const synced = applyScheduleSync(withLiveInputs);
      if (mode !== prevMode) {
        synced.cronExpr = transitionCronExpr(withLiveInputs, mode, prevMode);
      }
      synced._lastScheduleMode = mode;
      updateFormIfChanged(ds, synced);
    } catch (e) {
      log.warn('schedule.syncScheduleFields error', e);
    }
  },
  hideField() {
    return false;
  },
};
