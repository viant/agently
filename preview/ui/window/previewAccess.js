export function collectPreviewAccessOptions(metadata = {}) {
  const roles = new Set(metadata.authorizationSnapshot?.principal?.roles || []);
  const features = new Set(metadata.authorizationSnapshot?.principal?.features || []);
  const add = (target, value) => {
    if (typeof value === 'string' && value.trim()) target.add(value);
    else if (Array.isArray(value)) value.forEach(v => add(target, v));
  };
  const walk = node => {
    if (!node || typeof node !== 'object') return;
    if (Array.isArray(node)) return node.forEach(walk);
    for (const key of ['roles', 'features']) if (node[key]) add(key === 'roles' ? roles : features, node[key]);
    const field = String(node.field || node.selector || '');
    const target = /(?:^|\.)roles$/.test(field) ? roles : /(?:^|\.)features$/.test(field) ? features : null;
    if (target) for (const key of ['contains','containsAny','containsAll','equals','in','value','values']) add(target,node[key]);
    for (const value of Object.values(node)) {
      if (typeof value === 'string' && /^EXPOSE_[A-Z0-9_]+$/.test(value)) features.add(value);
      walk(value);
    }
  };
  walk(metadata);
  return {roles:[...roles].sort(),features:[...features].sort()};
}

// In-memory preview principal only. Resource capabilities and real auth stay intact.
export function withPreviewPrincipal(metadata, selection) {
  if (!metadata?.authorizationSnapshot || !selection) return metadata;
  const snapshot = metadata.authorizationSnapshot;
  const principal = snapshot.principal || {};
  if (JSON.stringify(principal.roles || []) === JSON.stringify(selection.roles) && JSON.stringify(principal.features || []) === JSON.stringify(selection.features)) return metadata;
  return {...metadata, authorizationSnapshot:{...snapshot, principal:{...principal, roles:[...selection.roles],features:[...selection.features]}}};
}

export function withPreviewTableMode(metadata, mode = null) {
  if (mode !== 'compact' && mode !== 'reserve10') return metadata;
  const visit = value => {
    if (!value || typeof value !== 'object') return value;
    if (Array.isArray(value)) {
      const next=value.map(visit);return next.some((v,i)=>v!==value[i])?next:value;
    }
    let next=value;
    for(const [key,child] of Object.entries(value)) {
      if(key === '__previewRowSlots') continue;
      let updated;
      if(key==='table' && child && Array.isArray(child.columns)) {
        if(mode==='reserve10') {
          updated=child.minRows===10 && child.rowHeight===32 ? child : {...child,minRows:10,rowHeight:32};
        } else if(Number(child.minRows)>0 || child.rowHeight !== undefined) {
          const {rowHeight, __previewRowSlots, ...rest}=child;
          updated={...rest,minRows:0};
        } else updated=child;
      } else updated=visit(child);
      if(updated!==child){if(next===value)next={...value};next[key]=updated;}
    }
    return next;
  };
  return visit(metadata);
}

export function withPreviewTableWidth(metadata, mode = null) {
  if(mode !== 'adaptive' && mode !== 'trailing-space') return metadata;
  const visit = value => {
    if (!value || typeof value !== 'object') return value;
    if (Array.isArray(value)) {const next=value.map(visit);return next.some((v,i)=>v!==value[i])?next:value;}
    let next=value;
    for(const [key,child] of Object.entries(value)) {
      if(key.startsWith('__preview')) continue;
      let updated;
      if(key==='table' && child && Array.isArray(child.columns)) {
        if(mode==='trailing-space') updated=child.fillRemainingWidth === true ? child : {...child,fillRemainingWidth:true};
        else if(child.fillRemainingWidth !== undefined || child.__previewWidthMode) {
          const {fillRemainingWidth, __previewWidthMode, ...rest}=child;updated=rest;
        }else updated=child;
      }else updated=visit(child);
      if(updated!==child){if(next===value)next={...value};next[key]=updated;}
    }
    return next;
  };
  return visit(metadata);
}


export function authoredPreviewTableMode(metadata) {
  const reserved = value => {
    if (!value || typeof value !== 'object') return false;
    if (value.table && Number(value.table.minRows)>0) return true;
    return Object.entries(value).some(([key,child])=>!key.startsWith('__preview') && reserved(child));
  };
  return reserved(metadata) ? 'reserve10' : 'compact';
}


export function authoredPreviewTableWidth(metadata) {
  const fills = value => {
    if (!value || typeof value !== 'object') return false;
    if (value.table?.fillRemainingWidth === true) return true;
    return Object.entries(value).some(([key,child])=>!key.startsWith('__preview') && fills(child));
  };
  return fills(metadata) ? 'trailing-space' : 'adaptive';
}
