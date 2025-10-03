import React from 'react';
import { Dialog, Classes, Button, ButtonGroup, Tooltip } from '@blueprintjs/core';
import CodeMirror from '@uiw/react-codemirror';
import { oneDark } from '@codemirror/theme-one-dark';
import { EditorView } from '@codemirror/view';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { go } from '@codemirror/lang-go';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { sql } from '@codemirror/lang-sql';
import { yaml } from '@codemirror/lang-yaml';
import { json } from '@codemirror/lang-json';
import { codeDiffDialogState, closeCodeDiffDialog } from '../utils/dialogBus.js';
import { useSignals } from '@preact/signals-react/runtime';

export default function CodeDiffDialog() {
  useSignals();
  const [mode, setMode] = React.useState('current');
  const state = codeDiffDialogState.value;
  React.useEffect(() => {
    // Reset mode when dialog opens
    if (state.open) setMode('current');
  }, [state.open]);

  const content = mode === 'current' ? state.current : (mode === 'prev' ? state.prev : state.diff);
  const showPrev = !!(state.hasPrev && (state.prevUri || (state.prev && String(state.prev).length > 0)));
  React.useEffect(() => {
    if (!showPrev && mode === 'prev') setMode('current');
  }, [showPrev]);

  function extFromPath(p = '') {
    const s = String(p || '').toLowerCase();
    const m = s.match(/\.([a-z0-9]+)$/);
    return m ? m[1] : '';
  }
  function cmExtForPath(p = '') {
    const ext = extFromPath(p);
    switch (ext) {
      case 'js':
      case 'jsx':
        return [javascript()];
      case 'ts':
      case 'tsx':
        return [javascript({ jsx: true, typescript: true })];
      case 'py':
        return [python()];
      case 'go':
        return [go()];
      case 'html':
      case 'htm':
        return [html()];
      case 'css':
        return [css()];
      case 'sql':
        return [sql()];
      case 'yaml':
      case 'yml':
        return [yaml()];
      case 'json':
        return [json()];
      default:
        return [];
    }
  }

  // Ensure CodeMirror shows scrollbars within the editor area
  const scrollTheme = EditorView.theme({
    '&': { height: '100%' },
    '.cm-editor': { height: '100%' },
    '.cm-scroller': { overflow: 'auto', height: '100%' },
  });

  function DiffView({ text = '' }) {
    const lines = String(text || '').split(/\r?\n/);
    let key = 0;
    return (
      <pre style={{ margin: 0, overflow: 'auto', height: '100%', background: 'var(--light-gray5)', padding: 8 }}>
        {lines.map((ln) => {
          const cls = ln.startsWith('+') ? { color: '#0a5', background: 'rgba(0,160,80,0.08)' }
                     : ln.startsWith('-') ? { color: '#c33', background: 'rgba(220,60,60,0.08)' }
                     : ln.startsWith('@@') ? { color: '#555' }
                     : (ln.startsWith('---') || ln.startsWith('+++')) ? { color: '#555' }
                     : {};
          return <div key={`d-${key++}`} style={{ fontFamily: 'monospace', whiteSpace: 'pre', ...cls }}>{ln}</div>;
        })}
      </pre>
    );
  }

  return (
    <Dialog isOpen={state.open} onClose={closeCodeDiffDialog} title={state.title} style={{ width: '90vw', height: '85vh', maxWidth: '95vw', maxHeight: '95vh' }}>
      <div className={Classes.DIALOG_BODY} style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {state.currentUri ? (
              <span style={{ fontSize: 11, color: 'var(--gray2)' }}>{state.currentUri}</span>
            ) : null}
            {state.currentUri ? (
              <Tooltip content="Copy uri" position={'bottom'}>
                <Button
                  small
                  minimal
                  icon="clipboard"
                  onClick={() => { try { navigator.clipboard?.writeText?.(String(state.currentUri || '')); } catch(_) {} }}
                />
              </Tooltip>
            ) : null}
          </div>
          <ButtonGroup minimal>
            <Button icon="document" active={mode === 'current'} onClick={() => setMode('current')}>Current</Button>
            {showPrev && <Button icon="history" active={mode === 'prev'} onClick={() => setMode('prev')}>Prev</Button>}
            <Button icon="changes" active={mode === 'diff'} onClick={() => setMode('diff')}>Diff</Button>
          </ButtonGroup>
        </div>
        <div style={{ flex: 1, minHeight: 0 }}>
          {state.loading ? (
            <div style={{ padding: 8, color: 'var(--gray2)' }}>Loading…</div>
          ) : (
            mode === 'diff' ? (
              <DiffView text={content} />
            ) : (
              <CodeMirror
                value={String(content || '')}
                height="65vh"
                theme={oneDark}
                basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: false }}
                readOnly
                extensions={[...(mode === 'current' ? cmExtForPath(state.currentUri) : cmExtForPath(state.prevUri)), scrollTheme]}
              />
            )
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
