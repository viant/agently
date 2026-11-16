import React from 'react';
import {Button} from '@blueprintjs/core';
import {Icon, Typography} from 'forge/components';
import './PersonalWorkspaceTiles.css';

// Pure presentation of the same prepared navigation entries. The sidebar owns
// launch dispatch for both menu items and tiles; no catalog read occurs here.
function PersonalWorkspaceTiles({title = 'My workspace', groups = [], onSelect, onManage}) {
  return <div className="personal-workspace-tiles" aria-label={`${title} shortcuts`}>
    <header><div><Typography as="h1" typeRole="heading">{title}</Typography><Typography as="p" typeRole="body">Your pinned windows and reports.</Typography></div><Button minimal disabled={!onManage} onClick={onManage}>Manage Workspace</Button></header>
    {!groups.some(group => group.children?.length) && <p>Pin a window or report to get started.</p>}
    {groups.map(group => group.children?.length ? <section key={group.id} aria-label={group.title}><Typography as="h2" typeRole="section">{group.title}</Typography><div className="personal-workspace-tile-grid">{group.children.map(item => <Button key={item.id} className="personal-workspace-tile" disabled={item.disabled} onClick={() => onSelect?.(item, [group.id])}>
      <Icon name={item.action?.type === 'window' ? 'lucide:panel-right' : 'lucide:chart-no-axes-combined'}/><span><Typography as="span" typeRole="label">{item.title}</Typography><Typography as="span" typeRole="metadata">{item.action?.type === 'window' ? 'Window' : 'Report'}</Typography></span><Icon name="lucide:arrow-right"/>
    </Button>)}</div></section> : null)}
  </div>;
}

export default React.memo(PersonalWorkspaceTiles);
