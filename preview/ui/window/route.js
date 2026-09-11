export function readRoute(search, fallback='') {
 const p=new URLSearchParams(search);
 const parameters=JSON.parse(p.get('parameters')||'{}');
 if(!parameters||typeof parameters!=='object'||Array.isArray(parameters))throw new Error('parameters must be a JSON object');
 return {windowKey:p.get('window')||fallback,parameters,variant:p.get('variant')||'default',query:p.has('query')?JSON.parse(p.get('query')):null};
}
export function windowURL(location, entry) {
 const url=new URL(location);
 const previous=url.searchParams.get('window');
 url.searchParams.set('window',entry.windowKey);
 const parameters=entry.parameters||{};
 if(Object.keys(parameters).length)url.searchParams.set('parameters',JSON.stringify(parameters));else url.searchParams.delete('parameters');
 if(previous!==entry.windowKey)url.searchParams.delete('query');
 return url.pathname+url.search;
}
