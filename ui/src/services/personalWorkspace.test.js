import {expect,test} from 'vitest';
import {personalWorkspaceLayout,visibleWorkspaceApplications,workspaceApplicationKey,isWorkspacePreferenceChange,workspaceShowsTiles,workspaceShowsMenu,visibleWorkspaceMenus,workspaceMenuKey} from './personalWorkspace';
import {layoutWindowDestinations} from '../components/layoutNavigation';
const source={applications:[{id:'published',title:'Published',providerIdentity:'example',menus:[{id:'orders',title:'Orders',action:{type:'window',resourceUri:'window://example/orders'}}]}]};
test('projects exact window and report pins without changing published discovery',()=>{
 const original=JSON.stringify(source), prefs={enabled:true,hiddenApplications:[workspaceApplicationKey(source.applications[0])],groups:[{id:'weekly',label:'Weekly',items:[{id:'window',kind:'window',provider:'example',uri:'window://example/orders'},{id:'report',kind:'report',provider:'example',uri:'report://example/overview',alias:'Original overview',parameters:{period:'month'}}]}]};
 const projection=personalWorkspaceLayout(source,prefs);
 expect(visibleWorkspaceApplications(projection,prefs).map(app=>app.title)).toEqual(['My workspace']);
 expect(layoutWindowDestinations(projection).find(item=>item.id==='window').action).toMatchObject({type:'window',resourceUri:'window://example/orders'});
 expect(layoutWindowDestinations(projection).find(item=>item.id==='report').action).toMatchObject({type:'report',resourceUri:'report://example/overview',parameters:{period:'month'}});
 expect(JSON.stringify(source)).toBe(original);expect(layoutWindowDestinations(source)).toHaveLength(1);
 expect(projection.applications[1].menus[0].children[0].title).toBe('Orders');
});
test('explicitly disabled personal menu preserves ordinary applications',()=>{
 const prefs={enabled:false,groups:[{id:'one',items:[{id:'pin',kind:'report',provider:'example',uri:'report://example/overview'}]}]};
 const projected=personalWorkspaceLayout(source,prefs);
 expect(visibleWorkspaceApplications(projected,prefs)).toEqual(source.applications);
 expect(layoutWindowDestinations(projected)).toHaveLength(2);
});

test('only successful native workspace mutations invalidate navigation preferences',()=>{
 expect(isWorkspacePreferenceChange({type:'tool_call_completed',toolName:'my_workspace:update'})).toBe(true);
 for (const event of [{type:'tool_call_failed',toolName:'my_workspace:update'},{type:'tool_call_completed',toolName:'my_workspace:get'},{type:'tool_call_completed',toolName:'example:search'}]) expect(isWorkspacePreferenceChange(event)).toBe(false);
});

test('explicit enable shows an empty personal workspace',()=>{
 const prefs={enabled:true};const projected=personalWorkspaceLayout(source,prefs);
 expect(visibleWorkspaceApplications(projected,prefs).map(app=>app.title)).toEqual(['Published','My workspace']);
 expect(projected.applications[1].menus).toEqual([]);
});

test('presentation changes keep exact launch metadata intact',()=>{
 for(const presentation of ['menu','tiles','both']) {
  const preferences={enabled:true,presentation,groups:[{id:'weekly',label:'Weekly',items:[{id:'pin',kind:'report',provider:'example',uri:'report://example/overview',parameters:{period:'month'}}]}]};
  expect(workspaceShowsTiles(preferences)).toBe(presentation!=='menu');
  expect(workspaceShowsMenu(preferences)).toBe(presentation!=='tiles');
  const projected=personalWorkspaceLayout(source,preferences);
  expect(layoutWindowDestinations(projected).find(item=>item.id==='pin').action).toEqual({type:'report',resourceUri:'report://example/overview',providerIdentity:'example',parameters:{period:'month'}});
 }
 expect(workspaceShowsMenu({})).toBe(true);
 expect(workspaceShowsTiles({})).toBe(false);
});

