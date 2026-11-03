import React, {useEffect, useMemo, useRef, useState} from 'react';
import {Button, Callout, Checkbox, Spinner} from '@blueprintjs/core';
import {BasicTable, SchemaBasedForm, Typography} from 'forge/components';
import {signal} from '@preact/signals-react';
import datetimeStyles from '@blueprintjs/datetime/lib/css/blueprint-datetime.css?inline';
import datetime2Styles from '@blueprintjs/datetime2/lib/css/blueprint-datetime2.css?inline';
import {auditCapabilities, auditTurns, auditCSV, auditFilter, auditDuration} from '../services/conversationAudit.js';
import './conversation-audit.css';

// Shared calendar vendor styles are missing from the host baseline. Scope them
// to this destination so opening the experiment cannot restyle other windows.
const calendarStyles = `@scope (.conversation-audit) { ${datetimeStyles}\n${datetime2Styles} }`;
const dateProperties = {dateFnsFormat: 'yyyy-MM-dd HH:mm', popoverProps: {usePortal: false}};

// This guard also covers restored/direct windows. The audit view cannot mount
// or read conversation data before the server advertises the experimental UI.
export default function ConversationAuditWindow() {
  const [capabilities, setCapabilities] = useState(null);
  const [error, setError] = useState('');
  useEffect(() => {
    const controller = new AbortController();
    auditCapabilities(controller.signal).then(setCapabilities).catch(e => {
      if (!controller.signal.aborted) setError(e.message);
    });
    return () => controller.abort();
  }, []);
  if (error) return <Callout intent="danger" title="Conversation audit unavailable">{error}</Callout>;
  if (!capabilities) return <Spinner aria-label="Checking audit availability" size={20}/>;
  if (!capabilities.uiEnabled) return <Callout title="Conversation audit unavailable">The experimental audit window is disabled on this server.</Callout>;
  return <AuditView capabilities={capabilities}/>;
}

const pageSizes = [20, 50, 100];
const pagination = {pageSizeOptions: pageSizes};

function tableContext(navigate, resize) {
  const signals = {collection: signal([]), input: signal({fetch: false, page: 1, pageSize: 50}),
    control: signal({loaded: true, loading: false}), selection: signal({selected: null, rowIndex: -1, selection: []}),
    message: signal([]), form: signal({}), formStatus: signal({dirty: false, version: 1}), windowForm: signal({}), metrics: signal({})};
  const noop = () => {};
  const dataSource = {fetchCollection: noop, getCollection: () => signals.collection.peek(), peekCollection: () => signals.collection.peek(),
    getCollectionInfo: () => signals.metrics.value, isInactive: () => false, getSelection: () => signals.selection.peek(), isSelected: () => false,
    getPage: () => signals.input.value.page, setPage: navigate, setPageSize: resize,
    getFilterSets: () => [], getFilterSet: () => [], peekFilter: () => ({}), setAllSelection: noop, resetSelection: noop};
  return {identity: {windowId: 'conversation-audit', dataSourceRef: 'audit-turns'}, dataSource: {selectionMode: 'none', autoFetch: false, paging: {enabled: true, size: 50}},
    signals, handlers: {dataSource}, Context: () => null, tableSettingKey: () => 'conversation-audit-turns',
    lookupHandler: () => noop};
}

const columns = [
  ['createdAt', 'Turn time', 180, 'dateTime24'], ['id', 'Turn ID', 220],
  ['conversationId', 'Conversation ID', 220], ['conversationTitle', 'Conversation', 180],
  ['input', 'Input', 240], ['output', 'Output', 240],
  ['userId', 'User', 160], ['agentIdUsed', 'Agent', 160],
  ['status', 'Status', 120], ['models', 'Models', 240], ['inputTokens', 'Input tokens', 120],
  ['outputTokens', 'Output tokens', 120], ['executionSpanMs', 'Turn duration', 140],
  ['feedbackRating', 'Feedback', 100], ['feedbackComment', 'Feedback comment', 200],
  ['modelCalls', 'Model calls', 100], ['toolCalls', 'Tool calls', 100],
  ['conversationCreatedAt', 'Conversation created at', 180, 'dateTime24'],
].map(([id, name, width, format]) => ({id, name, width, format, visible: true, sortable: false,
  enforceColumnSize: true, showFullContent: true}));
const container = {id: 'conversation-audit-turns', fetchData: false, table: {selectionEnabled: false,
  queryDisabledWhen: {source: 'control', field: 'auditDisabled', equals: true},
  toolbar: {items: []}}};

