// @vitest-environment jsdom
import {beforeEach,expect,it,vi} from 'vitest';

const {openWindow,removeWindow,windows}=vi.hoisted(()=>({openWindow:vi.fn(),removeWindow:vi.fn(),windows:vi.fn(()=>[])}));
vi.mock('../components/menuBarHelpers.js',()=>({openWindow}));
vi.mock('forge/core',()=>({removeWindow,activeWindows:{peek:windows}}));
import {createPublishedManagementHostWindow,downloadPublishedManagementFile,
  confirmClosePublishedManagementWindow,clearPublishedManagementWindowDirty} from './publishedManagementHostWindow.js';

const source={uri:'window://example/catalog',providerIdentity:'example-owner'};
const routes={detail:{resourceUri:'window://example/details',title:'Record details'}};
const current={windowId:'catalog-one',resourceRef:{uri:source.uri},providerIdentity:source.providerIdentity,
  parameters:{contextId:3},conversationId:'chat-one',parentKey:'root',presentation:'hosted',region:'app.section'};
beforeEach(()=>{vi.clearAllMocks();clearPublishedManagementWindowDirty(current.windowId);});

it('opens the declared destination under the admitted publisher and placement',()=>{
  const host=createPublishedManagementHostWindow(current,{source,routes});
  expect(host.navigate('detail',{recordId:7,discardConfirmed:true})).toBe(true);
  expect(openWindow).toHaveBeenCalledWith(routes.detail.resourceUri,routes.detail.title,[],{
    providerIdentity:source.providerIdentity,resourceRef:{uri:routes.detail.resourceUri},parameters:{contextId:3,recordId:7},
    conversationId:'chat-one',parentKey:current.windowId,presentation:'hosted',region:'app.section'});
});

it('returns a section drill to its exact parent without copying the child filters to it',()=>{
  const target={resourceUri:source.uri,title:'Catalog'};
  const childSource={uri:routes.detail.resourceUri,providerIdentity:source.providerIdentity};
  const child={...current,windowId:'detail-one',parentKey:current.windowId,resourceRef:{uri:childSource.uri},parameters:{recordId:7}};
  windows.mockReturnValueOnce([current,child]);
  const host=createPublishedManagementHostWindow(child,{source:childSource,routes:{back:target}});
  host.navigate('back',{discardConfirmed:true});
  expect(openWindow).toHaveBeenCalledWith(source.uri,'Catalog',[],expect.objectContaining({
    providerIdentity:source.providerIdentity,parentKey:current.parentKey,parameters:current.parameters}));
});

it('does not supply navigation from window parameters, a different publisher or a different source',()=>{
  expect(createPublishedManagementHostWindow({...current,parameters:{routes}})).toBeNull();
  expect(createPublishedManagementHostWindow({...current,providerIdentity:'other'},{source,routes})).toBeNull();
  expect(createPublishedManagementHostWindow({...current,resourceRef:{uri:'window://elsewhere/catalog'}},{source,routes})).toBeNull();
});

it('rejects missing, inherited, foreign and noncanonical destinations before opening any window',()=>{
  const host=createPublishedManagementHostWindow(current,{source,routes:{...routes,
    foreign:{resourceUri:'window://another/details',title:'Details'},
    owner:{...routes.detail,providerIdentity:'other'},
    traversal:{resourceUri:'window://example/../another',title:'Outside'},
    url:{resourceUri:'https://example.test',title:'Outside'}}});
  for(const key of ['unlisted','constructor','__proto__','window://example/details','foreign','owner','traversal','url']){
    expect(()=>host.navigate(key)).toThrow(/Unsupported/);
  }
  expect(openWindow).not.toHaveBeenCalled();
});

it('tracks unsaved state per window and blocks close until the user confirms',()=>{
  const host=createPublishedManagementHostWindow(current,{source,routes:{}});
  const confirm=vi.spyOn(window,'confirm').mockReturnValue(false);
  host.setDirty(true);
  expect(confirmClosePublishedManagementWindow(current.windowId)).toBe(false);
  expect(host.close()).toBe(false);
  expect(removeWindow).not.toHaveBeenCalled();
  confirm.mockReturnValue(true);
  expect(host.close()).toBe(true);
  expect(removeWindow).toHaveBeenCalledWith(current.windowId);
  confirm.mockRestore();
});

it('downloads only validated producer-returned bytes through a local Blob URL',()=>{
  URL.createObjectURL=vi.fn(()=> 'blob:local');
  URL.revokeObjectURL=vi.fn();
  const click=vi.spyOn(HTMLAnchorElement.prototype,'click').mockImplementation(function(){expect(this.download).toBe('report.zip');expect(this.href).toBe('blob:local');});
  downloadPublishedManagementFile({archive:[80,75],filename:'../report.zip',mediaType:'application/zip'});
  expect(URL.createObjectURL).toHaveBeenCalledOnce();
  expect(click).toHaveBeenCalledOnce();
  expect(()=>downloadPublishedManagementFile({archive:'not base64?'})).toThrow(/not valid base64/);
  click.mockRestore();
});
