import React, { useState } from 'react';
import { Dialog, Classes, Button, Spinner } from '@blueprintjs/core';

import { endpoints } from '../endpoint';
import { joinURL } from '../utils/url';

/**
 * MCPInteraction – Blueprint Dialog that renders an approval prompt for
 * MCP user-interaction messages (role == "mcpuserinteraction").  Presents
 * the description and provides Accept / Reject / Cancel actions:
 *   • Accept   – opens the provided URL in a new browser tab and notifies
 *                the backend (action="accept").
 *   • Reject   – notifies backend with action="decline".
 *   • Cancel   – notifies backend with action="cancel" (no decision).
 *
 * The component removes itself from the local chat collection once the
 * request has been submitted successfully so that the prompt disappears
 * immediately without waiting for the next poll cycle.
 *
 * Props:
 *   message  – memory.Message with role == "mcpuserinteraction"
 *   context  – Forge window context (for chat.signals manipulation)
 */
export default function MCPInteraction({ message, context }) {
  if (!message || !message.interaction) return null;

  const { interaction, callbackURL, id } = message;
  const { url, description } = interaction || {};

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

      closeLocal(); // optimistic removal
    } catch (e) {
      setError(e.message || String(e));
    } finally {
      setSubmitting(false);
    }
  };

  const onAccept = () => {
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer');
    }
    post('accept');
  };

  const onReject = () => post('decline');
  const onCancel = () => post('cancel');

  return (
    <Dialog
      isOpen={true}
      canEscapeKeyClose={!submitting}
      canOutsideClickClose={!submitting}
      title="User interaction required"
      style={{ width: 'auto', maxWidth: 500 }}
    >
      <div className={Classes.DIALOG_BODY}>
        {description && <p style={{ marginBottom: 12 }}>{description}</p>}

        {url && (
          <p style={{ marginBottom: 12 }}>
            <a href={url} target="_blank" rel="noopener noreferrer">
              {url}
            </a>
          </p>
        )}

        {error && <p style={{ color: 'red', marginTop: 8 }}>{error}</p>}
      </div>

      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          {submitting && <Spinner size={16} />}
          <Button minimal onClick={onReject} disabled={submitting}>
            Reject
          </Button>
          <Button minimal onClick={onCancel} disabled={submitting}>
            Cancel
          </Button>
          <Button intent="primary" onClick={onAccept} disabled={submitting}>
            Accept &amp; Open
          </Button>
        </div>
      </div>
    </Dialog>
  );
}