function AuditView({capabilities}) {
  const initial = useMemo(() => ({conversationId: '', userId: capabilities.currentUserId,
    from: new Date(Date.now() - 7 * 86400000).toISOString(), to: new Date().toISOString()}), [capabilities.currentUserId]);
  const [draft, setDraft] = useState(initial);
  const [allUsers, setAllUsers] = useState(false);
  const [applied, setApplied] = useState(null);
  const [page, setPage] = useState(null);
  const [pageSize, setPageSize] = useState(50);
  const [previous, setPrevious] = useState([]);
  const [cursor, setCursor] = useState('');
  const [busy, setBusy] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [error, setError] = useState('');
  const requests = useRef({});
  const navigateRef = useRef(null);
  const resizeRef = useRef(null);
  const context = useMemo(() => tableContext(target => navigateRef.current?.(target), size => resizeRef.current?.(size)), []);
  const fields = useMemo(() => [
    {name: 'conversationId', label: 'Conversation ID (optional)', type: 'string'},
    {name: 'userId', label: 'Conversation owner ID', type: 'string'},
    {name: 'from', label: 'Conversation created from', type: 'string', format: 'date-time', properties: dateProperties},
    {name: 'to', label: 'Conversation created before', type: 'string', format: 'date-time', properties: dateProperties},
  ], []);
  useEffect(() => () => Object.values(requests.current).forEach(controller => controller.abort()), []);
  const begin = name => {
    requests.current[name]?.abort();
    const controller = new AbortController(); requests.current[name] = controller; return controller;
  };
  const filter = () => {
    if (capabilities.canAuditAllUsers && !allUsers && !draft.userId?.trim()) throw new Error('Enter a user ID or explicitly select All users.');
    return auditFilter({...draft, userId: allUsers ? '' : draft.userId});
  };
  let dirty = true;
  try { dirty = JSON.stringify(filter()) !== JSON.stringify(applied); } catch {}
  useEffect(() => {
    context.signals.input.value = {fetch: false, page: previous.length + 1, pageSize};
    context.signals.metrics.value = {hasMore: !!page?.nextCursor, hasMoreExplicit: true};
    context.signals.collection.value = (page?.rows || []).map(row => ({...row,
      agentIdUsed: row.agentIdUsed || 'unknown', models: row.models?.join(', ') || 'unknown',
      inputTokens: row.inputTokens ?? 'unknown', outputTokens: row.outputTokens ?? 'unknown',
      executionSpanMs: auditDuration(row.executionSpanMs),
    }));
    context.signals.control.value = {loaded: true, loading: busy, auditDisabled: busy || dirty};
  }, [page, pageSize, previous.length, busy, dirty, context]);

  const search = async (query, next = '', back = [], size = pageSize) => {
    const controller = begin('list');
    setBusy(true); setError('');
    try {
      const result = await auditTurns({...query, includeContent: true, limit: size, ...(next ? {cursor: next} : {})}, controller.signal);
      if (!controller.signal.aborted) {setPage(result); setPageSize(size); setApplied(query); setCursor(next); setPrevious(back);}
    } catch (e) {if (!controller.signal.aborted) setError(e.message);}
    finally {if (!controller.signal.aborted) setBusy(false);}
  };
  const apply = () => {try {search(filter());} catch(e) {setError(e.message);}};
  navigateRef.current = target => {
    if (!applied || busy || dirty) return;
    if (target === 1) return search(applied);
    if (target === previous.length + 2 && page?.nextCursor) return search(applied, page.nextCursor, [...previous, cursor]);
    if (target === previous.length && previous.length) return search(applied, previous.at(-1), previous.slice(0, -1));
  };
  resizeRef.current = size => {
    if (!applied || busy || dirty || !pageSizes.includes(size) || size === pageSize) return;
    return search(applied, '', [], size);
  };
  const download = async () => {
    if (dirty || !applied) return;
    const controller = begin('export'); setExporting(true); setError('');
    try {
      const blob = await auditCSV(applied, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob), link = document.createElement('a');
      link.href = url; link.download = 'conversation-audit.csv'; link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch(e) {if (!controller.signal.aborted) setError(e.message);}
    finally {if (!controller.signal.aborted) setExporting(false);}
  };

  return <section className="conversation-audit" aria-label="Conversation audit">
    <style>{calendarStyles}</style>
    <Typography as="h1" typeRole="heading">Conversation audit</Typography>
    <Typography as="p" typeRole="body">Inspect saved turns and current user feedback. One exported row represents one turn.</Typography>
    <SchemaBasedForm id="conversation-audit-filter" fields={fields} data={initial} onChange={setDraft} showSubmit={false}
      style={{gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 240px), 1fr))'}}/>
    {capabilities.canAuditAllUsers && <Checkbox checked={allUsers} onChange={e => setAllUsers(e.target.checked)}>All users (instance audit access)</Checkbox>}
    <Typography as="p" typeRole="metadata">{capabilities.canAuditAllUsers
      ? 'You have audit access to all conversation owners.'
      : 'You can audit your own conversations. Other owners require additional audit access.'}</Typography>
    <Typography as="p" typeRole="metadata">Dates select when conversations were created, from inclusive to exclusive. Use a maximum of 31 days. An exact conversation ID takes precedence over dates. Times use the timezone shown in the date controls.</Typography>
    <div className="conversation-audit-actions">
      <Button intent="primary" icon="search" loading={busy} onClick={apply}>Search turns</Button>
      <Button icon="refresh" disabled={!applied || dirty || busy} onClick={() => search(applied)}>Refresh</Button>
      <Button icon="download" loading={exporting} disabled={!applied || dirty || busy || exporting} onClick={download}>Export CSV</Button>
      {dirty && applied && <Typography as="span" typeRole="metadata">Apply changed filters before exporting.</Typography>}
    </div>
    {error && <Callout intent="danger" title="Audit request failed" role="alert">{error}</Callout>}
    {!page && !busy && <Typography as="p" typeRole="body">Choose filters and search to load saved turns.</Typography>}
    {page && <>
      <Typography as="p" typeRole="metadata">Page {previous.length + 1}. {page.rows.length} turns on this page.{!page.nextCursor && ' End of results.'} Feedback: 1 positive, −1 negative, 0 no rating. CSV exports all matching turns, up to 1,000 turns and 32 MiB. Values reflect saved data at read time.</Typography>
      <div className="conversation-audit-table"><BasicTable context={context} container={container} columns={columns} pagination={pagination}/></div>
    </>}
  </section>;
}
