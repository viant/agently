import React, {useEffect, useRef, useState} from 'react';
import {Button, Callout, FormGroup, InputGroup, Switch, TextArea} from '@blueprintjs/core';
import {refreshWorkspaceMetadata} from '../services/agentlyClient';
import {request} from '../services/httpClient';
import {namespaceFromStarterTaskID, readStarterTaskPreferences, starterTaskPreferenceEntries} from '../services/starterTaskPreferences';

const emptyTask = {title: '', prompt: ''};

export default function StarterTaskSettings() {
  const [metadata, setMetadata] = useState(null);
  const [preferences, setPreferences] = useState(null);
  const [draft, setDraft] = useState(emptyTask);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const active = useRef(false);

  useEffect(() => {
    active.current = true;
    refreshWorkspaceMetadata().then(data => {
      if (!active.current) return;
      setMetadata(data);
      setPreferences(readStarterTaskPreferences(data));
    }).catch(() => {
      if (active.current) setError('Could not load starting tasks.');
    });
    return () => { active.current = false; };
  }, []);

  const entries = preferences ? starterTaskPreferenceEntries(metadata, preferences, {includeHidden: true}) : [];
  const save = async (next) => {
    setSaving(true);
    setStatus('');
    setError('');
    try {
      await request('/v1/api/me/preferences', {
        method: 'PATCH',
        body: JSON.stringify({starterTasks: next}),
        notify: false,
      });
      if (!active.current) return false;
      setPreferences(next);
      setStatus('Starting task preferences saved.');
      return true;
    } catch {
      if (active.current) setError('Could not save starting task preferences.');
      return false;
    } finally {
      if (active.current) setSaving(false);
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
  const edit = (task) => setDraft(preferences.own.find(entry => `personal#${entry.id}` === task.id) || emptyTask);
  const remove = (task) => save({
    ...preferences,
    order: preferences.order.filter(id => id !== task.id),
    own: preferences.own.filter(entry => `personal#${entry.id}` !== task.id),
  });
  const submit = async (event) => {
    event.preventDefault();
    const next = {id: draft.id || crypto.randomUUID(), title: draft.title.trim(), prompt: draft.prompt.trim()};
    if (!next.title || !next.prompt || saving) return;
    const own = draft.id ? preferences.own.map(task => task.id === draft.id ? next : task) : [...preferences.own, next];
    if (await save({...preferences, own})) setDraft(emptyTask);
  };

  return <section className="app-ui-settings-card" aria-label="Starting tasks">
    <div>
      <h2 className="app-ui-settings-title">Starting tasks</h2>
      <p>Choose the suggestions shown on your chat home. New namespace tasks follow their published priority until you order them.</p>
      {error && <Callout intent="danger" role="alert">{error}</Callout>}
      {!preferences && !error && <p role="status">Loading starting tasks…</p>}
      {preferences && <>
        {entries.length > 0 ? <ol>{entries.map((task, index) => <li key={task.id}>
          <span>{task.title}</span>{' '}
          <small>{task.personal ? 'Personal' : namespaceFromStarterTaskID(task.id)}</small>
          {!task.personal && <Switch inline checked={!task.hidden} disabled={saving} aria-label={`Show ${task.title}`} onChange={(event) => toggle(task.id, event.target.checked)}/>}
          <Button minimal disabled={saving || index === 0} onClick={() => move(index, -1)} aria-label={`Move ${task.title} up`}>Move up</Button>
          <Button minimal disabled={saving || index === entries.length - 1} onClick={() => move(index, 1)} aria-label={`Move ${task.title} down`}>Move down</Button>
          {task.personal && <>
            <Button minimal disabled={saving} onClick={() => edit(task)} aria-label={`Edit ${task.title}`}>Edit</Button>
            <Button minimal disabled={saving} onClick={() => remove(task)} aria-label={`Remove ${task.title}`}>Remove</Button>
          </>}
        </li>)}</ol> : <p>No namespace or personal tasks are available.</p>}
        <form onSubmit={submit}>
          <FormGroup label="Task title" labelFor="personal-starter-title">
            <InputGroup id="personal-starter-title" value={draft.title} maxLength={255} disabled={saving} onChange={event => setDraft({...draft, title: event.target.value})}/>
          </FormGroup>
          <FormGroup label="Prompt" labelFor="personal-starter-prompt">
            <TextArea fill id="personal-starter-prompt" value={draft.prompt} maxLength={16384} disabled={saving} onChange={event => setDraft({...draft, prompt: event.target.value})}/>
          </FormGroup>
          <Button type="submit" intent="primary" disabled={saving || !draft.title.trim() || !draft.prompt.trim()}>{draft.id ? 'Save task' : 'Add personal task'}</Button>
          {draft.id && <Button minimal disabled={saving} onClick={() => setDraft(emptyTask)}>Cancel edit</Button>}
        </form>
        <p role="status">{saving ? 'Saving…' : status}</p>
      </>}
    </div>
  </section>;
}
