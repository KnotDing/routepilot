// Inspect built UI against deterministic API fixtures, without a live backend.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs');
const source=fs.readFileSync('web/src/full-i18n.test.jsx','utf8');
const fixture=Function(source.slice(source.indexOf('const settings='),source.indexOf('function noUntranslatedText'))+';return fixture;')();
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
 const page=await browser.newPage({viewport:{width:1440,height:1000},locale:'zh-CN',colorScheme:'light',reducedMotion:'reduce'});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const out='artifacts/ui-forms';fs.mkdirSync(out,{recursive:true});
 await page.route('**/*',async route=>{
  const url=new URL(route.request().url());
  if(url.pathname==='/api/v1/events/stream')return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
  if(url.pathname.startsWith('/api/v1/')){
   const path=url.pathname.slice(8)+url.search;
   let value=fixture(path);
   if(path==='management/backends')value=[];
   if(path==='settings')value={...value,cache:{...value.cache,optimistic:false,optimistic_responses:false},notifications:{enabled:true,surge:true,bgp:true,dns:true,controller:true,recovery:true,cooldown_seconds:300,webhook_url:'',webhook_configured:false,errors:[]}};
   if(path==='status'){value.software={...value.software,...JSON.parse(fs.readFileSync('internal/buildinfo/release.json'))};}
   if(path==='surge/status'){value.snapshot.ipv6_state='disabled';value.snapshot.always_real_ip_count=61;}
   return route.fulfill({contentType:'application/json',body:JSON.stringify(value)});
  }
  const file=url.pathname==='/'?'web/dist/index.html':'web/dist'+url.pathname;
  if(fs.existsSync(file))return route.fulfill({path:file,contentType:file.endsWith('.css')?'text/css':file.endsWith('.js')?'application/javascript':'text/html'});
  return route.abort();
 });
 await page.goto('http://127.0.0.1:18917');
 await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();
 async function go(label){
  if(await page.getByRole('button',{name:'打开导航',exact:true}).isVisible()){
   await page.getByRole('button',{name:'打开导航',exact:true}).click();
   await page.getByRole('dialog',{name:'主导航',exact:true}).getByRole('button',{name:label,exact:true}).click();
  }else await page.locator('.desktop-sidebar nav').getByRole('button',{name:label,exact:true}).click();
  await page.getByRole('heading',{level:1,name:label,exact:true}).waitFor();
  await page.locator('.desktop-sidebar nav').getByRole('button',{name:label,exact:true,includeHidden:true}).and(page.locator('[aria-current=page]')).waitFor({state:'attached'});
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
 }
 async function check(name){
  const overflow=await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1);
  if(overflow)throw Error(name+' overflows the viewport');
 }
 const pages=['仪表盘','DNS 概览','查询记录','决策缓存','DNS 拦截','本地 DNS','域名覆盖','规则测试','路由后端','BGP 概览','BGP 邻居','手动路由','健康状态','事件','设置'];
 const categories=['通用','DNS','Surge','BGP','缓存','健康','日志与隐私','安全','软件更新','通知','关于'];
 for(const width of [1440,1120,390]){
  await page.setViewportSize({width,height:1000});
  for(const label of pages){await go(label);await check(width+' '+label);}
  for(const label of categories){await page.locator('.tabs').getByRole('button',{name:label,exact:true}).click();await check(width+' '+label);}
  await go('DNS 概览');await page.screenshot({path:out+'/dns-'+width+'.png',fullPage:true});
  await go('设置');await page.locator('.tabs').getByRole('button',{name:'DNS',exact:true}).click();await page.screenshot({path:out+'/dns-settings-'+width+'.png',fullPage:true});
  await page.locator('.tabs').getByRole('button',{name:'安全',exact:true}).click();await page.getByLabel('启用管理认证',{exact:true}).uncheck();await page.getByLabel('启用管理认证',{exact:true}).check();
  if(!(await page.getByLabel('新密码（至少 12 字节）',{exact:true}).getAttribute('required')!==null))throw Error('Initial password requirement did not update');
  await page.screenshot({path:out+'/security-'+width+'.png',fullPage:true});
  await go('手动路由');await page.getByRole('button',{name:'＋ 添加',exact:true}).click();
  const required=await page.locator('.modal label[data-missing=true]').count();if(required<2)throw Error('Missing route requirements');
  await check('route modal '+width);await page.screenshot({path:out+'/route-'+width+'.png',fullPage:true});await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).click();
 }
 await page.setViewportSize({width:1440,height:1000});await go('DNS 概览');
 await page.evaluate(()=>document.documentElement.dataset.theme='dark');await page.screenshot({path:out+'/dns-dark.png',fullPage:true});
 if(errors.length)throw Error(errors.join('\n'));
 fs.writeFileSync(out+'/verification.json',JSON.stringify({passed:true,widths:[1440,1120,390],pages:pages.length,categories:categories.length,errors},null,2));
 console.log(JSON.stringify({passed:true,pages:pages.length,categories:categories.length,widths:[1440,1120,390],screenshots:out}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
