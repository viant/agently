import assert from 'node:assert/strict';
import http from 'node:http';
import {chromium} from 'playwright';

let retired=false;
let chunkRequests=0;
const server=http.createServer((req,res)=>{
  if(req.url==='/assets/content-version.js'){
    chunkRequests++;
    if(retired){res.writeHead(404);res.end('Missing old asset');return;}
    res.writeHead(200,{'Content-Type':'text/javascript','Cache-Control':'public, max-age=31536000, immutable'});
    res.end('export const value="Loaded content";');return;
  }
  res.writeHead(200,{'Content-Type':'text/html'});
  res.end(`<!doctype html><html><head><link rel="modulepreload" href="/assets/content-version.js"></head><body><input id="draft" value="Unsaved message"><button id="open">Open</button><div id="result"></div><script type="module">document.querySelector('#open').onclick=async()=>{try{const module=await import('/assets/content-version.js');document.querySelector('#result').textContent=module.value;}catch(error){document.querySelector('#result').textContent='FAILED';}};</script></body></html>`);
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const browser=await chromium.launch({headless:true,channel:process.env.PLAYWRIGHT_CHANNEL||'chromium'});
try{
 const page=await browser.newPage();
 const loaded=page.waitForEvent('requestfinished',{predicate:r=>r.url().endsWith('/assets/content-version.js')});
 await page.goto(`http://127.0.0.1:${server.address().port}/`);
 await loaded;
 retired=true;
 await page.getByRole('button',{name:'Open'}).click();
 await page.waitForFunction(()=>document.querySelector('#result').textContent==='Loaded content');
 assert.equal(chunkRequests,1,'an open tab must not request the retired chunk again');
 assert.equal(await page.locator('#draft').inputValue(),'Unsaved message');
 console.log('deployment rollover preload proof passed: one chunk request, draft retained');
}finally{
 await browser.close();
 await new Promise(resolve=>server.close(resolve));
}
