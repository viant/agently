import React from 'react';
import {createRoot} from 'react-dom/client';
import {Classes,OverlayToaster,Position} from '@blueprintjs/core';
import {Icon} from 'forge/components';

// The host keeps one native Blueprint owner under its existing theme boundary.
// Timing, hover/focus pause, dismissal and announcements remain native Toast work.
const owners=new WeakMap();
const icons={danger:'lucide:circle-alert',warning:'lucide:triangle-alert',success:'lucide:circle-check',primary:'lucide:info'};

export async function showDesignSystemToast(container,message,{intent='danger',ttlMs=4000}={}){
  if(!owners.has(container))owners.set(container,OverlayToaster.create({position:Position.BOTTOM_RIGHT,autoFocus:false},{
    container:container.ownerDocument.body,
    // Use the host's existing Blueprint portal layer above workspace drawers.
    // The application theme on <html> supplies its live semantic variables.
    domRenderer:(element,root)=>{root.classList.add(Classes.PORTAL);createRoot(root).render(element);},
  }));
  const toaster=await owners.get(container);
  const tone=intent==='info'?'primary':intent;
  const key=toaster.show({message,intent:tone,icon:<Icon className="bp6-icon" name={icons[tone]||icons.primary}/>,
    timeout:ttlMs,isCloseButtonShown:false});
  return ()=>toaster.dismiss(key);
}
