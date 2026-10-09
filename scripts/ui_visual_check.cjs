const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'../web/node_modules/playwright');
const fs=require('node:fs'),path=require('node:path');
(async()=>{
 const browser=await chromium.launch({headless:true});
 const page=await browser.newPage({viewport:{width:1440,height:1000},colorScheme:'light',reducedMotion:'reduce',locale:'zh-CN',timezoneId:'Asia/Shanghai'});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const out='artifacts/ui-refresh';fs.mkdirSync(out,{recursive:true});
 // Render the built assets against a read-only live API. No settings are saved.
 await page.route('**/*',async route=>{
  const url=new URL(route.request().url());
  if(url.pathname.startsWith('/api/')){
   if(route.request().method()!=='GET')throw new Error('Visual verification must not mutate the live service');
   return route.continue();
  }
  const file=url.pathname==='/'?'web/dist/index.html':'web/dist'+url.pathname;
  if(fs.existsSync(file)&&fs.statSync(file).isFile())return route.fulfill({path:file,contentType:file.endsWith('.css')?'text/css':file.endsWith('.js')?'application/javascript':'text/html'});
  return route.continue();
 });
 await page.goto('http://127.0.0.1:1055');await page.getByRole('heading',{name:'仪表盘',exact:true}).waitFor();
 const checks=[];
 async function checkControls(name,selector){const metrics=await page.locator(selector).evaluateAll(elements=>elements.map(e=>({tag:e.tagName,height:e.getBoundingClientRect().height,font:getComputedStyle(e).fontSize})));if(!metrics.length||metrics.some(m=>Math.abs(m.height-42)>.5||m.font!=='14px'))throw new Error(name+': inconsistent control geometry '+JSON.stringify(metrics));checks.push({page:name,controls:metrics});}
 async function shot(name){await page.screenshot({path:out+'/'+name+'.png',fullPage:!name.startsWith('query-details')});}
 const go=async label=>{await page.getByRole('navigation').getByRole('button',{name:label,exact:true}).click();await page.getByRole('heading',{name:label,exact:true}).waitFor();};
 await checkControls('dashboard','.top-actions select,.top-actions button');await shot('dashboard-light');
 await go('查询记录');await checkControls('queries','.filters input,.filters select');await shot('queries-light');
 const first=page.locator('tbody tr.clickable').first();if(await first.count()){
  await first.click();await page.getByRole('dialog',{name:'查询详情'}).waitFor();
  const sizes=await page.locator('.modal h2,.modal h3,.modal h4').evaluateAll(es=>es.map(e=>({tag:e.tagName,size:parseFloat(getComputedStyle(e).fontSize)})));
  if(sizes.some(e=>e.tag==='H2'&&e.size!==19||e.tag==='H3'&&e.size!==15||e.tag==='H4'&&e.size!==13))throw new Error('Incorrect dialog type hierarchy');
  if(await page.locator('.modal >section').evaluate(e=>e.scrollWidth>e.clientWidth))throw new Error('Nested DNS panel overflows dialog');
  checks.push({page:'query-details',headings:sizes});await shot('query-details-light');await page.getByRole('button',{name:'关闭',exact:true}).click();
 }
 await go('规则测试');await checkControls('rule-test','.rule-form input,.rule-form select,.rule-form button');
 const padding=await page.locator('.rule-form').evaluate(e=>{const s=getComputedStyle(e);const r=e.getBoundingClientRect();const children=[...e.children].map(x=>x.getBoundingClientRect());return {padding:[s.paddingTop,s.paddingRight,s.paddingBottom,s.paddingLeft],minimumClearance:Math.min(...children.flatMap(x=>[x.left-r.left,r.right-x.right,x.top-r.top,r.bottom-x.bottom]))}});
 if(padding.padding.some(p=>p!=='24px')||padding.minimumClearance<24)throw new Error('Rule form content touches its border');checks.push({page:'rule-test-insets',...padding});await shot('rule-test-light');
 await page.setViewportSize({width:390,height:844});
 const mobilePadding=await page.locator('.rule-form').evaluate(e=>{const s=getComputedStyle(e);return [s.paddingTop,s.paddingRight,s.paddingBottom,s.paddingLeft]});
 if(mobilePadding.some(p=>p!=='18px'))throw new Error('Mobile rule form has inconsistent padding');await shot('rule-test-mobile');
 await page.setViewportSize({width:1440,height:1000});
 await go('设置');for(const category of ['通用','DNS','Surge','BGP','安全']){
  await page.locator('.tabs').getByRole('button',{name:category,exact:true}).click();
  await checkControls('settings-'+category,'.settings-form input:not([type=checkbox]):not([type=file]),.settings-form select');
  const checkboxes=await page.locator('.settings-form input[type=checkbox]').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().height));if(checkboxes.some(h=>h!==18))throw new Error('Incorrect checkbox size');
  await shot('settings-'+category+'-light');
 }
 await page.evaluate(()=>document.documentElement.dataset.theme='dark');await shot('settings-dark');
 await page.evaluate(()=>document.documentElement.dataset.theme='light');
 await go('仪表盘');await page.setViewportSize({width:1120,height:780});await shot('dashboard-app-size');
 await page.setViewportSize({width:390,height:844});await shot('dashboard-mobile');
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('Mobile layout overflows');
 await go('设置');await page.locator('.tabs').getByRole('button',{name:'Surge',exact:true}).click();await checkControls('mobile-settings','.settings-form input:not([type=checkbox]):not([type=file]),.settings-form select');await shot('settings-mobile');
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('Mobile settings overflow');
 if(errors.length)throw new Error(errors.join('\n'));
 fs.writeFileSync(out+'/verification.json',JSON.stringify({passed:true,checks,browser_errors:errors,live_configuration_changed:false},null,2));
 console.log(JSON.stringify({passed:true,checks:checks.length,screenshots:out}));
 await browser.close();
})().catch(e=>{console.error(e);process.exitCode=1});
