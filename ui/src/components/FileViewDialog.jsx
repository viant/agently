import React from 'react';
import { Dialog, Classes, Button, Tooltip } from '@blueprintjs/core';
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
import { fileViewDialogState, closeFileViewDialog } from '../utils/dialogBus.js';
import { useSignals } from '@preact/signals-react/runtime';

export default function FileViewDialog() {
  useSignals();
  const state = fileViewDialogState.value;

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

  const scrollTheme = EditorView.theme({
    '&': { height: '100%' },
    '.cm-editor': { height: '100%' },
    '.cm-scroller': { overflow: 'auto', height: '100%' },
  });

  return (
    <Dialog isOpen={state.open} onClose={closeFileViewDialog} title={state.title} style={{ width: '90vw', height: '85vh', maxWidth: '95vw', maxHeight: '95vh' }}>
      <div className={Classes.DIALOG_BODY} style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            {state.uri ? (
              <span style={{ fontSize: 11, color: 'var(--gray2)' }}>{state.uri}</span>
            ) : null}
            {state.uri ? (
              <Tooltip content="Copy uri" position={'bottom'}>
                <Button
                  small
                  minimal
                  icon="clipboard"
                  onClick={() => { try { navigator.clipboard?.writeText?.(String(state.uri || '')); } catch(_) {} }}
                />
              </Tooltip>
            ) : null}
          </div>
        </div>
        <div style={{ flex: 1, minHeight: 0 }}>
          {state.loading ? (
            <div style={{ padding: 8, color: 'var(--gray2)' }}>Loading…</div>
          ) : (
            <CodeMirror
              value={String(state.content || '')}
              height="65vh"
              theme={oneDark}
              basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: false }}
              readOnly
              extensions={[...cmExtForPath(state.uri), scrollTheme]}
            />
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

