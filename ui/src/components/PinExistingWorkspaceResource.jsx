import React, {useEffect, useMemo, useRef, useState} from 'react';
import {Button, Callout, InputGroup} from '@blueprintjs/core';
import {Icon, SelectControl} from 'forge/components';
import {layoutWindowDestinations} from './layoutNavigation';
import {listReports} from '../services/reportStoreService';
import {workspaceErrorMessage} from '../services/personalWorkspace';

// Choose only identities returned by the authorized menu or report catalog.
// Saving a pin changes presentation preferences; it never clones or runs data.
export default function PinExistingWorkspaceResource({applications, groups, saving, onPin, onCancel}) {
  const [kind, setKind] = useState('window'), [query, setQuery] = useState('');
  const [reports, setReports] = useState([]), [cursor, setCursor] = useState('');
  const [loading, setLoading] = useState(false), [error, setError] = useState('');
  const [selected, setSelected] = useState(null), [alias, setAlias] = useState('');
  const [groupId, setGroupId] = useState(groups[0]?.id || 'default'), [groupLabel, setGroupLabel] = useState('');
  const [itemId, setItemId] = useState(() => crypto.randomUUID());
  const [newGroupId] = useState(() => crypto.randomUUID());
  const generation = useRef(0);
  const windows = useMemo(() => {
    const providers = new Map(applications.map(app => [app.id, app]));
    return layoutWindowDestinations({applications}).filter(item => item.action?.type === 'window' && item.action.resourceUri && !item.disabled).map(item => ({
      kind:'window', provider:item.action.providerIdentity || providers.get(item.applicationId)?.providerIdentity || '', uri:item.action.resourceUri,
      title:item.title, application:providers.get(item.applicationId)?.title || '', parameters:item.action.parameters || {},
    })).filter(item => item.provider);
  }, [applications]);
  const reportSearch = kind === 'report' ? query.trim() : '';
  const loadReports = async (next = '', current = generation.current) => {
    setLoading(true);setError('');
    try {
      const result = await listReports({limit:25, ...(reportSearch ? {search:reportSearch} : {}), ...(next ? {cursor:next} : {})});
      if (current !== generation.current) return;
      const rows = (result.reports || []).filter(row => (row.unavailable || row.resource?.providerIdentity) && (row.resourceUri || row.artifactRef)).map(row => ({
        kind:'report',unavailable:row.unavailable,provider:row.resource?.providerIdentity,uri:row.resourceUri || row.artifactRef,title:row.title || row.name || 'Report',
        application:row.namespace || '',personal:row.namespace === 'users',parameters:{},
      }));
      setReports(previous => next ? [...previous, ...rows] : rows);setCursor(result.nextCursor || '');
    } catch (cause) {if (current === generation.current) setError(workspaceErrorMessage(cause, 'Reports could not be loaded. Try again.'));}
    finally {if (current === generation.current) setLoading(false);}
  };
  useEffect(() => {
    const current = ++generation.current;
    setSelected(null);setAlias('');setError('');setReports([]);setCursor('');
    setLoading(kind === 'report');
    let timer;
    if (kind === 'report') {
      if (reportSearch) timer = setTimeout(() => loadReports('', current), 250);
      else loadReports('', current);
    }
    return () => {clearTimeout(timer);++generation.current;};
  }, [kind, reportSearch]);
  const rows = kind === 'report' ? reports : windows.filter(row =>
    `${row.title} ${row.application}`.toLocaleLowerCase().includes(query.toLocaleLowerCase())
  );
  const submit = async event => {
    event.preventDefault();
    if (!selected || saving || groupId === '__new__' && !groupLabel.trim()) return;
    const saved = await onPin({action:'pin',groupId:groupId === '__new__' ? newGroupId : groupId,label:groupId === '__new__' ? groupLabel.trim() : groupId === 'default' ? 'Pinned' : undefined,
      pin:{id:itemId,kind:selected.kind,provider:selected.provider,uri:selected.uri,parameters:selected.parameters,alias:alias.trim() || selected.title}});
    if (saved) onCancel();
  };
  return <section className="app-ui-settings-card my-workspace-pin-picker" aria-labelledby="workspace-pin-heading"><form onSubmit={submit}>
    <h2 id="workspace-pin-heading" className="app-ui-settings-title">Pin an existing window or report</h2>
    <p>Choose an original or a personal report. Pinning saves a menu shortcut; the resource stays in its current location.</p>
    <label>Resource type<SelectControl aria-label="Resource type" value={kind} disabled={saving} onChange={event => {setQuery('');setKind(event.target.value);}}><option value="window">Window</option><option value="report">Report</option></SelectControl></label>
    <label>{kind === 'window' ? 'Find a window' : 'Find a report'}<InputGroup aria-label={kind === 'window' ? 'Find a window' : 'Find a report'} value={query} disabled={saving} onChange={event => setQuery(event.target.value)} placeholder="Name or application"/></label>
    {error && <Callout intent="danger" role="alert">{error}<Button minimal disabled={loading} type="button" onClick={() => loadReports(cursor)}>Retry loading reports</Button></Callout>}
    <ul className="my-workspace-resource-options" aria-label="Available resources">{rows.map((row,index) => <li key={JSON.stringify([row.provider,row.uri,row.parameters,index])}><Button minimal fill alignText="left" disabled={saving || row.unavailable} aria-pressed={selected === row} active={selected === row} onClick={() => {setSelected(row);setItemId(crypto.randomUUID());setAlias(row.title);}} icon={<Icon name={row.kind === 'window' ? 'lucide:panel-right' : 'lucide:chart-no-axes-combined'}/>}>{row.title}{row.unavailable && ' · Unavailable'}<span className="my-workspace-pin-kind"> · {row.application}{row.kind === 'report' ? row.personal ? ' · Personal' : ' · Original' : ''}</span></Button></li>)}</ul>
    {loading && <p role="status">Loading reports…</p>}
    {!loading && !error && rows.length === 0 && <p>{kind === 'window' ? 'No matching windows in your available navigation.' : 'No matching reports.'}</p>}
    {kind === 'report' && cursor && !error && <Button type="button" disabled={loading || saving} onClick={() => loadReports(cursor)}>Load more reports</Button>}
    {selected && <p role="status">Selected: {selected.title}</p>}
    <div className="my-workspace-pin-fields"><label>Group<SelectControl aria-label="Pin group" value={groupId} disabled={saving} onChange={event => setGroupId(event.target.value)}>{!groups.some(group => group.id === 'default') && <option value="default">Pinned</option>}{groups.map(group => <option key={group.id} value={group.id}>{group.label}</option>)}<option value="__new__">New group…</option></SelectControl></label>
      {groupId === '__new__' && <label>New group name<InputGroup aria-label="New group name" value={groupLabel} maxLength={120} disabled={saving} onChange={event => setGroupLabel(event.target.value)}/></label>}
      <label>Menu label<InputGroup aria-label="Menu label" placeholder={selected?.title || 'Use the resource name'} value={alias} maxLength={120} disabled={saving} onChange={event => setAlias(event.target.value)}/></label></div>
    <div className="my-workspace-pin-actions"><Button type="button" disabled={saving} onClick={onCancel}>Cancel</Button><Button type="submit" intent="primary" disabled={!selected || saving || groupId === '__new__' && !groupLabel.trim()} loading={saving}>Pin to My workspace</Button></div>
  </form></section>;
}
