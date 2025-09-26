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
        const resp = await api.put?.({
            inputParameters: { name },
            body: { ...formData },
        });
        log.debug('PUT', resp);
        return resp;
    } catch (err) {
        log.error('agentService.saveAgent error', err);
        handlers?.setError?.(err);
        return false;
    } finally {
        handlers?.setLoading?.(false);
    }
}

export const agentService = {
    saveAgent,
};
