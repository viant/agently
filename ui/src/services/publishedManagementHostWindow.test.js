// @vitest-environment jsdom
import {beforeEach,expect,it,vi} from 'vitest';

const {openWindow,removeWindow}=vi.hoisted(()=>({openWindow:vi.fn(),removeWindow:vi.fn()}));
vi.mock('../components/menuBarHelpers.js',()=>({openWindow}));
vi.mock('forge/core',()=>({removeWindow}));
import {createPublishedManagementHostWindow,downloadPublishedManagementFile,
  confirmClosePublishedManagementWindow,clearPublishedManagementWindowDirty} from './publishedManagementHostWindow.js';

const current={windowId:'catalog-one',resourceRef:{uri:'window://datastudio/component-catalog'},providerIdentity:'datly-owner',
  parameters:{currentNamespace:{name:'demo'}},conversationId:'conv',parentKey:'chat/new',presentation:'hosted',region:'chat.top'};
beforeEach(()=>{openWindow.mockReset();removeWindow.mockReset();clearPublishedManagementWindowDirty(current.windowId);document.body.innerHTML='';});

it('opens only linked management sections with the same provider and a newly admitted root',()=>{
  const host=createPublishedManagementHostWindow(current);
  expect(host.navigate('builder',{report:{id:'report-7'}})).toBe(true);
  expect(openWindow).toHaveBeenCalledWith('window://datastudio/reader-workbench','Reader Builder',[],{
    providerIdentity:'datly-owner',resourceRef:{uri:'window://datastudio/reader-workbench'},
    parameters:{currentNamespace:{name:'demo'},report:{id:'report-7'}},conversationId:'conv',parentKey:'chat/new',
    presentation:'hosted',region:'chat.top',
  });
  host.navigate('components',{discardConfirmed:true});
  expect(openWindow.mock.calls.at(-1)[0]).toBe('window://datastudio/component-catalog');
  expect(openWindow.mock.calls.at(-1)[3].parameters).not.toHaveProperty('discardConfirmed');
  expect(()=>host.navigate('window://foreign/target')).toThrow(/Unsupported/);
  expect(createPublishedManagementHostWindow({...current,resourceRef:{uri:'window://other/component-catalog'}})).toBeNull();
});

it('tracks unsaved state per window and blocks close until the user confirms',()=>{
  const host=createPublishedManagementHostWindow(current);
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
