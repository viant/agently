import React, {useEffect, useRef, useState} from 'react';
import {Button, Callout, Dialog, FormGroup, InputGroup, Switch, TextArea} from '@blueprintjs/core';
import {Icon} from 'forge/components';
import './StarterTaskSettings.css';
import {getAuthMeSilently, refreshWorkspaceMetadata} from '../services/agentlyClient';
import {request} from '../services/httpClient';
import {workspaceErrorMessage} from '../services/personalWorkspace';
import {namespaceFromStarterTaskID, readStarterTaskPreferences, starterTaskPreferenceEntries} from '../services/starterTaskPreferences';

const emptyTask = {title: '', prompt: ''};

export default function StarterTaskSettings() {
  const [metadata, setMetadata] = useState(null);
  const [preferences, setPreferences] = useState(null);
  const [draft, setDraft] = useState(null);
  const [search, setSearch] = useState('');
  const [reordering, setReordering] = useState(false);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const formOwner = useRef(null);
  const active = useRef(false);
  const generation=useRef(0);
  const [loadVersion,setLoadVersion]=useState(0);

  useEffect(() => {
    active.current = true;
    ++generation.current;
    let current=true;
    setError('');
    const invalidate=()=>{++generation.current;current=false;active.current=false;formOwner.current=null;setMetadata(null);setPreferences(null);setDraft(null);setSearch('');setReordering(false);setSaving(false);setStatus('');};
    const recover=()=>{invalidate();setLoadVersion(value=>value+1);};
    window.addEventListener('agently:session-reset',recover);
    window.addEventListener('agently:authorized',recover);
    window.addEventListener('agently:logout',invalidate);
    Promise.all([getAuthMeSilently(), refreshWorkspaceMetadata()]).then(([principal, data]) => {
      if (!current) return;
      if (!data?.preferenceOwnerId) throw new Error('Starting task account unavailable');
      formOwner.current=principal?.subject || principal?.username || null;
      setMetadata(data);
      setPreferences(readStarterTaskPreferences(data));
    }).catch(cause => {
      if (!current) return;
      const status=Number(cause?.status || cause?.statusCode);
      setError(status===401 ? 'Your session needs to be renewed. Sign in, then retry loading starting tasks.'
        : status===403 ? 'Starting tasks are not available to this account.'
        : 'Starting tasks could not be loaded. Retry to request them again.');
    });
    return () => {
      ++generation.current;current=false;active.current=false;
      window.removeEventListener('agently:session-reset',recover);
      window.removeEventListener('agently:authorized',recover);
      window.removeEventListener('agently:logout',invalidate);
    };
  }, [loadVersion]);

  const entries = preferences ? starterTaskPreferenceEntries(metadata, preferences, {includeHidden: true}) : [];
  const save = async (next) => {
    const currentGeneration=generation.current;
    setSaving(true);
    setStatus('');
    setError('');
    try {
      const principal = await getAuthMeSilently({refresh:true});
      if (!active.current || currentGeneration !== generation.current) return false;
      if (!formOwner.current || formOwner.current !== (principal?.subject || principal?.username)) {
        setDraft(null);
        setPreferences(null);
        setLoadVersion(version=>version+1);
        return false;
      }
      await request('/v1/api/me/preferences', {
        method: 'PATCH',
        body: JSON.stringify({expectedOwnerId: metadata.preferenceOwnerId, starterTasks: next}),
        notify: false,
      });
      if (!active.current || currentGeneration!==generation.current) return false;
      setPreferences(next);
      setStatus('Starting task preferences saved.');
      return true;
    } catch (cause) {
      if (active.current && currentGeneration===generation.current) setError(workspaceErrorMessage(cause, 'Could not confirm your starting-task changes. Reload to check before trying again.'));
      return false;
    } finally {
      if (active.current && currentGeneration===generation.current) setSaving(false);
    }
  };
  const toggle = (id, enabled) => save({
    ...preferences,
    hidden: enabled ? preferences.hidden.filter(value => value !== id) : [...preferences.hidden, id],
  });
  const move = (index, offset) => {
    const order = entries.map(task => task.id);
    [order[index], order[index + offset]] = [order[index + offset], order[index]];
    // Keep preferences for temporarily unavailable tasks so they are restored
    // only if the current published catalog admits those tasks again.
    const available = new Set(order);
    void save({...preferences, order: [...order, ...preferences.order.filter(id => !available.has(id))]});
  };
  const edit = task => setDraft(task.personal ? preferences.own.find(entry => `personal#${entry.id}` === task.id) || emptyTask : {title:task.title,prompt:task.prompt,sourceId:task.id});
  const remove = (task) => save({
    ...preferences,
    hidden: preferences.hidden.filter(id => id !== task.id),
    order: preferences.order.filter(id => id !== task.id),
    own: preferences.own.filter(entry => `personal#${entry.id}` !== task.id),
  });
  const submit = async (event) => {
    event.preventDefault();
    const next = {id: draft.id || crypto.randomUUID(), title: draft.title.trim(), prompt: draft.prompt.trim()};
    if (!next.title || !next.prompt || saving) return;
    const own = draft.id ? preferences.own.map(task => task.id === draft.id ? {...task, ...next} : task) : [...preferences.own, next];
    const nextPreferences={...preferences,own};
    if(draft.sourceId){
      nextPreferences.hidden=[...new Set([...preferences.hidden,draft.sourceId])];
      const order=entries.map(task=>task.id),index=order.indexOf(draft.sourceId);
      order.splice(index<0?order.length:index,0,`personal#${next.id}`);
      const available=new Set(order);
      nextPreferences.order=[...order,...preferences.order.filter(id=>!available.has(id))];
    }
    if (await save(nextPreferences)) { setDraft(null); setSearch(''); }
  };

  const sourceName = task => task.personal ? 'Personal' : namespaceFromStarterTaskID(task.id);
  const query = search.trim().toLowerCase();
  const shown = entries.filter(task => (task.title + ' ' + sourceName(task)).toLowerCase().includes(query));
  const visibleCount = entries.filter(task => !task.hidden).length;
  const editorTitle = draft?.sourceId ? 'Customize starting task' : draft?.id ? 'Edit personal task' : 'New personal task';

  return <section className="starter-settings" aria-label="Starting tasks">
    <header className="starter-heading">
      <div><h2>Starting tasks</h2><p>Choose which prompts appear on your chat home.</p></div>
      {preferences && <Button intent="primary" icon={<Icon name="lucide:plus"/>} disabled={saving} onClick={()=>setDraft(emptyTask)}>New task</Button>}
    </header>
    {error && !draft && <Callout intent="danger" role="alert">{error} {!preferences && <Button onClick={()=>setLoadVersion(value=>value+1)}>Retry loading starting tasks</Button>}</Callout>}
    {!preferences && !error && <p role="status">Loading starting tasks…</p>}
    {preferences && <>
      <div className="starter-toolbar">
        <InputGroup type="search" aria-label="Find a starting task" placeholder="Find by task or source" value={search}
          leftIcon="search" disabled={reordering} onChange={event=>setSearch(event.target.value)}/>
        <Button minimal active={reordering} aria-pressed={reordering} disabled={saving || entries.length<2}
          icon={<Icon name="lucide:list"/>} onClick={()=>{setSearch('');setReordering(!reordering);}}>{reordering?'Done reordering':'Reorder'}</Button>
      </div>
      <div className="starter-summary"><span>{query ? shown.length+' matches' : visibleCount+' of '+entries.length+' shown on home'}</span><span role="status">{saving ? 'Saving…' : status}</span></div>
      {shown.length ? <ol className="starter-list" aria-label="Starting tasks in home order">{shown.map((task,index)=><li key={task.id}>
        {reordering && <span className="starter-position" aria-hidden="true">{index+1}</span>}
        <div className="starter-name"><strong>{task.title}</strong><small>{sourceName(task)}</small></div>
        {reordering ? <div className="starter-actions">
          <Button minimal aria-label={`Move ${task.title} up`} icon={<Icon name="lucide:arrow-up"/>} disabled={saving || index===0} onClick={()=>move(index,-1)}/>
          <Button minimal aria-label={`Move ${task.title} down`} icon={<Icon name="lucide:arrow-down"/>} disabled={saving || index===entries.length-1} onClick={()=>move(index,1)}/>
        </div> : <>
          <div className="starter-visible"><Switch checked={!task.hidden} disabled={saving} aria-label={`Show ${task.title}`} onChange={event=>toggle(task.id,event.target.checked)}/></div>
          <Button minimal disabled={saving} aria-label={`${task.personal?'Edit':'Customize'} ${task.title}`} onClick={()=>edit(task)}
            icon={<Icon name="lucide:pencil"/>} title={task.personal?'Edit personal task':'Customize task'}/>
        </>}
      </li>)}</ol> : <p>{query ? 'No tasks match your search.' : 'No starting tasks are available.'}</p>}
      <Dialog isOpen={!!draft} title={editorTitle} onClose={()=>{if(!saving)setDraft(null);}} className="starter-editor">
        {draft && <form onSubmit={submit}>
          {draft.sourceId && <p>Your personal version replaces the original suggestion on your chat home.</p>}
          {error && <Callout role="alert" intent="danger">{error}</Callout>}
          <FormGroup label="Task title" labelFor="personal-starter-title">
            <InputGroup id="personal-starter-title" value={draft.title} maxLength={255} disabled={saving} onChange={event => setDraft({...draft, title: event.target.value})}/>
          </FormGroup>
          <FormGroup label="Prompt" labelFor="personal-starter-prompt">
            <TextArea fill id="personal-starter-prompt" value={draft.prompt} maxLength={16384} disabled={saving} onChange={event => setDraft({...draft, prompt: event.target.value})}/>
          </FormGroup>
          <div className="starter-editor-actions">
            {draft.id && <Button minimal intent="danger" disabled={saving} onClick={async()=>{if(await remove({id:'personal#'+draft.id}))setDraft(null);}}>Remove task</Button>}
            <Button disabled={saving} onClick={()=>setDraft(null)}>Cancel</Button>
            <Button type="submit" intent="primary" disabled={saving || !draft.title.trim() || !draft.prompt.trim()}>{draft.id || draft.sourceId ? 'Save personal task' : 'Add personal task'}</Button>
          </div>
        </form>}
      </Dialog>
    </>}
  </section>;
}
