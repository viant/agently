import { client } from './agentlyClient';
import { getLogger } from 'forge/utils/logger';
import { registerDynamicEvaluator } from 'forge/runtime/binding';
import { getBusSignal } from 'forge/core';
import {
  filterLookupCollection,
  normalizeWorkspaceAgentInfos,
  normalizeWorkspaceModelInfos
} from './workspaceMetadata';

const log = getLogger('agently');
const liveScheduleDraft = { dailyTime: '', intervalHours: '', cronExpr: '', lastMode: '' };

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
  const rawType = String(row._rawScheduleType || row.scheduleType || '').toLowerCase();
  const rawCron = row._rawCronExpr || row.cronExpr || '';
  const dailyParsed = parseDailyCron(rawCron);
  const intervalParsed = parseIntervalCron(rawCron);
  row.intervalHours = row._rawIntervalSeconds ? Math.max(1, Math.round(Number(row._rawIntervalSeconds) / 3600)) : (intervalParsed?.intervalHours || 24);
  row.dailyTime = dailyParsed?.dailyTime || '09:00 AM';
  row.weekdays = dailyParsed?.weekdays || ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
  if (rawType === 'interval') row.scheduleMode = 'interval';
  else if (dailyParsed) row.scheduleMode = 'daily';
  else if (rawType === 'cron') row.scheduleMode = 'custom';
  else if (intervalParsed) row.scheduleMode = 'interval';
  else if (String(rawCron || '').trim()) row.scheduleMode = 'custom';
  else row.scheduleMode = 'daily';
  row._lastScheduleMode = row.scheduleMode;
  if (!String(row.cronExpr || '').trim()) {
    if (row.scheduleMode === 'daily') row.cronExpr = toDailyCron(row.dailyTime, row.weekdays);
    else if (row.scheduleMode === 'interval') row.cronExpr = toIntervalCron(row.intervalHours);
  }
  return row;
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
  if (Array.isArray(currentCollection) && typeof ds?.setCollection === 'function') {
    const nextCollection = currentCollection.slice();
    const index = nextCollection.findIndex((item) => String(item?.id || '') === String(normalized.id || ''));
    if (index >= 0) {
      nextCollection[index] = { ...nextCollection[index], ...normalized };
      ds.setCollection(nextCollection);
    }
  }

  const selection = ds?.peekSelection?.() || ds?.getSelection?.() || {};
  const rowIndex = Number.isInteger(selection?.rowIndex) ? selection.rowIndex : -1;
  if (typeof ds?.setSelected === 'function') {
    ds.setSelected({ selected: normalized, rowIndex });
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
  const dow = uniq.length ? uniq.join(',') : '*';
  return `${parsed.mm} ${parsed.hh} * * ${dow}`;
}

function toIntervalCron(intervalHoursValue) {
  const raw = Number(intervalHoursValue);
  const every = Number.isFinite(raw) && raw > 0 ? Math.round(raw) : 24;
  return `0 */${every} * * *`;
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
  const weekdays = String(dow || '')
    .split(',')
    .map((v) => CRON_TO_WEEKDAY[String(v).trim()])
    .filter(Boolean);
  return {
    dailyTime: toAmPm(hh, mm),
    weekdays: weekdays.length ? [...new Set(weekdays)] : ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun']
  };
}

