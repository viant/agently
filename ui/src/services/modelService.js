// Model service for managing workspace models.



export async function saveModel({ context } ) {

    const modelsCtx = context?.Context('models');
    if (!modelsCtx) {
        console.error('modelService.saveModel: models context not found');
        return false;
    }

    const api = modelsCtx.connector;
    const handlers = modelsCtx?.handlers?.dataSource;

    const formData = handlers?.getFormData?.() || handlers?.getSelection?.()?.selected;
    if (!formData) {
        console.warn('modelService.saveModel: no form data');
        return false;
    }

    const id = formData?.id;
    if(formData.meta && typeof formData.meta === 'string') {
        formData.meta  = JSON.parse(formData.meta);
    }

    console.log('modelService.saveModel', id);

    handlers?.setLoading?.(true);
    console.log('ID', id, formData)
    try {
        let resp;
            // Existing model – PUT /id to update
            resp = await api.put?.({
                inputParameters: { id },
                body: { ...formData },
            });
            console.log('PUT', resp);
        return resp;
    } catch (err) {
        console.error('modelService.saveModel error:', err);
        handlers?.setError?.(err);
        return false;
    } finally {
        handlers?.setLoading?.(false);
    }
}


export const modelService = {
    saveModel
};
