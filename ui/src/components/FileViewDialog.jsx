import React from 'react';
import { Dialog, Classes, Button, Tooltip } from '@blueprintjs/core';
import { closeFileViewDialog, useFileViewDialogState } from '../utils/dialogBus';

function PlainCode({ text = '' }) {
  return (
    <pre
      style={{
        margin: 0,
        height: '100%',
        overflow: 'auto',
        padding: 12,
        borderRadius: 12,
        background: '#101521',
        color: '#edf2f7',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
        fontSize: 12,
        lineHeight: 1.45,
        whiteSpace: 'pre-wrap'
      }}
    >
      {String(text || '')}
    </pre>
  );
}

export default function FileViewDialog() {
  const state = useFileViewDialogState();
  return (
    <Dialog isOpen={state.open} onClose={closeFileViewDialog} title={state.title} style={{ width: '90vw', height: '85vh', maxWidth: '95vw', maxHeight: '95vh' }}>
      <div className={Classes.DIALOG_BODY} style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {state.uri ? <span style={{ fontSize: 11, color: 'var(--gray2)' }}>{state.uri}</span> : null}
            {state.uri ? (
              <Tooltip content="Copy uri" position="bottom">
                <Button
                  small
                  minimal
                  icon="clipboard"
                  onClick={() => { try { navigator.clipboard?.writeText?.(String(state.uri || '')); } catch (_) {} }}
                />
              </Tooltip>
            ) : null}
          </div>
        </div>
        <div style={{ flex: 1, minHeight: 0 }}>
          {state.loading ? (
            <div style={{ padding: 8, color: 'var(--gray2)' }}>Loading…</div>
          ) : (
            <PlainCode text={state.content} />
          )}
        </div>
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          <Button onClick={closeFileViewDialog}>Close</Button>
        </div>
      </div>
    </Dialog>
  );
}
