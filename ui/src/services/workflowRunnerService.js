/* eslint no-console: ["error", { allow: ["warn", "error", "log"] }] */

// workflowRunnerService – runs the selected workflow and opens a read-only
// chat window that streams execution logs.

export async function runSelected(prop) {
    const { context, data } = prop || {};

    const wfCtx = context?.Context('workflows');
    if (!wfCtx) {
        log.error('workflowRunnerService.runSelected – workflows context not found');
        return false;
    }

    const sel = wfCtx?.handlers?.dataSource?.peekSelection?.();
    if (!sel?.selected) {
        wfCtx.handlers?.setError?.('Select a workflow first');
        return false;
    }

    // Normalise input parameters – the schemaBasedForm returns an object with
    // arbitrary key/value pairs.
    let inputObj = {};
    try {
        if (data && typeof data === 'object' && !Array.isArray(data)) {
            inputObj = { ...data };
        }
    } catch (err) {
        log.error('Invalid input parameters', err);
    }

    const api = wfCtx.connector;

    try {
        const body = {
            location: sel.selected.name,
            input: inputObj,
        };

        const resp = await api.post?.({ body });
        const convID = resp?.data?.conversationId;

        if (!convID) {
            wfCtx.handlers?.setError?.('No conversation id returned');
            return false;
        }

        // Close dialog if present
        context.handlers?.dialog?.close?.();

        // Open read-only chat viewer
        wfCtx.app?.router?.open('WorkflowConversation', { conversationId: convID });

        return true;
    } catch (err) {
        log.error('workflowRunnerService.runSelected error', err);
        wfCtx.handlers?.setError?.(err);
        return false;
    } finally {
        wfCtx.handlers?.setLoading?.(false);
    }
}

export const workflowRunnerService = { runSelected };
import { getLogger, ForgeLog } from 'forge/utils/logger';
const log = getLogger('agently');
