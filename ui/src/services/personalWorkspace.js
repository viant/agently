export const PERSONAL_WORKSPACE_ID = 'agently.personal-workspace';
// UI copy only. Keep transport diagnostics on the original error, never in
// settings callouts where a server payload may contain implementation details.
export function workspaceErrorMessage(error, recovery) {
 const status=Number(error?.status || error?.statusCode);
 if(status===401)return 'Your session needs to be renewed. Sign in and try again.';
 if(status===403)return 'This action is not available to your account.';
 if(status===409)return 'Your workspace changed. Reload it before trying again.';
 return recovery;
}
export const workspacePresentation = preferences => preferences?.presentation || 'menu';
export const workspaceShowsTiles = preferences => ['tiles','both'].includes(workspacePresentation(preferences));
export const workspaceShowsMenu = preferences => workspacePresentation(preferences) !== 'tiles';
export const workspaceApplicationKey = app => JSON.stringify([app.providerIdentity || '', app.id]);
export const workspaceMenuKey = (app, path) => JSON.stringify([app.providerIdentity || '', app.id, ...path]);

function navigationLabels(app, preferences) {
 const visit = (items, parents=[]) => {
  if (!items) return items;
  const result = items.map(item => {
   const path=[...parents,item.id], children=visit(item.children,path);
   const title=preferences.menuLabels?.[workspaceMenuKey(app,path)] || item.title;
   return title===item.title && children===item.children ? item : {...item,title,...(children ? {children} : {})};
  });
  return result.every((item,index)=>item===items[index]) ? items : result;
 };
 const menus=visit(app.menus),title=preferences.applicationLabels?.[workspaceApplicationKey(app)] || app.title;
 return menus===app.menus && title===app.title ? app : {...app,title,menus};
}

// Hiding is a final rendering projection. Routing keeps the complete metadata
// so hiding a menu never closes its open window or disables an LLM resource.
export function visibleWorkspaceMenus(app, preferences={}) {
 const hidden=new Set(preferences.hiddenMenus || []);
 const visit=(items,parents=[]) => (items || []).flatMap(item=>{
  const path=[...parents,item.id];
  if(hidden.has(workspaceMenuKey(app,path))) return [];
  if(!item.children?.length)return [item];
  const children=visit(item.children,path);
  return children.length || item.action ? [{...item,children}] : [];
 });
 return visit(app?.menus);
}


// This is only the final navigation projection. It does not modify catalog
// metadata, provider configuration, datasource scope or LLM discovery.
export function personalWorkspaceLayout(layout, preferences = {}) {
  if (!layout) return layout;
  const applications = (layout.applications || []).filter(app => app.id !== PERSONAL_WORKSPACE_ID).map(app=>navigationLabels(app,preferences));
  if (preferences.enabled !== true && !(preferences.groups || []).length) return {...layout, applications};
  const titles = workspaceResourceTitles({applications});
  const menus = (preferences.groups || []).filter(group => group.items?.length).map(group => ({id:group.id, title:group.label, icon:'lucide:folder', children:group.items.map(item => ({
    id:item.id, title:item.alias || titles.get(JSON.stringify([item.provider, item.uri])) || `Pinned ${item.kind}`,
    icon:item.kind === 'window' ? 'lucide:panel-right' : 'lucide:chart-no-axes-combined',
    action:{type:item.kind, resourceUri:item.uri, providerIdentity:item.provider, parameters:item.parameters || {}},
  }))}));
  return {...layout, applications:[...applications, {id:PERSONAL_WORKSPACE_ID, title:preferences.title || 'My workspace', icon:'lucide:folder-open', menus}]};
}
export function visibleWorkspaceApplications(layout, preferences = {}) {
  const hidden = new Set(preferences.hiddenApplications || []);
  return (layout?.applications || []).filter(app => app.id === PERSONAL_WORKSPACE_ID ? preferences.enabled === true : !hidden.has(workspaceApplicationKey(app)));
}

export function workspaceResourceTitles(layout) {
 const titles = new Map();
 const visit = (items, provider) => {for (const item of items || []) {if (item.action?.resourceUri) titles.set(JSON.stringify([item.action.providerIdentity || provider || '', item.action.resourceUri]), item.title);visit(item.children,provider);}};
 (layout?.applications || []).forEach(app=>visit(app.menus,app.providerIdentity));
 return titles;
}

export const WORKSPACE_PREFERENCE_EVENT = 'agently:workspace-preferences-changed';
export const WORKSPACE_PREFERENCE_STORAGE_KEY = 'agently.workspacePreferences.changed';
export function isWorkspacePreferenceChange(event) {
 return event?.type === 'tool_call_completed' && event?.toolName === 'my_workspace:update';
}
export function notifyWorkspacePreferenceChange(detail) {
 if (typeof window === 'undefined') return;
 window.dispatchEvent(new CustomEvent(WORKSPACE_PREFERENCE_EVENT,{detail}));
 // Local visibility already emits its scoped storage event across tabs.
 if(detail?.localVisibility)return;
 // Cross-tab invalidation for server-owned preferences.
 try {window.localStorage?.setItem(WORKSPACE_PREFERENCE_STORAGE_KEY, `${Date.now()}:${Math.random()}`);} catch (_) {}
}

// Presentation overrides share the existing workspace preference events. They
// never change the authorized layout or the resource-discovery catalog.
export const workspaceVisibilityScope = (workspaceId, principal) => {
 const owner=principal?.subject || principal?.username || principal?.id;
 return owner && typeof workspaceId === 'string' && workspaceId.trim() ? JSON.stringify([workspaceId,owner]) : '';
};
export const visibilityKey=scope=>`agently.workspaceVisibility.v1:${scope}`;
export function browserWorkspaceVisibility(scope, preferences={}) {
 if(!scope || typeof window==='undefined')return preferences;
 let stored;
 try{stored=JSON.parse(window.localStorage.getItem(visibilityKey(scope)) || '{}');}catch{return preferences;}
 const project=(field,overrides)=>{
  const hidden=new Set(preferences[field] || []);
  if(overrides && typeof overrides==='object' && !Array.isArray(overrides))for(const [id,value] of Object.entries(overrides)){
   if(value===true)hidden.add(id);else if(value===false)hidden.delete(id);
  }
  return [...hidden];
 };
 return {...preferences,hiddenApplications:project('hiddenApplications',stored?.applications),hiddenMenus:project('hiddenMenus',stored?.menus)};
}
export function saveBrowserWorkspaceVisibility(scope, action) {
 if(!scope)throw new Error('Sign in before changing workspace visibility.');
 const field=action.action==='set_application_visibility'?'applications':action.action==='set_menu_visibility'?'menus':null;
 const id=field==='applications'?action.applicationId:action.menuId;
 if(!field || typeof id!=='string' || !id || typeof action.hidden!=='boolean')throw new Error('Invalid workspace visibility change.');
 const key=visibilityKey(scope);
 let previous={};
 try{previous=JSON.parse(window.localStorage.getItem(key) || '{}');}catch{}
 const values=previous?.[field];
 const next={...previous,[field]:{...(values && typeof values==='object'&&!Array.isArray(values)?values:{}),[id]:action.hidden}};
 window.localStorage.setItem(key,JSON.stringify(next));
 notifyWorkspacePreferenceChange({scope,localVisibility:true});
}
