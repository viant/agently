import React, {useEffect, useState} from 'react';
import {client} from '../services/agentlyClient';
import {readComposerDefaults, saveComposerDefaults} from '../services/composerDefaults';
export default function ComposerDefaultsSettings() {
  const [metadata, setMetadata] = useState(null), [value,setValue] = useState({}), [error,setError] = useState('');
  useEffect(()=>{let active=true;client.getWorkspaceMetadata().then(data=>{if(active){setMetadata(data);setValue(readComposerDefaults(data));}}).catch(()=>{if(active)setError('Could not load agent and model choices.');});return()=>{active=false;};},[]);
  const update = (field, next) => {const result={...value,[field]:next};try{saveComposerDefaults(metadata,result);setValue(result);setError('');}catch{setError('Could not save defaults in this browser.');}};
  return <section className="app-ui-settings-card" aria-label="Chat defaults"><div><h2 className="app-ui-settings-title">Chat defaults</h2><p>Choose defaults for new conversations. Current conversations keep their selection.</p><div className="app-workspace-appearance-controls">
    {['agent','model'].map(field=><label key={field}>{field === 'agent' ? 'Agent' : 'Model'}<select aria-label={`Default ${field}`} disabled={!metadata} value={value[field] || ''} onChange={event=>update(field,event.target.value)}><option value="">Workspace default</option>{(metadata?.[`${field}Infos`] || []).map(item=><option key={item.id} value={item.id}>{item.name || item.label || item.id}</option>)}</select></label>)}
  </div><p role="status">{error || 'Saved automatically in this browser.'}</p></div></section>;
}
