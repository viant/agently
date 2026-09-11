import React from 'react';
import {createRoot} from 'react-dom/client';
import '@blueprintjs/core/lib/css/blueprint.css';
import '@blueprintjs/icons/lib/css/blueprint-icons.css';
window.global ||= window;
window.process ||= {env: {}};
async function boot(){
 await import('forge-runtime');
 const {default: Preview}=await import('./window/WindowPreview.jsx');
 createRoot(document.getElementById('root')).render(<Preview/>);
}
boot().catch(error=>{document.getElementById('root').textContent=error.message});
