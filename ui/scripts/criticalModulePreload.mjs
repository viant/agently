// Preload the shared conversation/report renderer while this page's immutable
// assets are current. Its execution remains lazy; the browser retains the module
// even if a later deployment replaces the server's hashed asset directory.
export function criticalModulePreload() {
  let base='/';
  return {
    name:'critical-module-preload',
    apply:'build',
    configResolved(config){base=config.base;},
    transformIndexHtml:{order:'post',handler(_html,context){
      if(!context.bundle || !context.filename?.endsWith('index.html'))return [];
      return Object.values(context.bundle).filter(chunk=>chunk.type==='chunk' && !chunk.isEntry && String(chunk.facadeModuleId||'').replaceAll('\\','/').endsWith('/src/components/chat/RichContent.jsx'))
        .map(chunk=>({tag:'link',attrs:{rel:'modulepreload',crossorigin:'',href:base+chunk.fileName},injectTo:'head'}));
    }},
  };
}
