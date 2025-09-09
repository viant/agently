import React from 'react';
import CodeMirror from '@uiw/react-codemirror';
import { json } from '@codemirror/lang-json';
import { oneDark } from '@codemirror/theme-one-dark';

export default function JsonViewer({ value, height = '60vh', readOnly = true, useCodeMirror = false }) {
  const text = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
  if (!useCodeMirror) {
    return (
      <pre className="text-xs whitespace-pre-wrap break-all" style={{ margin: 0 }}>
        {text}
      </pre>
    );
  }
  return (
    <div style={{ height, minHeight: 200 }}>
      <CodeMirror
        value={text}
        height={typeof height === 'number' ? `${height}px` : height}
        theme={oneDark}
        readOnly={readOnly}
        editable={!readOnly ? true : false}
        basicSetup={{
          lineNumbers: true,
          highlightActiveLine: false,
          autocompletion: false,
          foldGutter: true,
        }}
        extensions={[json()]}
        onChange={() => { /* read-only viewer */ }}
        style={{ fontSize: 12 }}
      />
    </div>
  );
}