test('personal labels and hidden menus preserve authoritative metadata and exact routes',()=>{
 const original=JSON.stringify(source),app=source.applications[0];
 const appKey=workspaceApplicationKey(app),menuKey=workspaceMenuKey(app,['orders']);
 const preferences={enabled:true,title:'My desk',applicationLabels:{[appKey]:'My application'},menuLabels:{[menuKey]:'My orders'},hiddenMenus:[menuKey]};
 const projected=personalWorkspaceLayout(source,preferences);
 expect(projected.applications[0].title).toBe('My application');
 expect(projected.applications[0].menus[0].title).toBe('My orders');
 expect(projected.applications[1].title).toBe('My desk');
 expect(visibleWorkspaceMenus(projected.applications[0],preferences)).toEqual([]);
 expect(layoutWindowDestinations(projected)[0].action.resourceUri).toBe('window://example/orders');
 expect(JSON.stringify(source)).toBe(original);
});

test('browser visibility is isolated by user and workspace and leaves all routes available',async()=>{
 const {browserWorkspaceVisibility,saveBrowserWorkspaceVisibility,workspaceVisibilityScope}=await import('./personalWorkspace');
 const entries=new Map();
 const previous=globalThis.window;
 globalThis.window={localStorage:{getItem:key=>entries.get(key),setItem:(key,value)=>entries.set(key,value)},dispatchEvent:()=>{}};
 try{
  const alice=workspaceVisibilityScope('one',{subject:'alice'}),bob=workspaceVisibilityScope('one',{subject:'bob'}),other=workspaceVisibilityScope('two',{subject:'alice'});
  const id=workspaceApplicationKey(source.applications[0]);
  saveBrowserWorkspaceVisibility(alice,{action:'set_application_visibility',applicationId:id,hidden:true});
  expect(visibleWorkspaceApplications(source,browserWorkspaceVisibility(alice))).toEqual([]);
  expect(visibleWorkspaceApplications(source,browserWorkspaceVisibility(bob))).toEqual(source.applications);
  expect(visibleWorkspaceApplications(source,browserWorkspaceVisibility(other))).toEqual(source.applications);
  expect(layoutWindowDestinations(source)).toHaveLength(1);
  saveBrowserWorkspaceVisibility(alice,{action:'set_application_visibility',applicationId:id,hidden:false});
  expect(visibleWorkspaceApplications(source,browserWorkspaceVisibility(alice,{hiddenApplications:[id]}))).toEqual(source.applications);
 }finally{globalThis.window=previous;}
});

test('an actionable parent stays available when its children are hidden',()=>{
 const app={id:'app',menus:[{id:'parent',title:'Parent',action:{type:'window',resourceUri:'window://example/parent'},children:[{id:'child',title:'Child'}]}]};
 const menus=visibleWorkspaceMenus(app,{hiddenMenus:[workspaceMenuKey(app,['parent','child'])]});
 expect(menus).toHaveLength(1);expect(menus[0].children).toEqual([]);expect(menus[0].action.resourceUri).toBe('window://example/parent');
});

test('workspace theme defaults precede personal labels and stay out of other themes',()=>{
 const key=workspaceMenuKey(source.applications[0],['orders']);
 const layout={...source,left:{navigation:{labelsByTheme:{'design-system-2':{[key]:'Records'}}}}};
 expect(personalWorkspaceLayout(layout,{},'design-system-2').applications[0].menus[0].title).toBe('Records');
 expect(personalWorkspaceLayout(layout,{},'baseline').applications[0].menus[0].title).toBe('Orders');
 expect(personalWorkspaceLayout(layout,{menuLabels:{[key]:'My records'}},'design-system-2').applications[0].menus[0].title).toBe('My records');
 expect(layout.applications[0].menus[0].title).toBe('Orders');
});
