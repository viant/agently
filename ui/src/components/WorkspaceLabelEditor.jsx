import React, {useState} from 'react';
import {Button, InputGroup} from '@blueprintjs/core';

// A display label is saved through the existing intent-based preference writer.
export default function WorkspaceLabelEditor({label,value='',original='',disabled=false,allowEmpty=true,heading=false,onSave}) {
 const [editing,setEditing]=useState(false),[draft,setDraft]=useState('');
 const Title=heading?'h2':'span';
 const save=async event=>{event.preventDefault();if(disabled || !allowEmpty && !draft.trim())return;if(await onSave(draft.trim()))setEditing(false);};
 return <div className="workspace-label-editor">{editing ? <form onSubmit={save} aria-label={label}>
  <label>{label}<InputGroup aria-label={label} autoFocus value={draft} maxLength={120} disabled={disabled} onChange={event=>setDraft(event.target.value)}/></label>
  <div><Button type="button" disabled={disabled} onClick={()=>setEditing(false)}>Cancel</Button><Button type="submit" intent="primary" disabled={disabled || !allowEmpty && !draft.trim()} aria-label={`Save ${label}`}>Save</Button></div>
 </form> : <div className="workspace-label-editor-summary"><Title className={heading?'app-ui-settings-title':undefined}>{value || original}</Title><Button minimal disabled={disabled} aria-label={`Edit ${label}`} onClick={()=>{setDraft(value || original);setEditing(true);}}>Rename</Button>{value && allowEmpty && <Button minimal disabled={disabled} aria-label={`Reset ${label}`} onClick={()=>onSave('')}>Use original</Button>}</div>}</div>;
}
