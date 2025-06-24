import React, { useState } from 'react';
import { Dialog, Classes, Button, Spinner } from '@blueprintjs/core';

import { endpoints } from '../endpoint';
import { joinURL } from '../utils/url';

/**
 * PolicyApproval – Dialog for policy approval prompts (role == "policyapproval").
 * Allows the user to Accept, Decline, or Cancel execution of a tool.
 */
export default function PolicyApproval({ message, context }) {
  if (!message || !message.policyApproval) return null;

  const { id, callbackURL } = message;
  const { tool, args, reason } = message.policyApproval || {};

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const closeLocal = () => {
    const collSig = context.Context('messages').signals?.collection;
    if (collSig) {
      collSig.value = (collSig.value || []).filter((m) => m.id !== id);
    }
  };

  const post = async (action) => {
    if (!callbackURL) return;
    try {
      setSubmitting(true);
      setError('');
      const URL = joinURL(endpoints.appAPI.baseURL, callbackURL);
      const resp = await fetch(URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action }),
      });
      if (!resp.ok) throw new Error(`${resp.status} ${resp.statusText}`);
      closeLocal();
    } catch (e) {
      setError(e.message || String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const onAccept = () => post('accept');
  const onDecline = () => post('decline');
  const onCancel = () => post('cancel');

  return (
    <Dialog
      isOpen={true}
      canEscapeKeyClose={!submitting}
      canOutsideClickClose={!submitting}
      title="Approval required"
      style={{ width: 'auto', maxWidth: 560 }}
    >
      <div className={Classes.DIALOG_BODY}>
        <p style={{ marginBottom: 12 }}>
          The agent wants to execute tool <strong>{tool}</strong>
        </p>

        {args && (
          <pre
            style={{
              background: '#f5f8fa',
              padding: 8,
              maxHeight: 200,
              overflow: 'auto',
              borderRadius: 3,
            }}
          >
            {JSON.stringify(args, null, 2)}
          </pre>
        )}

        {reason && <p style={{ marginTop: 8 }}>Reason: {reason}</p>}

        {error && <p style={{ color: 'red', marginTop: 8 }}>{error}</p>}
      </div>

      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          {submitting && <Spinner size={16} />}
          <Button  onClick={onDecline} disabled={submitting}>
            Decline
          </Button>
          <Button  onClick={onCancel} disabled={submitting}>
            Cancel
          </Button>
          <Button intent="primary" onClick={onAccept} disabled={submitting}>
            Accept
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
