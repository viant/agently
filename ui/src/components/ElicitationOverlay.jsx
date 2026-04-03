import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Dialog, Classes, Spinner } from '@blueprintjs/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client } from '../services/agentlyClient';
import { dsTick } from '../services/chatRuntime';
import {
  collectElicitationFormValues,
  elicitationDataBindingKey,
  prepareRequestedSchema,
  triggerElicitationFormSubmit
} from './elicitationHelpers';
import {
  getPendingElicitation,
  clearPendingElicitation,
  onElicitationChange
} from '../services/elicitationBus';

export default function ElicitationOverlay({ context }) {
  const [pending, setPending] = useState(getPendingElicitation);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const formValuesRef = useRef({});
  const formWrapperId = useRef(`elic-overlay-${Date.now()}`);

  useEffect(() => {
    return onElicitationChange((next) => {
      setPending(next);
      setSubmitting(false);
      setError('');
      formValuesRef.current = {};
    });
  }, []);

  const schema = pending?.requestedSchema || null;
  const prompt = pending?.message || '';
  const url = pending?.url || '';
  const mode = pending?.mode || '';
  const conversationId = pending?.conversationId || '';
  const elicitationId = pending?.elicitationId || '';

  const isOOB = !!url || mode === 'oob' || mode === 'webonly' || mode === 'url';
  const hasSchemaProps = !!(schema && typeof schema === 'object' && schema.properties && Object.keys(schema.properties).length > 0);

  const preparedSchema = useMemo(() => prepareRequestedSchema(schema), [schema]);

  const dataBindingKey = elicitationDataBindingKey(elicitationId);

  // Collect form values — try multiple sources in priority order.
  const collectFormValues = useCallback(() => {
    return collectElicitationFormValues({
      dataBindingKey,
      formWrapperId: formWrapperId.current,
      schema,
      trackedValues: formValuesRef.current
    });
  }, [schema, dataBindingKey]);

  // Trigger SchemaBasedForm's internal submit button (sends properly-keyed values via onSubmit).
  const triggerFormSubmit = useCallback(() => {
    return triggerElicitationFormSubmit(formWrapperId.current);
  }, []);

  const resolve = useCallback(async (action, payload = null) => {
    if (!conversationId || !elicitationId) {
      setError('Missing conversation or elicitation id.');
      return;
    }
    const resolvedPayload = payload || collectFormValues();
    console.log('[ElicitationOverlay] resolve', {
      action, conversationId, elicitationId,
      payload: JSON.stringify(resolvedPayload).slice(0, 500)
    });
    setSubmitting(true);
    setError('');
    try {
      await client.resolveElicitation(conversationId, elicitationId, {
        action,
        payload: resolvedPayload
      });
      clearPendingElicitation();
      await dsTick(context, { conversationID: conversationId });
    } catch (err) {
      setError(String(err?.message || err || 'Failed'));
    } finally {
      setSubmitting(false);
    }
  }, [conversationId, elicitationId, context, collectFormValues]);

  if (!pending || (!schema && !isOOB)) return null;

  const displayURL = (() => {
    if (!url) return '';
    try { return new URL(url).host; } catch (_) { return url; }
  })();

  return (
    <Dialog
      isOpen={true}
      canEscapeKeyClose={!submitting}
      canOutsideClickClose={!submitting}
      onClose={() => resolve('cancel')}
      hasBackdrop={false}
      enforceFocus={false}
      autoFocus={false}
      title="Input Required"
      style={{ width: '50vw', minWidth: 520, maxWidth: '80vw' }}
    >
      <div className={Classes.DIALOG_BODY}>
        {prompt ? <p style={{ marginBottom: 12 }}>{prompt}</p> : null}

        {isOOB && url ? (
          <div style={{ marginBottom: 12 }}>
            <span style={{ marginRight: 6 }}>Open in browser:</span>
            <span style={{ fontWeight: 600 }}>{displayURL}</span>
          </div>
        ) : null}

        {hasSchemaProps && !isOOB ? (
          <div id={formWrapperId.current}>
            <SchemaBasedForm
              showSubmit={false}
              schema={preparedSchema}
              data={{}}
              dataBinding={dataBindingKey}
              transport="post"
              context={context}
              onChange={(payload) => {
                const values = payload?.values || payload?.data || payload || {};
                formValuesRef.current = values;
              }}
              onSubmit={(payload) => {
                const values = payload?.values || payload?.data || payload || {};
                resolve('accept', values);
              }}
              disabled={submitting}
            />
          </div>
        ) : null}

        {error ? <p style={{ color: '#ef4444', marginTop: 8 }}>{error}</p> : null}
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          {submitting ? <Spinner size={16} /> : null}
          <Button minimal onClick={() => resolve('decline')} disabled={submitting}>Decline</Button>
          {!isOOB ? (
            <Button onClick={() => resolve('cancel')} disabled={submitting}>Cancel</Button>
          ) : null}
          {isOOB ? (
            <Button
              intent="primary"
              disabled={submitting}
              onClick={() => {
                if (url) window.open(url, '_blank', 'noopener,noreferrer');
                resolve('accept', {});
              }}
            >
              Open
            </Button>
          ) : (
            <Button intent="primary" disabled={submitting} onClick={() => {
              // Try to trigger SchemaBasedForm's internal submit first (sends schema-keyed values).
              // If that fails, collect values ourselves.
              if (!triggerFormSubmit()) {
                resolve('accept');
              }
            }}>
              Submit
            </Button>
          )}
        </div>
      </div>
    </Dialog>
  );
}
