// MCP service for managing MCP servers inside the workspace.

/**
 * Persists changes made in the MCP form. Works in the same way as
 * modelService.saveModel but targets the `servers` DataSource context.
 *
 * It performs a PUT request against `/v1/workspace/mcp/{name}` where the
 * `name` parameter is extracted from the currently edited record.
 *
 * The function is intended to be referenced from forge toolbar handler:
 *   `mcp.saveServer`
 *
 * @param {Object} options - Options object passed by forge
 * @param {Object} options.context - forge SettingProvider context
 * @returns {Promise<boolean|Object>} - API response or false when failed
 */
export async function saveServer({ context }) {
    const serversCtx = context?.Context('servers');
    if (!serversCtx) {
        console.error('mcpService.saveServer: servers context not found');
        return false;
    }

    const api = serversCtx.connector;
    const handlers = serversCtx?.handlers?.dataSource;

    // Prefer explicit getFormData but fallback to current selection when the
    // DataSource is in table-only mode.
    const formData = handlers?.getFormData?.() || handlers?.getSelection?.()?.selected;
    if (!formData) {
        console.warn('mcpService.saveServer: no form data');
        return false;
    }

    const name = formData?.name;
    if (!name) {
        console.error('mcpService.saveServer: name field is required');
        return false;
    }

    console.log('mcpService.saveServer', name);

    handlers?.setLoading?.(true);
    try {
        const resp = await api.put?.({
            inputParameters: { name },
            body: { ...formData },
        });
        console.log('PUT', resp);
        return resp;
    } catch (err) {
        console.error('mcpService.saveServer error:', err);
        handlers?.setError?.(err);
        return false;
    } finally {
        handlers?.setLoading?.(false);
    }
}

export const mcpService = {
    saveServer,
};
