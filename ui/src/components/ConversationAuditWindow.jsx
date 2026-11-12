import {Icon as UnifiedIcon} from 'forge/components';
import React, {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {Button, Callout, Spinner} from '@blueprintjs/core';
import {BasicTable, SchemaBasedForm, Typography} from 'forge/components';
import {signal} from '@preact/signals-react';
import datetimeStyles from '@blueprintjs/datetime/lib/css/blueprint-datetime.css?inline';
import datetime2Styles from '@blueprintjs/datetime2/lib/css/blueprint-datetime2.css?inline';
import {auditCapabilities, auditTurns, auditCSV, auditJSON, auditFilter, auditDuration} from '../services/conversationAudit.js';
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
  if (!capabilities.canAudit) return <Callout title="Conversation audit unavailable">You do not have conversation audit access.</Callout>;
  if (!capabilities.uiEnabled) return <Callout title="Conversation audit unavailable">The experimental audit window is disabled on this server.</Callout>;
  return <AuditView capabilities={capabilities}/>;
}

const pageSizes = [10, 20, 50, 100];
const pagination = {pageSizeOptions: pageSizes};
const userScopes = [
  {label: 'My conversations', value: 'mine'},
  {label: 'All users', value: 'all'},
  {label: 'Exact user ID', value: 'exact'},
  {label: 'User ID contains', value: 'contains'},
];
const editableUserScope = scope => scope === 'exact' || scope === 'contains';

function tableContext(navigate, resize) {
  const signals = {collection: signal([]), input: signal({fetch: false, page: 1, pageSize: 10}),
    control: signal({loaded: true, loading: false}), selection: signal({selected: null, rowIndex: -1, selection: []}),
    message: signal([]), form: signal({}), formStatus: signal({dirty: false, version: 1}), windowForm: signal({}), metrics: signal({})};
  const noop = () => {};
  const dataSource = {fetchCollection: noop, getCollection: () => signals.collection.peek(), peekCollection: () => signals.collection.peek(),
    getCollectionInfo: () => signals.metrics.value, isInactive: () => false, getSelection: () => signals.selection.peek(), isSelected: () => false,
    getPage: () => signals.input.value.page, setPage: navigate, setPageSize: resize,
    getFilterSets: () => [], getFilterSet: () => [], peekFilter: () => ({}), setAllSelection: noop, resetSelection: noop};
  return {identity: {windowId: 'conversation-audit', dataSourceRef: 'audit-turns'}, dataSource: {selectionMode: 'none', autoFetch: false, paging: {enabled: true, size: 10}},
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
  toolbar: {items: [{id: 'auditPageStatus', type: 'status', align: 'center'}]}}};

