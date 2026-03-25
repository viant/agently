import React from 'react';
import { Dialog, Classes, Button, ButtonGroup, Tooltip } from '@blueprintjs/core';
import { closeCodeDiffDialog, useCodeDiffDialogState } from '../utils/dialogBus';

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

function DiffView({ text = '' }) {
  return (
    <pre style={{ margin: 0, overflow: 'auto', height: '100%', background: '#101521', color: '#edf2f7', padding: 12, borderRadius: 12 }}>
      {String(text || '').split(/\r?\n/).map((line, index) => {
        const style = line.startsWith('+')
          ? { color: '#6ee7b7', background: 'rgba(16,185,129,0.12)' }
          : line.startsWith('-')
            ? { color: '#fca5a5', background: 'rgba(239,68,68,0.12)' }
            : line.startsWith('@@') || line.startsWith('---') || line.startsWith('+++')
              ? { color: '#cbd5e1' }
              : {};
        return <div key={`diff-${index}`} style={{ fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace', whiteSpace: 'pre-wrap', ...style }}>{line}</div>;
      })}
    </pre>
  );
}

export default function CodeDiffDialog() {
  const [mode, setMode] = React.useState('current');
  const state = useCodeDiffDialogState();

  React.useEffect(() => {
    if (state.open) setMode('current');
  }, [state.open]);

  const content = mode === 'current' ? state.current : (mode === 'prev' ? state.prev : state.diff);
  const showPrev = !!(state.hasPrev && (state.prevUri || (state.prev && String(state.prev).length > 0)));

  React.useEffect(() => {
    if (!showPrev && mode === 'prev') setMode('current');
  }, [mode, showPrev]);

  return (
    <Dialog isOpen={state.open} onClose={closeCodeDiffDialog} title={state.title} style={{ width: '90vw', height: '85vh', maxWidth: '95vw', maxHeight: '95vh' }}>
      <div className={Classes.DIALOG_BODY} style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {state.currentUri ? <span style={{ fontSize: 11, color: 'var(--gray2)' }}>{state.currentUri}</span> : null}
            {state.currentUri ? (
              <Tooltip content="Copy uri" position="bottom">
                <Button
                  small
                  minimal
                  icon="clipboard"
                  onClick={() => { try { navigator.clipboard?.writeText?.(String(state.currentUri || '')); } catch (_) {} }}
                />
              </Tooltip>
            ) : null}
          </div>
          <ButtonGroup minimal>
            <Button icon="document" active={mode === 'current'} onClick={() => setMode('current')}>Current</Button>
            {showPrev ? <Button icon="history" active={mode === 'prev'} onClick={() => setMode('prev')}>Prev</Button> : null}
            <Button icon="changes" active={mode === 'diff'} onClick={() => setMode('diff')}>Diff</Button>
          </ButtonGroup>
        </div>
        <div style={{ flex: 1, minHeight: 0 }}>
          {state.loading ? (
            <div style={{ padding: 8, color: 'var(--gray2)' }}>Loading…</div>
          ) : mode === 'diff' ? (
            <DiffView text={content} />
          ) : (
            <PlainCode text={content} />
          )}
        </div>
      </div>
      <div className={Classes.DIALOG_FOOTER}>
        <div className={Classes.DIALOG_FOOTER_ACTIONS}>
          <Button onClick={closeCodeDiffDialog}>Close</Button>
        </div>
      </div>
    </Dialog>
  );
}
