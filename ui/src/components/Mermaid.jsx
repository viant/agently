import React from 'react';
import mermaid from 'mermaid';

let mermaidInit = false;
let nextId = 0;

function ensureMermaid() {
  if (!mermaidInit) {
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict' });
    mermaidInit = true;
  }
}

export default function Mermaid({ code = '', className = '', onError = null }) {
  const ref = React.useRef(null);
  const [svg, setSvg] = React.useState('');
  const [err, setErr] = React.useState(null);

  React.useEffect(() => {
    ensureMermaid();
    const src = String(code || '');
    if (!src.trim()) {
      setSvg('');
      setErr(null);
      return;
    }
    const id = `mmd-${Date.now().toString(36)}-${nextId++}`;
    let cancelled = false;
    (async () => {
      try {
        const { svg } = await mermaid.render(id, src);
        if (!cancelled) {
          setSvg(svg);
          setErr(null);
        }
      } catch (e) {
        if (!cancelled) {
          setErr(e);
          setSvg('');
          if (typeof onError === 'function') {
            try { onError(e); } catch (_) {}
          }
        }
      }
    })();
    return () => { cancelled = true; };
  }, [code]);

  if (err) {
    return (
      <div className={className} style={{ border: '1px solid var(--rose4)', borderRadius: 4, padding: 8 }}>
        <div style={{ color: 'var(--rose6)', fontSize: 12, marginBottom: 6 }}>Mermaid render error</div>
        <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontSize: 12 }}>{String(err?.message || err)}</pre>
      </div>
    );
  }
  // eslint-disable-next-line react/no-danger
  return <div ref={ref} className={className} dangerouslySetInnerHTML={{ __html: svg }} />;
}

