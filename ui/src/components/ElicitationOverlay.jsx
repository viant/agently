import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Dialog, Classes, Spinner } from '@blueprintjs/core';
import SchemaBasedForm from 'forge/widgets/SchemaBasedForm.jsx';
import { client } from '../services/agentlyClient';
import { dsTick } from '../services/chatRuntime';
import {
  collectElicitationFormValues,
  elicitationDataBindingKey,
  extractToolApprovalMeta,
  extractPlannerElicitationMeta,
  prepareRequestedSchema,
  resolveElicitationTarget,
  resolveElicitationSubmitAction,
  triggerElicitationFormSubmit
} from './elicitationHelpers';
import {
  getPendingElicitations,
  onElicitationChange,
  removePendingElicitation
} from '../services/elicitationBus';
import {
  translateSchema,
  extractLookupBindings,
  registerLookupDataSourceServices,
} from './lookups/forgeBridge';

export default function ElicitationOverlay({ context }) {
  const [pendingItems, setPendingItems] = useState(getPendingElicitations);

  useEffect(() => {
    return onElicitationChange((_next, list) => {
      setPendingItems(Array.isArray(list) ? list : getPendingElicitations());
    });
  }, []);

  const visibleItems = pendingItems.filter((item) => {
    const schema = item?.requestedSchema || null;
    const url = item?.url || '';
    const mode = item?.mode || '';
    const isOOB = !!url || mode === 'oob' || mode === 'webonly' || mode === 'url';
    return !!item && (!!schema || isOOB);
  });

  if (visibleItems.length === 0) return null;

  return (
    <>
      {visibleItems.map((item, index) => (
        <ElicitationDialog
          key={`${String(item?.conversationId || '').trim()}::${String(item?.elicitationId || '').trim()}`}
          context={context}
          pending={item}
          index={index}
          total={visibleItems.length}
        />
      ))}
    </>
  );
}

