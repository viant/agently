import assert from 'node:assert/strict';
import {createServer} from 'vite';
import {chromium} from 'playwright';
const server=await createServer({mode:'test',optimizeDeps:{entries:['report-window-scroll-proof.html']},server:{host:'127.0.0.1',port:0}});
await server.listen();
const browser=await chromium.launch({headless:true});
try {
 for(const viewport of [{width:1440,height:900},{width:390,height:844}])for(const {kind,placement} of [...['top','left','right'].map(placement=>({kind:'report',placement})),{kind:'window-local',placement:'top'},{kind:'window-mcp',placement:'top'}]) {
  const page=await browser.newPage({viewport});
  await page.goto(`http://127.0.0.1:${server.httpServer.address().port}/report-window-scroll-proof.html?placement=${placement}&kind=${kind}`);
  if(kind==='report')await page.getByRole('button',{name:'Apply filters'}).click();
  const last=page.getByRole('heading',{name:'Last report section',exact:true});
  await last.waitFor({state:'attached'});
  await page.getByRole('heading',{name:kind==='report'?'Example report':'Section 1',exact:true}).hover();
  await page.mouse.wheel(0,100000);
  await page.waitForFunction(()=>{
   const last=[...document.querySelectorAll('h1,h2,h3,h4,h5,h6')].find(node=>node.textContent==='Last report section');
   const box=last?.getBoundingClientRect();return box&&box.top>=0&&box.bottom<=innerHeight;
  },{},{timeout:5000});
  const box=await last.boundingBox();
  assert.ok(box&&box.y>=0&&box.y+box.height<=viewport.height);
  console.log(JSON.stringify({viewport,kind,placement,lastSectionReachable:true}));
  await page.close();
 }
} finally {await browser.close();await server.close();}
