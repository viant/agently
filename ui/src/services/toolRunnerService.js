/* eslint no-console: ["error", { allow: ["warn", "error", "log"] }] */

// toolRunnerService executes a selected tool against the backend REST API
// exposed at POST /v1/api/tools/{name} and streams the result back to the
// caller (dialog).

export async function runSelected(prop) {

    console.log('toolRunnerService.runSelected', prop);
    const { context, data, setFormState } = prop;

    const toolContext = context?.Context('tools')
    const sel    =toolContext?.handlers?.dataSource?.peekSelection();
    if (!sel?.selected) {
        console.warn('toolRunnerService.runSelected – no tool selected');
        return false;
    }


    const api = toolContext.connector;

    const tool = sel.selected;
    const handlers = toolContext.handlers
    try {
        const name = tool.name
        const resp = await api.post?.({
            inputParameters: { toolName: name },
            body: data,
        });
        console.log('tool call', resp);
        const body =  resp.data
        const result = body?.Result
        console.log('tool call', body);
        handlers.dataSource.setFormField({item: {"id":"result"}, value:result})


        return resp;
    } catch (err) {
        console.error('oauthService.saveOauth error:', err);
        toolContext.handlers?.setError?.(err);
        return false;
    } finally {
        toolContext.handlers?.setLoading?.(false);
    }



    //
    // // Form data (parameters) is collected from the schema-form container
    // const paramsForm = dlgCtx?.Context('tools');
    // const formData   = paramsForm?.handlers?.form?.getData?.() || {};
    //
    //
    // try {
    //     const endpoint = dlgCtx.app?.endpoints?.agentlyAPI || '/';
    //     const resp = await fetch(`${endpoint}/v1/api/tools/${encodeURIComponent(tool.name)}`, {
    //         method: 'POST',
    //         headers: {
    //             'Content-Type': 'application/json',
    //         },
    //         body: JSON.stringify(formData),
    //     });
    //     return true;
    // } catch (e) {
    //     console.error(e);
    //     return false;
    // }
}

export const toolRunnerService = { runSelected };