function AuditView({capabilities}) {
  const initial = useMemo(() => ({conversationId: '', userId: capabilities.currentUserId, userScope: 'mine',
    from: new Date(Date.now() - 7 * 86400000).toISOString(), to: new Date().toISOString()}), [capabilities.currentUserId]);
  const [draft, setDraft] = useState(initial);
  const savedOwner = useRef(initial.userId);
  const userScope = capabilities.canAuditAllUsers ? draft.userScope : 'mine';
  const changeDraft = useCallback(values => {
    if (editableUserScope(userScope)) savedOwner.current = values.userId;
    const scope = capabilities.canAuditAllUsers ? values.userScope : 'mine';
    const next = {...values, userScope: scope,
      userId: scope === 'mine' ? capabilities.currentUserId : scope === 'all' ? '' : savedOwner.current};
    // SchemaBasedForm also reports synchronized data. Keep an unchanged draft
    // stable so its local state and the host do not keep resetting one another.
    setDraft(previous => Object.keys(next).length === Object.keys(previous).length
      && Object.keys(next).every(key => next[key] === previous[key]) ? previous : next);
  }, [capabilities.canAuditAllUsers, capabilities.currentUserId, userScope]);
  const [applied, setApplied] = useState(null);
  const [page, setPage] = useState(null);
  const [pageSize, setPageSize] = useState(10);
  const [totalCount, setTotalCount] = useState(null);
  const [previous, setPrevious] = useState([]);
  const [cursor, setCursor] = useState('');
  const [busy, setBusy] = useState(false);
  const [exporting, setExporting] = useState('');
  const [error, setError] = useState('');
  const requests = useRef({});
  const navigateRef = useRef(null);
  const resizeRef = useRef(null);
  const context = useMemo(() => tableContext(target => navigateRef.current?.(target), size => resizeRef.current?.(size)), []);
  const fields = useMemo(() => [
    {name: 'conversationId', label: 'Conversation ID (optional)', type: 'string'},
    ...(capabilities.canAuditAllUsers ? [{name: 'userScope', label: 'Users', type: 'string', widget: 'select', options: userScopes,
      properties: {fill: true, alignText: 'start', filterable: false}}] : []),
    {name: 'userId', label: 'User ID', type: 'string', readOnly: !editableUserScope(userScope),
      disabled: userScope === 'all', properties: {placeholder: userScope === 'all' ? 'All users' : undefined},
      helperText: userScope === 'contains' ? 'Matches part of an owner ID, ignoring case.' : undefined},
    {name: 'from', label: 'Conversation created from', type: 'string', format: 'date-time', properties: dateProperties},
    {name: 'to', label: 'Conversation created before', type: 'string', format: 'date-time', properties: dateProperties},
  ], [capabilities.canAuditAllUsers, userScope]);
  useEffect(() => () => Object.values(requests.current).forEach(controller => controller.abort()), []);
  const begin = name => {
    requests.current[name]?.abort();
    const controller = new AbortController(); requests.current[name] = controller; return controller;
  };
  const filter = () => {
    const query = {conversationId: draft.conversationId, from: draft.from, to: draft.to};
    if (userScope === 'mine') query.userId = capabilities.currentUserId;
    else if (editableUserScope(userScope)) {
      if (!draft.userId?.trim()) throw new Error(userScope === 'contains'
        ? 'Enter a user ID fragment or select All users.' : 'Enter a user ID or select All users.');
      query[userScope === 'contains' ? 'userIdContains' : 'userId'] = draft.userId;
    } else if (userScope !== 'all') throw new Error('Choose a user scope.');
    return auditFilter(query);
  };
  let dirty = true;
  try { dirty = JSON.stringify(filter()) !== JSON.stringify(applied); } catch {}
  useEffect(() => {
    context.signals.input.value = {fetch: false, page: previous.length + 1, pageSize};
    context.signals.metrics.value = {hasMore: !!page?.nextCursor, hasMoreExplicit: true};
    // Keep totals out of Forge's paging metrics: Last requires a known cursor.
    context.signals.form.value = {auditPageStatus: totalCount === null ? `Page ${previous.length + 1}`
      : `Page ${previous.length + 1} of ${Math.max(1, Math.ceil(totalCount / pageSize))}`};
    context.signals.collection.value = (page?.rows || []).map(row => ({...row,
      agentIdUsed: row.agentIdUsed || 'unknown', models: row.models?.join(', ') || 'unknown',
      inputTokens: row.inputTokens ?? 'unknown', outputTokens: row.outputTokens ?? 'unknown',
      executionSpanMs: auditDuration(row.executionSpanMs),
    }));
    context.signals.control.value = {loaded: true, loading: busy, auditDisabled: busy || dirty};
  }, [page, pageSize, totalCount, previous.length, busy, dirty, context]);

  const search = async (query, next = '', back = [], size = pageSize, includeTotal = false) => {
    const controller = begin('list');
    setBusy(true); setError('');
    try {
      const result = await auditTurns({...query, includeContent: true, ...(includeTotal ? {includeTotal: true} : {}),
        limit: size, ...(next ? {cursor: next} : {})}, controller.signal);
      if (!controller.signal.aborted) {
        setPage(result); setPageSize(size); setApplied(query); setCursor(next); setPrevious(back);
        if (includeTotal) setTotalCount(result.totalCount ?? null);
      }
    } catch (e) {if (!controller.signal.aborted) setError(e.message);}
    finally {if (!controller.signal.aborted) setBusy(false);}
  };
  const apply = () => {try {search(filter(), '', [], pageSize, true);} catch(e) {setError(e.message);}};
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
  const download = async format => {
    if (dirty || !applied || busy || exporting) return;
    const controller = begin('export'); setExporting(format); setError('');
    try {
      const blob = await (format === 'json' ? auditJSON : auditCSV)(applied, controller.signal);
      if (controller.signal.aborted) return;
      const url = URL.createObjectURL(blob), link = document.createElement('a');
      link.href = url; link.download = `conversation-audit.${format}`; link.click();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch(e) {if (!controller.signal.aborted) setError(e.message);}
    finally {if (!controller.signal.aborted) setExporting('');}
  };

  return <section className="conversation-audit" aria-label="Conversation audit">
    <style>{calendarStyles}</style>
    <Typography as="h1" typeRole="heading">Conversation audit</Typography>
    <Typography as="p" typeRole="body">Inspect saved turns and current user feedback. One exported row represents one turn.</Typography>
    <SchemaBasedForm id="conversation-audit-filter" fields={fields} data={draft} onChange={changeDraft} showSubmit={false}
      style={{gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 240px), 1fr))'}}/>
    <Typography as="p" typeRole="metadata">Dates select when conversations were created, from inclusive to exclusive. Use a maximum of 31 days. An exact conversation ID takes precedence over dates. Times use the timezone shown in the date controls.</Typography>
    <div className="conversation-audit-actions">
      <Button intent="primary" icon={<UnifiedIcon name="search"/>} loading={busy} onClick={apply}>Search turns</Button>
      <Button icon={<UnifiedIcon name="refresh"/>} disabled={!applied || dirty || busy} onClick={() => search(applied, '', [], pageSize, true)}>Refresh</Button>
      <Button icon={<UnifiedIcon name="download"/>} loading={exporting === 'csv'} disabled={!applied || dirty || busy || !!exporting} onClick={() => download('csv')}>Export CSV</Button>
      <Button icon={<UnifiedIcon name="download"/>} loading={exporting === 'json'} disabled={!applied || dirty || busy || !!exporting} onClick={() => download('json')}>Export JSON</Button>
      {dirty && applied && <Typography as="span" typeRole="metadata">Apply changed filters before exporting.</Typography>}
    </div>
    {error && <Callout intent="danger" title="Audit request failed" role="alert">{error}</Callout>}
    {!page && !busy && <Typography as="p" typeRole="body">Choose filters and search to load saved turns.</Typography>}
    {page && <>
      <Typography as="p" typeRole="metadata">{page.rows.length} turns on this page.{!page.nextCursor && ' End of results.'} Feedback: 1 positive, −1 negative, 0 no rating. CSV and JSON export all matching turns, up to 1,000 turns and 32 MiB. Page totals reflect the last search or refresh. Values reflect saved data at read time.</Typography>
      <div className="conversation-audit-table"><BasicTable context={context} container={container} columns={columns} pagination={pagination}/></div>
    </>}
  </section>;
}
