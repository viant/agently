import {describe,it,expect} from 'vitest';
import {workspaceToolbarItem} from '../../../../forge/src/core/context/WorkspacePresentation.jsx';

describe('workspace action icons',()=>{
 it('uses icon-only filter/refresh/export actions while preserving accessible names',()=>{
  for(const item of [{id:'filterList',label:'Filters'},{id:'refresh',label:'Refresh'},{id:'export',type:'tableExport',label:'Export'}]){
   const result=workspaceToolbarItem(item,{label:'Report'});
   expect(result.hideLabel).toBe(true);
   expect(result.icon).toBeTruthy();
   expect(result.ariaLabel).toBe(item.label);
   expect(result.tooltip).toBe(item.label);
  }
 });
 it('keeps unrelated and non-workspace labels unchanged',()=>{
  const save={id:'save',label:'Save'};const filter={id:'filterList',label:'Filters'};
  expect(workspaceToolbarItem(save,{})).toBe(save);
  expect(workspaceToolbarItem(filter,null)).toBe(filter);
 });
});
