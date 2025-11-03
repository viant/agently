// Agent service for managing workspace agents.
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');

/**
 * Saves or updates an agent definition using the underlying DataSource
 * connector assigned to the `agents` context. The function mirrors the shape
 * and behaviour of `modelService.saveModel` so it can be used in a generic
 * way from forge toolbar definitions.
 *
 * The PUT request is routed to `/v1/workspace/agent/{name}` where the
 * `{name}` comes from the currently edited form.
 *
 * @param {Object} options - Options object provided by forge
 * @param {Object} options.context - SettingProvider context instance
 * @returns {Promise<boolean|Object>} - Response payload or false on error
 */
export async function saveAgent({ context }) {
    const agentsCtx = context?.Context('agents');
    if (!agentsCtx) {
        log.error('agentService.saveAgent: agents context not found');
        return false;
    }

    const api = agentsCtx.connector;
    const handlers = agentsCtx?.handlers?.dataSource;

    const formData = handlers?.getFormData?.() || handlers?.getSelection?.()?.selected;
    if (!formData) {
        log.warn('agentService.saveAgent: no form data');
        return false;
    }

    const name = formData?.name;
    if (!name) {
        log.error('agentService.saveAgent: name field is required');
        return false;
    }

    log.debug('agentService.saveAgent', { name });

    handlers?.setLoading?.(true);
    try {
        // Strip any UI-only helper fields before saving (e.g., edit meta)
        const { editMeta, ...toPersist } = (formData || {});
        const resp = await api.put?.({
            inputParameters: { name },
            body: { ...toPersist },
        });
        log.debug('PUT', resp);
        // Surface non-blocking warnings when present
        try {
            const data = resp && resp.data ? resp.data : resp;
            const warnings = data && data.warnings ? data.warnings : [];
            if (Array.isArray(warnings) && warnings.length) {
                warnings.forEach(w => log.warn('[agent.save] warning:', w));
            }
        } catch (_) {}
        return resp;
    } catch (err) {
        log.error('agentService.saveAgent error', err);
        handlers?.setError?.(err);
        return false;
    } finally {
        handlers?.setLoading?.(false);
    }
}

/**
 * Loads Agent Edit View meta for the currently selected agent and injects it
 * into the agents dataSource form under `editMeta` so UI controls can render
 * authoring hints without changing the stored YAML.
 */
export async function loadAgentEdit({ context }) {
    try {
        const agentsCtx = context?.Context('agents');
        if (!agentsCtx) return false;
        const sel = agentsCtx.handlers?.dataSource?.peekSelection?.();
        const name = sel?.selected?.name || sel?.selected?.id;
        if (!name) return false;

        // Build absolute URL using configured endpoints when available
        const endpoints = context?.endpoints || {};
        const base = endpoints?.agentlyAPI?.baseURL || (typeof window !== 'undefined' ? window.location.origin : '');
        const url = `${base}/v1/workspace/agent/${encodeURIComponent(name)}/edit`;

        const resp = await fetch(url, { method: 'GET' });
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const payload = await resp.json();
        const data = payload && payload.data ? payload.data : payload; // support raw or wrapped
        if (!data) return false;

        const handlers = agentsCtx.handlers?.dataSource;
        const form = handlers?.getFormData?.() || sel?.selected || {};
        handlers?.setFormData?.({ values: { ...form, editMeta: data.meta || {} } });
        return true;
    } catch (e) {
        try { getLogger('agently').error('loadAgentEdit failed', e); } catch (_) {}
        return false;
    }
}

export const agentService = {
    saveAgent,
    loadAgentEdit,
};
