import { describe, expect, it } from 'vitest';
import {activeWindows,selectedWindowId} from 'forge/core';
import { canonicalWindowParameters, openWindow } from './menuBarHelpers.js';

describe('workspace window parameter identity', () => {
  it('ignores object key order while preserving values and array order', () => {
    expect(canonicalWindowParameters({ b: 2, a: { y: 1, x: [1, 2] } }))
      .toBe(canonicalWindowParameters({ a: { x: [1, 2], y: 1 }, b: 2 }));
    expect(canonicalWindowParameters({ id: 1 })).not.toBe(canonicalWindowParameters({ id: '1' }));
    expect(canonicalWindowParameters({ ids: [1, 2] })).not.toBe(canonicalWindowParameters({ ids: [2, 1] }));
  });
});

it('restores the selected recovery child only under its exact shortcut',()=>{
  const uri='report://example/summary';
  const options={region:'app.section',presentation:'hosted',resourceRef:{uri},parameters:{period:'weekly'},restoreSelection:true};
  const root={windowId:'root',windowKey:uri,...options};
  const child={...root,windowId:'reopened',parentKey:root.windowId,parameters:{period:'weekly',executeOnOpen:false}};
  const other={...root,windowId:'other',parameters:{period:'monthly'}};
  const prior=activeWindows.peek(),selected=selectedWindowId.peek();
  try{
    activeWindows.value=[other,root,child];selectedWindowId.value=child.windowId;
    expect(openWindow(uri,'Summary',[],options).windowId).toBe(root.windowId);
    expect(selectedWindowId.peek()).toBe(child.windowId);
    expect(openWindow(uri,'Summary',[],{...options,parameters:{period:'monthly'}}).windowId).toBe(other.windowId);
  }finally{activeWindows.value=prior;selectedWindowId.value=selected;}
});
