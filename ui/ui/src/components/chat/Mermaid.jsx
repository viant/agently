import React from 'react';
import mermaid from 'mermaid';

let initialized = false;
let nextID = 0;

function ensureMermaid() {
  if (initialized) return;
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: 'default'
  });
  initialized = true;
}

export default function Mermaid({ code = '' }) {
  const [svg, setSvg] = React.useState('');
  const [error, setError] = React.useState('');

  React.useEffect(() => {
    const source = String(code || '').trim();
    if (!source) {
      setSvg('');
      setError('');
      return;
    }
    ensureMermaid();
    const id = `agently-app-mermaid-${Date.now().toString(36)}-${nextID++}`;
    let cancelled = false;
    (async () => {
      try {
        const result = await mermaid.render(id, source);
        if (cancelled) return;
        setSvg(String(result?.svg || ''));
        setError('');
      } catch (err) {
        if (cancelled) return;
        setSvg('');
        setError(String(err?.message || err || 'mermaid render failed'));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [code]);

  if (error) {
    return (
      <div className="app-rich-mermaid-error">
        <div className="app-rich-mermaid-error-title">Mermaid render error</div>
        <pre>{error}</pre>
      </div>
    );
  }

  return (
    <div className="app-rich-mermaid" dangerouslySetInnerHTML={{ __html: svg }} />
  );
}