function ElicitationDialog({ context, pending, index = 0, total = 1 }) {
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [plannerRows, setPlannerRows] = useState([]);
  const formValuesRef = useRef({});
  const formWrapperId = useRef(`elic-overlay-${String(pending?.elicitationId || 'local').trim() || 'local'}-${Date.now()}`);

  const schema = pending?.requestedSchema || null;
  const prompt = pending?.message || '';
  const url = pending?.url || '';
  const mode = pending?.mode || '';
  const conversationId = pending?.conversationId || '';
  const elicitationId = pending?.elicitationId || '';
  const resolveTarget = useMemo(() => resolveElicitationTarget(pending, conversationId), [pending, conversationId]);
  const resolveConversationId = resolveTarget.conversationId || conversationId;
  const resolveElicitationId = resolveTarget.elicitationId || elicitationId;
  const resolvedStatus = String(pending?.status || '').trim().toLowerCase();
  const isResolvedHistory = !!resolvedStatus && !['pending', 'open', 'waiting_for_user', 'running'].includes(resolvedStatus);
  const plannerMeta = useMemo(() => extractPlannerElicitationMeta(schema), [schema]);

  const isOOB = !!url || mode === 'oob' || mode === 'webonly' || mode === 'url';
  const hasSchemaProps = !!(schema && typeof schema === 'object' && schema.properties && Object.keys(schema.properties).length > 0);

  // Mirror ElicitationForm: run the default elicitation preparation first,
  // then let the lookup bridge turn server-emitted `x-ui-lookup` attachments
  // into forge `item.lookup` shape AND register a forge-compatible Service
  // on each referenced datasource so dialog fetches route through our
  // /v1/api/datasources/{id}/fetch endpoint instead of forge's default HTTP
  // Service.
  const preparedSchema = useMemo(() => {
    const base = prepareRequestedSchema(schema);
    if (plannerMeta?.field && base?.properties && typeof base.properties === 'object') {
      const clone = JSON.parse(JSON.stringify(base));
      delete clone.properties[plannerMeta.field];
      if (Array.isArray(clone.required)) {
        clone.required = clone.required.filter((key) => key !== plannerMeta.field);
      }
      const translated = translateSchema(clone);
      const bindings = extractLookupBindings(translated);
      if (bindings.length > 0) {
        registerLookupDataSourceServices(bindings);
      }
      return translated;
    }
    const translated = translateSchema(base);
    const bindings = extractLookupBindings(translated);
    if (bindings.length > 0) {
      registerLookupDataSourceServices(bindings);
    }
    return translated;
  }, [schema, plannerMeta]);
  const approvalMeta = useMemo(() => extractToolApprovalMeta(schema), [schema]);
  const submitAction = useMemo(() => resolveElicitationSubmitAction(schema), [schema]);
  const hasPreparedSchemaProps = !!(preparedSchema && typeof preparedSchema === 'object' && preparedSchema.properties && Object.keys(preparedSchema.properties).length > 0);

  const dataBindingKey = elicitationDataBindingKey(elicitationId);

  useEffect(() => {
    if (!plannerMeta) {
      setPlannerRows([]);
      return;
    }
    const nextRows = Array.isArray(plannerMeta.defaultRows) ? plannerMeta.defaultRows : [];
    setPlannerRows(nextRows);
    formValuesRef.current = {
      ...formValuesRef.current,
      rows: nextRows,
    };
  }, [plannerMeta]);

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
    if (!resolveConversationId || !resolveElicitationId) {
      setError('Missing conversation or elicitation id.');
      return;
    }
    let resolvedAction = action;
    let resolvedPayload = payload || collectFormValues();
    if (plannerMeta) {
      resolvedPayload = {
        ...(resolvedPayload && typeof resolvedPayload === 'object' ? resolvedPayload : {}),
        rows: plannerRows.map((row) => ({ ...row })),
      };
    }
    console.log('[ElicitationOverlay] resolve', {
      action: resolvedAction, conversationId, elicitationId,
      resolveConversationId, resolveElicitationId,
      payload: JSON.stringify(resolvedPayload).slice(0, 500)
    });
    setSubmitting(true);
    setError('');
    try {
      await client.resolveElicitation(resolveConversationId, resolveElicitationId, {
        action: resolvedAction,
        payload: resolvedPayload
      });
      removePendingElicitation({ conversationId, elicitationId }, { allConversationsForElicitation: true });
      await dsTick(context, { conversationID: conversationId || resolveConversationId });
      if (resolveConversationId && resolveConversationId !== conversationId) {
        await dsTick(context, { conversationID: resolveConversationId });
      }
    } catch (err) {
      setError(String(err?.message || err || 'Failed'));
    } finally {
      setSubmitting(false);
    }
  }, [conversationId, elicitationId, resolveConversationId, resolveElicitationId, context, collectFormValues, plannerMeta, plannerRows]);

  if (!pending || (!schema && !isOOB)) return null;

  const togglePlannerRow = (rowIndex) => {
    if (!plannerMeta) return;
    const nextRows = plannerRows.map((row, index) => (
      index === rowIndex
        ? { ...row, [plannerMeta.selectionField]: !row?.[plannerMeta.selectionField] }
        : row
    ));
    setPlannerRows(nextRows);
    formValuesRef.current = {
      ...formValuesRef.current,
      rows: nextRows,
    };
  };

  const displayURL = (() => {
    if (!url) return '';
    try { return new URL(url).host; } catch (_) { return url; }
  })();

  return (
    <Dialog
      isOpen={true}
      canEscapeKeyClose={!submitting}
      canOutsideClickClose={!submitting}
      onClose={() => (isResolvedHistory ? removePendingElicitation({ conversationId, elicitationId }, { allConversationsForElicitation: true }) : resolve('cancel'))}
      hasBackdrop={false}
      enforceFocus={false}
      autoFocus={false}
      title={`${approvalMeta?.title || 'Needs your input'}${total > 1 ? ` - ${index + 1} of ${total}` : ''}`}
      style={{ width: '50vw', minWidth: 520, maxWidth: '80vw', marginTop: index ? 32 * index : undefined }}
    >
      <div className={Classes.DIALOG_BODY}>
        {prompt ? <p style={{ marginBottom: 12 }}>{prompt}</p> : null}
        {approvalMeta?.toolName ? (
          <div style={{ marginBottom: 12 }}>
            <strong>Tool:</strong> {approvalMeta.toolName}
          </div>
        ) : null}

        {isOOB && url ? (
          <div style={{ marginBottom: 12 }}>
            <span style={{ marginRight: 6 }}>Open in browser:</span>
            <span style={{ fontWeight: 600 }}>{displayURL}</span>
          </div>
        ) : null}
        {plannerMeta ? (
          <div style={{
            display: 'grid',
            gap: 14,
            border: '1px solid #d8e1ee',
            borderRadius: 16,
            background: '#ffffff',
            padding: '16px 18px',
            marginBottom: 12,
          }}>
            <div style={{ fontSize: 16, fontWeight: 700 }}>{plannerMeta.title}</div>
            <div style={{ overflowX: 'auto' }}>
              <table style={{ width: '100%', borderCollapse: 'collapse' }}>
                <thead>
                  <tr>
                    <th style={plannerHeaderStyle}>Review</th>
                    {plannerMeta.columns.map((column) => (
                      <th key={column.key} style={plannerHeaderStyle}>{column.label}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {plannerRows.map((row, rowIndex) => (
                    <tr key={String(row?.id || row?.site_id || rowIndex)}>
                      <td style={plannerCellStyle}>
                        <label style={{ display: 'inline-flex', alignItems: 'center', gap: 8, fontWeight: 600 }}>
                          <input
                            type="checkbox"
                            checked={Boolean(row?.[plannerMeta.selectionField])}
                            onChange={() => togglePlannerRow(rowIndex)}
                            disabled={isResolvedHistory}
                          />
                          <span>{row?.[plannerMeta.selectionField] ? 'Keep' : 'Drop'}</span>
                        </label>
                      </td>
                      {plannerMeta.columns.map((column) => (
                        <td key={`${column.key}-${rowIndex}`} style={plannerCellStyle}>
                          {String(row?.[column.key] ?? '')}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        ) : null}
        {hasPreparedSchemaProps && !isOOB ? (
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
                resolve(submitAction, values);
              }}
              disabled={submitting || isResolvedHistory}
            />
          </div>
        ) : null}
        {isResolvedHistory ? (
          <p style={{ color: '#5f6b7c', marginTop: 8 }}>
            Historical review state: {resolvedStatus}.
          </p>
        ) : null}
        {error ? <p style={{ color: '#ef4444', marginTop: 8 }}>{error}</p> : null}
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          {submitting ? <Spinner size={16} /> : null}
          {isResolvedHistory ? (
            <Button onClick={() => removePendingElicitation({ conversationId, elicitationId }, { allConversationsForElicitation: true })}>Close</Button>
          ) : (
            <>
              <Button minimal onClick={() => resolve('decline')} disabled={submitting}>
                {approvalMeta?.rejectLabel || 'Decline'}
              </Button>
              {!isOOB ? (
                <Button onClick={() => resolve('cancel')} disabled={submitting}>
                  {approvalMeta?.cancelLabel || 'Cancel'}
                </Button>
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
                    resolve(submitAction);
                  }
                }}>
                  {approvalMeta?.acceptLabel || 'Submit'}
                </Button>
              )}
            </>
          )}
        </div>
      </div>
    </Dialog>
  );
}

const plannerHeaderStyle = {
  textAlign: 'left',
  color: '#6b7688',
  fontSize: 12,
  letterSpacing: '0.06em',
  textTransform: 'uppercase',
  padding: '0 0 10px',
  borderBottom: '1px solid #d8e1ee',
};

const plannerCellStyle = {
  padding: '14px 0',
  borderBottom: '1px solid #eef2f8',
  verticalAlign: 'top',
  fontSize: 14,
};
