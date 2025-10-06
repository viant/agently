import { endpoints } from '../endpoint';
import { getLogger } from 'forge/utils/logger';
const log = getLogger('agently');

function joinURL(base, path) {
  const b = (base || '').replace(/\/+$/, '');
  const p = (path || '').replace(/^\/+/, '');
  return `${b}/${p}`;
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
  // Normalize model: convert boolean enabled to 0/1 expected by backend
  const enabled = (typeof form.enabled === 'boolean') ? (form.enabled ? 1 : 0) : form.enabled;
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
  saveSchedule,
  async onFetchSchedules({ context }) {
    try {
      const ds = context?.Context('schedules')?.handlers?.dataSource;
      const coll = (ds?.getCollection?.() || []).map((r) => ({
        id: r.id,
        name: r.name,
        description: r.description,
        agentRef: r.agent_ref,
        modelOverride: r.model_override,
        enabled: r.enabled,
        startAt: r.start_at,
        endAt: r.end_at,
        scheduleType: r.schedule_type,
        cronExpr: r.cron_expr,
        intervalSeconds: r.interval_seconds,
        timezone: r.timezone,
        taskPromptUri: r.task_prompt_uri,
        taskPrompt: r.task_prompt,
        nextRunAt: r.next_run_at,
        lastStatus: r.last_status,
        lastRunAt: r.last_run_at,
      }));
      ds?.setCollection?.(coll);
      // Keep form in sync if a selection exists
      const sel = ds?.getSelection?.()?.selected?.id;
      if (sel) {
        const rec = coll.find(x => x.id === sel);
        if (rec) ds?.setFormData?.({ values: rec });
      }
    } catch (_) {}
  },
};
