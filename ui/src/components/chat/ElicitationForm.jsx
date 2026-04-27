import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Button, Dialog, Classes, Spinner } from '@blueprintjs/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client } from '../../services/agentlyClient';
import { dsTick } from '../../services/chatRuntime';
import {
  collectElicitationFormValues,
  elicitationDataBindingKey,
  extractToolApprovalMeta,
  prepareRequestedSchema,
  resolveElicitationSubmitAction,
  triggerElicitationFormSubmit
} from '../elicitationHelpers';
import {
  translateSchema,
  extractLookupBindings,
  registerLookupDataSourceServices,
} from '../lookups/forgeBridge';

export function parseConversationAndElicitation(message = {}) {
  const elicitation = message?.elicitation || {};
  const callbackURL = String(elicitation?.callbackURL || message?.callbackURL || '').trim();
  const directConversationId = String(message?.conversationId || message?.ConversationId || '').trim();
  const directElicitationId = String(
    message?.elicitationId || message?.ElicitationId
    || elicitation?.elicitationId || elicitation?.ElicitationId || ''
  ).trim();
  const match = callbackURL.match(/\/v1\/(?:api\/)?(?:conversations\/([^/]+)\/elicitation\/([^/?#]+)|elicitations\/([^/]+)\/([^/?#]+)\/resolve)/i);
  const persistedConversationId = typeof window !== 'undefined'
    ? String(window?.localStorage?.getItem('agently.selectedConversationId') || '').trim()
    : '';
  const conversationId = directConversationId
    || persistedConversationId
    || (match ? (match[1] || match[3] || '') : '');
  const elicitationId = directElicitationId || (match ? (match[2] || match[4] || '') : '');
  return {
    conversationId: String(conversationId || '').trim(),
    elicitationId: String(elicitationId || '').trim()
  };
}

export default function ElicitationForm({ message, context, onResolved = null }) {
  const elicitation = message?.elicitation || {};
  const requestedSchema = elicitation?.requestedSchema || null;
  const prompt = String(elicitation?.message || '').trim();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [closed, setClosed] = useState(false);
  const formValuesRef = useRef({});
  const formWrapperId = useRef(`elic-form-${Date.now()}`);
  const ids = useMemo(() => parseConversationAndElicitation(message), [message]);
  // First run the existing elicitation preparation, then translate any
  // server-emitted `x-ui-lookup` attachments into forge `item.lookup` and
  // register a forge-compatible Service on each referenced datasource so
  // dialog fetches route through /v1/api/datasources/{id}/fetch.
  const preparedSchema = useMemo(() => {
    const base = prepareRequestedSchema(requestedSchema);
    const translated = translateSchema(base);
    const bindings = extractLookupBindings(translated);
    if (bindings.length > 0) {
      registerLookupDataSourceServices(bindings);
    }
    return translated;
  }, [requestedSchema]);
  const approvalMeta = useMemo(() => extractToolApprovalMeta(requestedSchema), [requestedSchema]);
  const dataBindingKey = elicitationDataBindingKey(ids.elicitationId);
  const submitAction = useMemo(() => resolveElicitationSubmitAction(requestedSchema), [requestedSchema]);

  const collectFormValues = useCallback(() => {
    return collectElicitationFormValues({
      dataBindingKey,
      formWrapperId: formWrapperId.current,
      schema: requestedSchema,
      trackedValues: formValuesRef.current
    });
  }, [dataBindingKey, requestedSchema]);

  const triggerFormSubmit = useCallback(() => {
    return triggerElicitationFormSubmit(formWrapperId.current);
  }, []);

  const resolveAction = async (action, payload = null) => {
    if (!ids.conversationId || !ids.elicitationId) {
      setError('Missing conversation or elicitation id.');
      return;
    }
    let resolvedAction = action;
    let resolvedPayload = payload || collectFormValues();
    setSubmitting(true);
    setError('');
    try {
      await client.resolveElicitation(ids.conversationId, ids.elicitationId, { action: resolvedAction, payload: resolvedPayload });
      await dsTick(context, { conversationID: ids.conversationId });
      setClosed(true);
      onResolved?.(resolvedAction);
    } catch (err) {
      setError(String(err?.message || err || 'Failed to resolve elicitation'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleSubmit = async (payload) => {
    const values = payload?.values || payload?.data || payload || {};
    resolveAction(submitAction, values);
  };
  const handleDecline = async () => resolveAction('decline', {});
  const handleCancel = async () => resolveAction('cancel', {});

  if (!requestedSchema || closed) return null;

  return (
    <Dialog
      isOpen={true}
      canEscapeKeyClose={!submitting}
      canOutsideClickClose={!submitting}
      onClose={handleCancel}
      hasBackdrop={false}
      enforceFocus={false}
      autoFocus={false}
      title={approvalMeta?.title || 'Needs your input'}
      style={{ width: '100%', maxWidth: 520 }}
    >
      <div className={Classes.DIALOG_BODY}>
        {prompt ? <p style={{ marginBottom: 12 }}>{prompt}</p> : null}
        {approvalMeta?.toolName ? <p style={{ marginBottom: 12 }}><strong>Tool:</strong> {approvalMeta.toolName}</p> : null}
        {
          <div id={formWrapperId.current}>
            <SchemaBasedForm
              showSubmit={false}
              schema={preparedSchema}
              data={{}}
              dataBinding={dataBindingKey}
              context={context}
              onChange={(payload) => {
                const values = payload?.values || payload?.data || payload || {};
                formValuesRef.current = values;
              }}
              onSubmit={handleSubmit}
              disabled={submitting}
            />
          </div>
        }
        {error ? <p style={{ color: '#ef4444', marginTop: 8 }}>{error}</p> : null}
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          {submitting ? <Spinner size={16} /> : null}
          <Button minimal onClick={handleDecline} disabled={submitting}>{approvalMeta?.rejectLabel || 'Decline'}</Button>
          <Button onClick={handleCancel} disabled={submitting}>{approvalMeta?.cancelLabel || 'Cancel'}</Button>
          <Button
            intent="primary"
            onClick={() => {
              if (!triggerFormSubmit()) {
                resolveAction(submitAction);
              }
            }}
            disabled={submitting}
          >
            {approvalMeta?.acceptLabel || (submitAction === 'accept' ? 'Accept' : 'Submit')}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
