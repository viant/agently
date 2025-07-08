// OAuth service for managing workspace OAuth2 credentials.
// Provides save operation, mapping to PUT /v1/workspace/oauth/{name}.

/** 
 * Persists changes made in the OAuth form. Works similarly to modelService.saveModel but targets the 'oauth' DataSource context.
 * Performs a PUT request against `/v1/workspace/oauth/{name}` where `name` is extracted from the form's id.
 *
 * @param {Object} options - Options object provided by forge
 * @param {Object} options.context - SettingProvider context instance
 * @returns {Promise<boolean|Object>} - API response or false when failed
 */
export async function saveOauth({ context }) {
    const oauthCtx = context?.Context('oauth');
    if (!oauthCtx) {
        console.error('oauthService.saveOauth: oauth context not found');
        return false;
    }

    const api = oauthCtx.connector;
    const handlers = oauthCtx.handlers.dataSource;

    const formData = handlers?.getFormData?.() || handlers?.getSelection?.()?.selected;
    if (!formData) {
        console.warn('oauthService.saveOauth: no form data');
        return false;
    }

    const name = formData?.name;
    if (!name) {
        console.error('oauthService.saveOauth: name field is required');
        return false;
    }

    handlers?.setLoading?.(true);
    try {
        const resp = await api.post?.({
            inputParameters: { name },
            body: { ...formData },
        });
        return resp;
    } catch (err) {
        console.error('oauthService.saveOauth error:', err);
        handlers?.setError?.(err);
        return false;
    } finally {
        handlers?.setLoading?.(false);
    }
}

export const oauthService = {
    saveOauth,
};