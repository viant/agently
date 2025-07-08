/* eslint no-console: ["error", { allow: ["warn", "error", "log"] }] */

// WorkflowConversation service: provides minimal polling for a read-only
// chat viewer that shows workflow execution logs.

export function onInit({ context }) {
    try {
        const convID = context?.windowParams?.conversationId || '';
        const convCtx = context.Context('conversations');

        if (convID && convCtx?.handlers?.dataSource?.setFormData) {
            convCtx.handlers.dataSource.setFormData({ values: { id: convID } });
        }

        // Start 1-second polling loop using existing chat.startPolling helper
        const chatService = context.services?.chat;
        if (!chatService?.startPolling) {
            console.warn('workflowConversation.onInit – chat.startPolling not found');
            return;
        }

        if (context.resources.workflowChatTimer) {
            clearInterval(context.resources.workflowChatTimer);
        }

        context.resources['workflowChatTimerState'] = { busy: false };
        context.resources['workflowChatTimer'] = setInterval(() => {
            chatService.startPolling({ context });
        }, 1000);
    } catch (err) {
        console.error('workflowConversation.onInit error:', err);
    }
}

export function onDestroy({ context }) {
    if (context?.resources?.workflowChatTimer) {
        clearInterval(context.resources.workflowChatTimer);
        delete context.resources.workflowChatTimer;
    }
}

export const workflowConversationService = { onInit, onDestroy };
