import React, {useEffect, useRef, useState} from 'react';
import {Button} from '@blueprintjs/core';

// The recorded definition is retained for reset. Display a publisher's title
// only from a current, owner-qualified metadata read requested by the user.
export default function PersonalReportOrigin({source, getReport}) {
  const details = useRef(null);
  const [expanded, setExpanded] = useState(false);
  const [message, setMessage] = useState('');
  const [attempt, setAttempt] = useState(0);
  const [retry, setRetry] = useState(false);
  const uri = source?.uri;
  const publisher = source?.provider;

  useEffect(() => {
    if (!expanded || !uri || !publisher) return;
    let current = true;
    let expiry;
    const close = () => {
      current = false;
      if (details.current) details.current.open = false;
      setExpanded(false);
      setMessage('');
    };
    const onHidden = () => { if (document.hidden) close(); };
    const onChange = event => {
      const changed = event.detail?.report?.artifactRef || event.detail?.artifactRef;
      if (changed === uri && event.detail?.action !== 'ran') close();
    };
    const sessionEvents = ['agently:session-reset', 'agently:logout', 'agently:unauthorized', 'forge:retry-window-metadata'];
    sessionEvents.forEach(name => window.addEventListener(name, close));
    window.addEventListener('forge:report-store-changed', onChange);
    document.addEventListener('visibilitychange', onHidden);
    setMessage('Loading original report…');
    setRetry(false);
    Promise.resolve().then(() => getReport({resource:{uri}, sourceProvider:publisher, include:{}})).then(original => {
      if (!current || !details.current?.open) return;
      const until = original.resource?.validUntil;
      if (until && !until.startsWith('0001-01-01T')) {
        const remaining = Date.parse(until) - Date.now();
        if (!(remaining > 0)) { close(); return; }
        expiry = setTimeout(close, Math.min(remaining, 2147483647));
      }
      setMessage(original.title && original.title !== uri ? `Based on ${original.title}` : 'Original report');
    }).catch(error => {
      if (!current) return;
      const unavailable = [401,403,404,410].includes(error?.status);
      setMessage(unavailable ? 'Original report unavailable.' : 'Could not load the original report.');
      setRetry(!unavailable);
    });
    return () => {
      current = false;
      clearTimeout(expiry);
      sessionEvents.forEach(name => window.removeEventListener(name, close));
      window.removeEventListener('forge:report-store-changed', onChange);
      document.removeEventListener('visibilitychange', onHidden);
    };
  }, [expanded, uri, publisher, getReport, attempt]);

  if (!uri || !publisher) return <p className="personal-report-origin">Personal report</p>;
  return <details className="personal-report-origin" ref={details} onToggle={event => {
    if (expanded === event.currentTarget.open) return;
    setMessage('');
    setRetry(false);
    setExpanded(event.currentTarget.open);
  }}>
    <summary>Personal copy · Source report</summary>
    {expanded && <div role="status">{message} {retry && <Button minimal small onClick={() => setAttempt(value => value + 1)}>Retry</Button>}</div>}
  </details>;
}
