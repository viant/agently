import React from 'react';
import { Button, Tooltip } from '@blueprintjs/core';
import CodeMirror from '@uiw/react-codemirror';
import { oneDark } from '@codemirror/theme-one-dark';
import { javascript } from '@codemirror/lang-javascript';
import { python } from '@codemirror/lang-python';
import { go } from '@codemirror/lang-go';
import { html } from '@codemirror/lang-html';
import { css } from '@codemirror/lang-css';
import { sql } from '@codemirror/lang-sql';
import { yaml } from '@codemirror/lang-yaml';
import { json } from '@codemirror/lang-json';

function mapLanguage(lang = '') {
  const v = String(lang || '').trim().toLowerCase();
  switch (v) {
    case 'js':
    case 'jsx':
    case 'javascript':
      return javascript();
    case 'ts':
    case 'tsx':
    case 'typescript':
      return javascript({ jsx: true, typescript: true });
    case 'py':
    case 'python':
      return python();
    case 'go':
      return go();
    case 'html':
    case 'htm':
      return html();
    case 'css':
      return css();
    case 'sql':
      return sql();
    case 'yaml':
    case 'yml':
      return yaml();
    case 'json':
      return json();
    default:
      return [];
  }
}

export default function CodeBlock({ value = '', language = 'plaintext', height = 'auto', maxLines = 25, showCopy = true }) {
  const [copied, setCopied] = React.useState(false);
  const extensions = mapLanguage(language);
  const text = String(value ?? '');
  const lineCount = Math.max(1, text.split('\n').length);
  const visibleLines = Math.min(lineCount, Number.isFinite(maxLines) ? maxLines : 25);
  const lineHeightPx = 18; // approx for fontSize 12
  const computedHeight = `${visibleLines * lineHeightPx + 6}px`;
  const cmHeight = height === 'auto' ? computedHeight : height;
  const isJsdom = typeof navigator !== 'undefined' && /jsdom/i.test(navigator.userAgent || '');

  const doCopy = async () => {
    try {
      if (navigator?.clipboard?.writeText) {
        await navigator.clipboard.writeText(String(value ?? ''));
      } else {
        const ta = document.createElement('textarea');
        ta.value = String(value ?? '');
        ta.style.position = 'fixed';
        ta.style.left = '-1000px';
        document.body.appendChild(ta);
        ta.select();
        document.execCommand('copy');
        document.body.removeChild(ta);
      }
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (_) {
      // no-op
    }
  };

  if (isJsdom) {
    return (
      <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', margin: 0 }}>
        <code>{text}</code>
      </pre>
    );
  }

  return (
    <div style={{ position: 'relative' }}>
      {showCopy && (
        <div style={{ position: 'absolute', top: 6, right: 6, zIndex: 2 }}>
          <Tooltip content={copied ? 'Copied' : 'Copy code'} position={'bottom'}>
            <Button
              small
              minimal
              icon={copied ? 'tick' : 'clipboard'}
              onClick={doCopy}
              aria-label="Copy code"
            />
          </Tooltip>
        </div>
      )}
      <CodeMirror
        value={value}
        height={cmHeight}
        width={'100%'}
        theme={oneDark}
        readOnly={true}
        editable={false}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: false,
          autocompletion: false,
          foldGutter: true,
        }}
        extensions={Array.isArray(extensions) ? extensions : [extensions]}
        onChange={() => { /* read-only viewer */ }}
        style={{ fontSize: 12 }}
      />
    </div>
  );
}
