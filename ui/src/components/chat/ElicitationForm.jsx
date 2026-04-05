import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Button, Dialog, Classes, Spinner } from '@blueprintjs/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client } from '../../services/agentlyClient';
import { dsTick } from '../../services/chatRuntime';
import ApprovalEditorFields from '../ApprovalEditorFields.jsx';
import ApprovalForgeRenderer from '../ApprovalForgeRenderer.jsx';
import { executeApprovalCallbacks } from '../../services/approvalCallbacks';
import {
  buildApprovalEditorState,
  collectElicitationFormValues,
  elicitationDataBindingKey,
  extractToolApprovalMeta,
  prepareRequestedSchema,
  serializeApprovalEditedFields,
  triggerElicitationFormSubmit
} from '../elicitationHelpers';

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
  const preparedSchema = useMemo(() => prepareRequestedSchema(requestedSchema), [requestedSchema]);
  const approvalMeta = useMemo(() => extractToolApprovalMeta(requestedSchema), [requestedSchema]);
  const dataBindingKey = elicitationDataBindingKey(ids.elicitationId);
  const [approvalValues, setApprovalValues] = useState(() => buildApprovalEditorState(approvalMeta));
  const [approvalForgeContext, setApprovalForgeContext] = useState(null);
  const [approvalForgeError, setApprovalForgeError] = useState('');

  React.useEffect(() => {
    setApprovalValues(buildApprovalEditorState(approvalMeta));
    setApprovalForgeContext(null);
    setApprovalForgeError('');
  }, [approvalMeta]);

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
    let resolvedPayload = payload || collectFormValues();
    if (approvalMeta?.forge?.containerRef && approvalForgeContext?.handlers?.dataSource?.peekFormData) {
      const formData = approvalForgeContext.handlers.dataSource.peekFormData() || {};
      if (formData.editedFields && typeof formData.editedFields === 'object') {
        resolvedPayload = { editedFields: formData.editedFields };
      }
    }
    if (approvalMeta) {
      resolvedPayload = await executeApprovalCallbacks({
        meta: approvalMeta,
        event: action,
        context,
        payload: {
          approval: approvalMeta,
          editedFields: resolvedPayload?.editedFields || {},
          originalArgs: message?.approvalArguments || {}
        }
      });
    }
    setSubmitting(true);
    setError('');
    try {
      await client.resolveElicitation(ids.conversationId, ids.elicitationId, { action, payload: resolvedPayload });
      await dsTick(context, { conversationID: ids.conversationId });
      setClosed(true);
      onResolved?.(action);
    } catch (err) {
      setError(String(err?.message || err || 'Failed to resolve elicitation'));
    } finally {
      setSubmitting(false);
    }
  };

  const handleSubmit = async (payload) => {
    const values = payload?.values || payload?.data || payload || {};
    resolveAction('accept', values);
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
      title={approvalMeta?.title || 'Input Required'}
      style={{ width: '100%', maxWidth: 520 }}
    >
      <div className={Classes.DIALOG_BODY}>
        {prompt ? <p style={{ marginBottom: 12 }}>{prompt}</p> : null}
        {approvalMeta?.toolName ? <p style={{ marginBottom: 12 }}><strong>Tool:</strong> {approvalMeta.toolName}</p> : null}
        {approvalMeta?.forge?.containerRef ? (
          <ApprovalForgeRenderer
            meta={approvalMeta}
            approvalValues={approvalValues}
            originalArgs={message?.approvalArguments || {}}
            onReady={setApprovalForgeContext}
            onError={setApprovalForgeError}
          />
        ) : approvalMeta?.editors?.length ? (
            <ApprovalEditorFields
              meta={approvalMeta}
              value={approvalValues}
            onChange={setApprovalValues}
            disabled={submitting}
          />
        ) : (
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
        )}
        {approvalForgeError ? <p style={{ color: '#b42318', marginTop: 8 }}>{approvalForgeError}</p> : null}
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
              if (approvalMeta?.forge?.containerRef && !approvalForgeContext) {
                return;
              }
              if (approvalMeta?.editors?.length) {
                resolveAction('accept', { editedFields: serializeApprovalEditedFields(approvalMeta, approvalValues) });
                return;
              }
              if (!triggerFormSubmit()) {
                resolveAction('accept');
              }
            }}
            disabled={submitting || (!!approvalMeta?.forge?.containerRef && !approvalForgeContext)}
          >
            {approvalMeta?.acceptLabel || 'Accept'}
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
