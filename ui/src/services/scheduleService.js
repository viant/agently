import { endpoints } from '../endpoint';
import { getLogger } from 'forge/utils/logger';
import { registerDynamicEvaluator } from 'forge/runtime/binding';
const log = getLogger('agently');

function joinURL(base, path) {
  const b = (base || '').replace(/\/+$/, '');
  const p = (path || '').replace(/^\/+/, '');
  return `${b}/${p}`;
}

// Convert 0/1 or truthy/falsy to strict boolean
function asBoolean(value) {
  return (typeof value === 'number') ? value === 1 : !!value;
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

// Save or update a schedule via datly PATCH API
export async function saveSchedule({ context }) {
  const ctx = context?.Context('schedules');
  if (!ctx) {
    log.error('scheduleService.saveSchedule: schedules context not found');
    return false;
  }
  const ds = ctx.handlers?.dataSource;
  const form = (ds?.peekFormData?.()?.values) || ds?.getFormData?.() || ds?.getSelection?.()?.selected;
  if (!form) {
    log.warn('scheduleService.saveSchedule: no form data');
    return false;
  }
  // Normalize model: ensure boolean enabled
  const enabled = asBoolean(form.enabled);
  // UI uses lowerCamel; map to write API (TitleCase)
  const payload = {
    id: form.id || '',
    name: form.name,
    description: form.description,
    agentRef: form.agentRef || form.agent,
    modelOverride: form.modelOverride,
    enabled: enabled,
    startAt: form.startAt,
    endAt: form.endAt,
    scheduleType: form.scheduleType,
    cronExpr: form.cronExpr,
    intervalSeconds: form.intervalSeconds,
    timezone: form.timezone,
    taskPromptUri: form.taskPromptUri,
    taskPrompt: form.taskPrompt,
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
    // Reload list to reflect any computed fields
    await ctx.connector.get?.({});
    return true;
  } catch (err) {
    log.error('scheduleService.saveSchedule error', err);
    ds?.setError?.(err);
    return false;
  } finally {
    ds?.setLoading?.(false);
  }
}

export const scheduleService = {
  onInit({ context }) {
    try {
      if (scheduleService._visibilityHookInstalled) return;
      registerDynamicEvaluator('onVisible', ({ item, context: ctx }) => {
        try {
          const ds = ctx?.Context?.('schedules')?.handlers?.dataSource;
          const form = ds?.getFormData?.() || {};
          const t = String(form?.scheduleType || '').toLowerCase();
          if (item?.id === 'cronExpr') return t === 'cron';
          if (item?.id === 'intervalSeconds') return t === 'interval';
        } catch (_) { /* ignore */ }
        return undefined;
      });
      scheduleService._visibilityHookInstalled = true;
    } catch (e) {
      log.warn('scheduleService.onInit visibility hook error', e);
    }
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
      ctx.handlers?.setLoading?.(false);
    }
  },
  // Forge onFetch: transform raw records and return them for the datasource
  onFetchSchedules({ context, collection = [] }) {
    try {
      const incoming = Array.isArray(collection) ? collection : [];
      log.debug('schedule.onFetchSchedules: incoming size', incoming.length);
      const mapped = incoming.map((r) => ({
        id: firstDefined(r, ['id']),
        name: firstDefined(r, ['name']),
        description: firstDefined(r, ['description']),
        agentRef: firstDefined(r, ['agentRef', 'agent_ref']),
        modelOverride: firstDefined(r, ['modelOverride', 'model_override']),
        enabled: asBoolean(firstDefined(r, ['enabled'])),
        startAt: firstDefined(r, ['startAt', 'start_at']),
        endAt: firstDefined(r, ['endAt', 'end_at']),
        scheduleType: firstDefined(r, ['scheduleType', 'schedule_type']),
        cronExpr: firstDefined(r, ['cronExpr', 'cron_expr']),
        intervalSeconds: firstDefined(r, ['intervalSeconds', 'interval_seconds']),
        timezone: firstDefined(r, ['timezone']),
        taskPromptUri: firstDefined(r, ['taskPromptUri', 'task_prompt_uri']),
        taskPrompt: firstDefined(r, ['taskPrompt', 'task_prompt']),
        nextRunAt: firstDefined(r, ['nextRunAt', 'next_run_at']),
        lastStatus: firstDefined(r, ['lastStatus', 'last_status']),
        lastRunAt: firstDefined(r, ['lastRunAt', 'last_run_at']),
        createdAt: firstDefined(r, ['createdAt', 'created_at']),
        updatedAt: firstDefined(r, ['updatedAt', 'updated_at']),
      }));
      log.debug('schedule.onFetchSchedules: mapped size', mapped.length);
      return mapped;
    } catch (e) {
      log.error('schedule.onFetchSchedules error', e);
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
      return String(form?.scheduleType || '').toLowerCase() === 'interval';
    } catch (_) {
      return undefined;
    }
  },
};
