import React, { useMemo, useState } from 'react';
import { Button, Dialog, Classes, Spinner } from '@blueprintjs/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client } from '../../services/agentlyClient';
import { dsTick } from '../../services/chatRuntime';

function parseConversationAndElicitation(message = {}) {
  const elicitation = message?.elicitation || {};
  const callbackURL = String(elicitation?.callbackURL || message?.callbackURL || '').trim();
  const directConversationId = String(message?.conversationId || message?.ConversationId || '').trim();
  const directElicitationId = String(
    message?.elicitationId || message?.ElicitationId
    || elicitation?.elicitationId || elicitation?.ElicitationId || ''
  ).trim();
  const match = callbackURL.match(/\/v1\/(?:api\/)?(?:conversations\/([^/]+)\/elicitation\/([^/?#]+)|elicitations\/([^/]+)\/([^/?#]+)\/resolve)/i);
  const conversationId = directConversationId
    || String(window?.localStorage?.getItem('agently.selectedConversationId') || '').trim()
    || (match ? (match[1] || match[3] || '') : '');
  const elicitationId = directElicitationId || (match ? (match[2] || match[4] || '') : '');
  return {
    conversationId: String(conversationId || '').trim(),
    elicitationId: String(elicitationId || '').trim()
  };
}

/**
 * ElicitationForm — Blueprint Dialog (modal) that renders a schema-based form
 * for MCP tool elicitations. Matches the original agently's ElicitionForm behavior.
 *
 * Used as both the 'form' and 'elicition' renderer in chatService.renderers.
 */
export default function ElicitationForm({ message, context, onResolved = null }) {
  const elicitation = message?.elicitation || {};
  const requestedSchema = elicitation?.requestedSchema || null;
  const prompt = String(elicitation?.message || '').trim();
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [closed, setClosed] = useState(false);
  const ids = useMemo(() => parseConversationAndElicitation(message), [message]);

  if (!requestedSchema || closed) return null;

  const resolveAction = async (action, payload = null) => {
    if (!ids.conversationId || !ids.elicitationId) {
      setError('Missing conversation or elicitation id.');
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      await client.resolveElicitation(ids.conversationId, ids.elicitationId, { action, payload: payload || {} });
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

  const preparedSchema = useMemo(() => {
    try {
      if (!requestedSchema || typeof requestedSchema !== 'object') return requestedSchema;
      const clone = JSON.parse(JSON.stringify(requestedSchema));
      const props = (clone.properties = clone.properties || {});
      Object.keys(props).forEach((key) => {
        const p = props[key];
        if (!p || typeof p !== 'object') return;
        const t = (p.type || '').toLowerCase();
        if (t === 'array') {
          if (p.default === undefined) p.default = [];
          if (p.default && !Array.isArray(p.default)) p.default = [];
        } else if (t === 'object') {
          if (p.default === undefined) p.default = {};
        }
      });
      return clone;
    } catch (_) {
      return requestedSchema;
    }
  }, [requestedSchema]);

  return (
    <Dialog
      isOpen={true}
      canEscapeKeyClose={!submitting}
      canOutsideClickClose={!submitting}
      onClose={handleCancel}
      hasBackdrop={false}
      enforceFocus={false}
      autoFocus={false}
      title="Input Required"
      style={{ width: '100%', maxWidth: 520 }}
    >
      <div className={Classes.DIALOG_BODY}>
        {prompt ? <p style={{ marginBottom: 12 }}>{prompt}</p> : null}
        <SchemaBasedForm
          showSubmit={false}
          schema={preparedSchema}
          context={context}
          onSubmit={handleSubmit}
          disabled={submitting}
        />
        {error ? <p style={{ color: '#ef4444', marginTop: 8 }}>{error}</p> : null}
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          {submitting ? <Spinner size={16} /> : null}
          <Button minimal onClick={handleDecline} disabled={submitting}>Decline</Button>
          <Button onClick={handleCancel} disabled={submitting}>Cancel</Button>
          <Button intent="primary" onClick={() => handleSubmit({})} disabled={submitting}>Accept</Button>
        </div>
      </div>
    </Dialog>
  );
}