function parseIntervalCron(cronExpr) {
  const parts = String(cronExpr || '').trim().split(/\s+/).filter(Boolean);
  if (parts.length < 5) return null;
  const [min, hour, dom, mon, dow] = parts;
  if (dom !== '*' || mon !== '*' || dow !== '*') return null;
  const topOfHour = min.match(/^0$/);
  const stepHour = hour.match(/^\*\/(\d{1,2})$/);
  if (topOfHour && stepHour) {
    const every = Number(stepHour[1]);
    if (Number.isFinite(every) && every > 0) return { intervalHours: every };
  }
  if (min.match(/^(\d{1,2})$/) && hour.match(/^\*$/)) {
    return { intervalHours: 1 };
  }
  return null;
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
  const next = {
    ...form,
    scheduleType: scheduleTypeRaw !== undefined ? scheduleTypeRaw : form.scheduleType,
    cronExpr: cronRaw !== undefined ? cronRaw : form.cronExpr,
    intervalSeconds: intervalSecondsRaw !== undefined ? intervalSecondsRaw : form.intervalSeconds,
    scheduleMode: normalizeMode(form)
  };

  const dailyParsed = parseDailyCron(next.cronExpr);
  const intervalParsed = parseIntervalCron(next.cronExpr);
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

  if (next.scheduleMode === 'daily') {
    const timeValue = String(next.dailyTime || '').trim() || '09:00 AM';
    const weekdays = normalizeWeekdays(next.weekdays).length
      ? normalizeWeekdays(next.weekdays)
      : ['mon', 'tue', 'wed', 'thu', 'fri', 'sat', 'sun'];
    next.scheduleType = 'cron';
    next.dailyTime = timeValue;
    next.weekdays = weekdays;
    next.cronExpr = toDailyCron(timeValue, weekdays);
    next.intervalSeconds = null;
  } else if (next.scheduleMode === 'interval') {
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

function getSchedulesDataSource(context) {
  return context?.Context?.('schedules')?.handlers?.dataSource;
}

function getFormState(ds) {
  const peek = ds?.peekFormData?.();
  const peekValues = (peek && typeof peek === 'object' && peek.values && typeof peek.values === 'object') ? peek.values : {};
  const get = ds?.getFormData?.() || {};
  const selected = ds?.peekSelection?.()?.selected || ds?.getSelection?.()?.selected || {};
  return { ...selected, ...get, ...peekValues };
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

function panelHasRenderableRows(wrapper) {
  const body = wrapper?.querySelector?.('tbody');
  if (!body) return false;
  const rows = Array.from(body.querySelectorAll('tr'));
  if (rows.length === 0) return false;
  return rows.some((row) => {
    if (row.classList.contains('empty-row')) return false;
    const values = Array.from(row.querySelectorAll('.cell-content, td'))
      .map((cell) => String(cell.textContent || '').replace(/\u00a0/g, ' ').trim())
      .filter(Boolean);
    return values.length > 0;
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

function applyLiveScheduleInputs(form = {}) {
  if (typeof document === 'undefined') return form;
  const panel = document.querySelector('#bp6-tab-panel_form-tabs_schedule');
  if (!panel) return form;
  const next = { ...form };
  const dailyInput = panel.querySelector("input[placeholder='09:00 AM']");
  if (dailyInput?.value?.trim()) {
    next.dailyTime = dailyInput.value.trim();
    liveScheduleDraft.dailyTime = next.dailyTime;
  }
  const cronInput = panel.querySelector("input[placeholder='0 9 * * 1-5']");
  if (cronInput?.value?.trim()) {
    next.cronExpr = cronInput.value.trim();
    liveScheduleDraft.cronExpr = next.cronExpr;
  }
  const intervalInput = panel.querySelector("input[type='number']");
  if (intervalInput?.value?.trim()) {
    const n = Number(intervalInput.value);
    if (Number.isFinite(n) && n > 0) {
      next.intervalHours = Math.round(n);
      liveScheduleDraft.intervalHours = String(next.intervalHours);
    }
  }
  if (!String(next.dailyTime || '').trim() && liveScheduleDraft.dailyTime) next.dailyTime = liveScheduleDraft.dailyTime;
  if (!String(next.cronExpr || '').trim() && liveScheduleDraft.cronExpr) next.cronExpr = liveScheduleDraft.cronExpr;
  if (!(Number(next.intervalHours) > 0) && liveScheduleDraft.intervalHours) {
    const n = Number(liveScheduleDraft.intervalHours);
    if (Number.isFinite(n) && n > 0) next.intervalHours = Math.round(n);
  }
  return next;
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

function syncOnModeTransition(ds, form = {}) {
  const withLive = applyLiveScheduleInputs({ ...getFormState(ds), ...form });
  const mode = normalizeMode(withLive);
  const prevMode = String(withLive._lastScheduleMode || liveScheduleDraft.lastMode || 'daily').toLowerCase();
  const next = { ...withLive, scheduleMode: mode, _lastScheduleMode: mode };
  if (mode !== prevMode) next.cronExpr = transitionCronExpr(withLive, mode, prevMode);
  liveScheduleDraft.lastMode = mode;
  updateFormIfChanged(ds, next);
}

function setupLiveDraftListeners() {
  if (typeof document === 'undefined' || setupLiveDraftListeners._installed) return;
  const capture = (event) => {
    try {
      const el = event?.target;
      if (!el || typeof el.closest !== 'function' || !el.closest('#bp6-tab-panel_form-tabs_schedule')) return;
      if (el.matches?.("input[placeholder='09:00 AM']")) {
        liveScheduleDraft.dailyTime = String(el.value || '').trim();
      } else if (el.matches?.("input[placeholder='0 9 * * 1-5']")) {
        liveScheduleDraft.cronExpr = String(el.value || '').trim();
      } else if (el.matches?.("input[type='number']")) {
        liveScheduleDraft.intervalHours = String(el.value || '').trim();
      }
    } catch (_) {}
  };
  document.addEventListener('input', capture, true);
  document.addEventListener('change', capture, true);
  document.addEventListener('keyup', capture, true);
  setupLiveDraftListeners._installed = true;
}

async function saveSchedule({ context }) {
  if (scheduleService._saveScheduleInFlight) return true;
  const ctx = context?.Context('schedules');
  if (!ctx) {
    log.error('scheduleService.saveSchedule: schedules context not found');
    return false;
  }
  const ds = ctx.handlers?.dataSource;
  const form = getFormState(ds);
  if (!Object.keys(form).length) {
    log.warn('scheduleService.saveSchedule: no form data');
    return false;
  }
  const enabledRaw = firstDefined(form, ['enabled']);
  const mode = normalizeMode(form);
  const weekdays = normalizeWeekdays(form.weekdays);
  const intervalHoursRaw = Number(form.intervalHours);
  const intervalHours = Number.isFinite(intervalHoursRaw) && intervalHoursRaw > 0 ? intervalHoursRaw : 24;
  const computedScheduleType = mode === 'interval' ? 'interval' : 'cron';
  const existingID = String(form.id || '').trim();
  const scheduleID = existingID || newScheduleID();
  const visibility = normalizeVisibility(form.visibility);
  const payload = {
    id: scheduleID,
    name: form.name,
    description: form.description,
    agentRef: form.agentRef || form.agent,
    modelOverride: form.modelOverride,
    enabled: enabledRaw === undefined ? false : asBoolean(enabledRaw),
    startAt: form.startAt,
    endAt: form.endAt,
    scheduleType: computedScheduleType,
    cronExpr: mode === 'custom'
      ? String(form.cronExpr || '').trim()
      : (mode === 'interval' ? toIntervalCron(intervalHours) : toDailyCron(form.dailyTime, weekdays)),
    intervalSeconds: computedScheduleType === 'interval' ? Math.round(intervalHours * 3600) : null,
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
  scheduleService._saveScheduleInFlight = true;
  try {
    await client.upsertSchedules([payload]);
    let synced = null;
    try {
      synced = syncSavedSchedule(ds, await client.getSchedule(scheduleID));
    } catch (refreshErr) {
      log.warn('scheduleService.saveSchedule refresh error', refreshErr);
    }
    if (!existingID || !synced) {
      ds?.fetchCollection?.();
    }
    return true;
  } catch (err) {
    log.error('scheduleService.saveSchedule error', err);
    ds?.setError?.(err);
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

function fmtCronSummary(cronExpr) {
  if (!cronExpr) return '—';
  const expr = String(cronExpr).trim();
  const daily = parseDailyCron(expr);
  if (daily) {
    const dayCount = (daily.weekdays || []).length;
    const dayLabel = dayCount === 7 ? 'every day' : (daily.weekdays || []).map(d => d.charAt(0).toUpperCase() + d.slice(1, 3)).join(', ');
    return `${daily.dailyTime} · ${dayLabel}`;
  }
  const interval = parseIntervalCron(expr);
  if (interval) {
    return interval.intervalHours === 1 ? 'Every hour' : `Every ${interval.intervalHours}h`;
  }
  return expr;
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
    setupLiveDraftListeners();
    installScheduleEmptyStateObserver();
    setTimeout(decorateScheduleEmptyStates, 0);
    try {
      if (!scheduleService._visibilityHookInstalled) {
        registerDynamicEvaluator('onVisible', ({ item, context: ctx }) => {
          try {
            const ds = ctx?.Context?.('schedules')?.handlers?.dataSource;
            const form = ds?.getFormData?.() || {};
            const mode = normalizeMode(form);
            if (item?.id === 'dailyTime') return mode === 'daily';
            if (item?.id === 'weekdays') return mode === 'daily' || mode === 'interval';
            if (item?.id === 'intervalHours') return mode === 'interval';
            if (item?.id === 'cronExpr') return mode === 'custom';
            if (item?.id === 'scheduleType' || item?.id === 'intervalSeconds') return false;
          } catch (_) {}
          return undefined;
        });
        scheduleService._visibilityHookInstalled = true;
      }
    } catch (e) {
      log.warn('scheduleService.onInit visibility hook error', e);
    }
    try {
      const ds = getSchedulesDataSource(context);
      if (ds) {
        const synced = applyScheduleSync(applyLiveScheduleInputs(getFormState(ds)));
        synced._lastScheduleMode = normalizeMode(synced);
        updateFormIfChanged(ds, synced);
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
  onFetchRuns({ collection = [] }) {
    try {
      setTimeout(decorateScheduleEmptyStates, 0);
      const incoming = Array.isArray(collection) ? collection : [];
      return incoming.map((r) => ({
        id: firstDefined(r, ['ScheduleRunId', 'scheduleRunId', 'schedule_run_id', 'Id', 'id']),
        conversationId: firstDefined(r, ['Id', 'id', 'ConversationId', 'conversationId', 'conversation_id']),
        status: firstDefined(r, ['Status', 'status']),
        createdAt: firstDefined(r, ['CreatedAt', 'createdAt', 'created_at']),
        startedAt: firstDefined(r, ['LastActivity', 'UpdatedAt', 'updatedAt', 'updated_at', 'StartedAt', 'startedAt', 'started_at']),
        completedAt: firstDefined(r, ['LastActivity', 'UpdatedAt', 'updatedAt', 'updated_at', 'CompletedAt', 'completedAt', 'completed_at']),
        errorMessage: firstDefined(r, ['ErrorMessage', 'errorMessage', 'error_message', 'StatusMessage', 'statusMessage', 'status_message'])
      }));
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
      if (mode !== prevMode) synced.cronExpr = transitionCronExpr(withLiveInputs, mode, prevMode);
      synced._lastScheduleMode = mode;
      updateFormIfChanged(ds, synced);
    } catch (e) {
      log.warn('schedule.syncScheduleFields error', e);
    }
  },
  addNewSchedule({ context }) {
    try {
      const ds = getSchedulesDataSource(context);
      if (ds) {
        ds.handleAddNew();
      }
      // Switch to Editor tab via bus message
      const windowId = context?.identity?.windowId;
      if (windowId) {
        const bus = getBusSignal(windowId);
        bus.value = [...(bus.peek() || []), { type: 'selectTab', tabId: 'scheduleEditor' }];
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
      const raw = row?.cronExpr || row?.cron_expr;
      return fmtCronSummary(raw);
    } catch (_) { return undefined; }
  },
  formatDuration({ row }) {
    try {
      return fmtDuration(row);
    } catch (_) { return undefined; }
  }
};
