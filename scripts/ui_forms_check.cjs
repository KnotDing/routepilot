// Inspect built UI against deterministic API fixtures, without a live backend.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'../web/node_modules/playwright');
const fs=require('node:fs');
const source=fs.readFileSync('web/src/full-i18n.test.jsx','utf8');
const fixture=Function(source.slice(source.indexOf('const settings='),source.indexOf('function noUntranslatedText'))+';return fixture;')();
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
 const page=await browser.newPage({viewport:{width:1440,height:1000},locale:'zh-CN',colorScheme:'light',reducedMotion:'reduce'});
 let crowdedPolicyFlows=false,sourceNames=false;
 const sourceRemark='住院楼二层护理站工作电脑用于观察实时解析策略与最终出口名称';
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const out='artifacts/ui-forms';fs.mkdirSync(out,{recursive:true});
 await page.route('**/*',async route=>{
  const url=new URL(route.request().url());
  if(url.pathname==='/api/v1/events/stream')return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
  if(url.pathname.startsWith('/api/v1/')){
   const path=(url.pathname.slice(8)+url.search).replace(/^management\/relay\/preview\//,'');
   let value=fixture(path);
   if(path==='management/backends')value=[{id:'preview',name:'远程预览',url:'http://192.0.2.1:1055'}];
   if(path==='updates/check')value={app_current:'0.4.16',current:'0.4.17',latest:'v0.4.18',available:true,supported:true,phase:'idle',release_notes:'## 界面优化\n\n- **Markdown** 渲染更新说明。\n- 紧凑排列开关选项。\n\n查看 [项目发布](https://gitea.knotding.com/knot/routepilot/releases)。\n\n<script>window.untrustedRelease=true</script>\n\n![image](https://example.invalid/image.png)'};
   if(path==='dns/clients')value={...value,clients:[{client_ip:'192.0.2.10',queries:12,direct:12,surge:0,reject:0,unknown:0,errors:0,last_seen:new Date().toISOString()}],stats:{...value.stats,clients:1,queries:12,direct:12}};
   if(path==='settings')value={...value,cache:{...value.cache,optimistic:false,optimistic_responses:false},notifications:{enabled:true,surge:true,bgp:true,dns:true,controller:true,recovery:true,cooldown_seconds:300,webhook_url:'',webhook_configured:false,errors:[]}};
   if(path==='settings'&&sourceNames)value={...value,dns:{...value.dns,clients:[{id:'named',ip:'192.0.2.10',name:sourceRemark,decision:''}]}};
   if(path==='status'){value.online_update=true;value.software={...value.software,...JSON.parse(fs.readFileSync('internal/buildinfo/release.json'))};}
   if(path==='status'&&crowdedPolicyFlows){
    const pairs=[['192.168.20.131','',1492],['192.168.20.131','Hidding',1061],['192.168.20.131','REJECT',900],['192.168.20.131','US_FallBack',130],['192.168.20.131','LX_US',20],['192.168.20.131','DIRECT',6],['192.168.20.3','REJECT',143],['192.168.20.3','US_FallBack',583],['192.168.20.3','LX_US',135],['192.168.20.3','DIRECT',58],['192.168.20.3','HK_FallBack',14],['192.168.20.3','JP_FallBack',6],['192.168.20.111','JP_FallBack',4],['192.168.20.111','SG_FallBack',5],['192.168.20.246','SG_FallBack',3]];
    value.dns_monitoring.source_policy_flows=pairs.map(([source,policy,queries])=>({source,policy,queries,kind:policy?'named':'unnamed',final_kind:policy?'named':'unnamed',final_policy:policy?policy+'_Node':''}));
   }
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
 const pages=['仪表盘','大屏监控','DNS 概览','查询记录','决策缓存','客户端','DNS 拦截','本地 DNS','域名覆盖','规则测试','路由','BGP 概览','BGP 邻居','手动路由','健康状态','事件','设置'];
 const categories=['通用','DNS','Surge','路由','缓存','健康','日志与隐私','安全','软件更新','通知','状态栏','CLI 与 skill','关于'];
 for(const width of [1440,1120,390]){
  await page.setViewportSize({width,height:1000});
  for(const label of pages){await go(label);await check(width+' '+label);}
  for(const label of categories){
   await page.locator('.tabs').getByRole('button',{name:label,exact:true}).click();
   if(label==='路由'){
    const mode=page.getByLabel('路由模式',{exact:true});
    await page.getByRole('heading',{name:'BGP',exact:true}).waitFor();
    await mode.selectOption('static');
    await page.getByRole('heading',{name:'BGP',exact:true}).waitFor({state:'detached'});
    await mode.selectOption('bgp');
    await page.getByRole('heading',{name:'BGP',exact:true}).waitFor();
   }
   if(label==='软件更新'){const save=page.getByRole('button',{name:'保存设置',exact:true}),checkButton=page.getByRole('button',{name:'立即检查更新',exact:true});await checkButton.waitFor();if(!(await checkButton.evaluate((el)=>el.parentElement.querySelector('button.primary')?.textContent==='保存设置')))throw Error('Update check button is not beside Save');const install=page.getByRole('button',{name:'升级应用',exact:true});if(!(await install.evaluate(el=>[...el.parentElement.querySelectorAll('button')].some(button=>button.textContent==='立即检查更新'))))throw Error('Install button is not beside update check');const buttonY=await checkButton.evaluate(el=>el.getBoundingClientRect().y),installY=await install.evaluate(el=>el.getBoundingClientRect().y);if(Math.abs(buttonY-installY)>1)throw Error('Update check and install buttons are not on one row');if(await page.getByText('检查与升级针对当前连接的后端所在 Mac。',{exact:true}).count())throw Error('Removed update hint remains');if(await page.locator('.settings-form .update-status').evaluateAll(els=>els.some(el=>getComputedStyle(el).borderBottomStyle!=='none')))throw Error('Update settings divider remains');await page.screenshot({path:out+'/update-settings-'+width+'.png',fullPage:true});}
   await check(width+' '+label);
  }
  await go('客户端');await page.getByRole('heading',{name:'客户端列表',exact:true}).waitFor();if(await page.locator('.panel').getByRole('heading',{name:'客户端设置',exact:true}).count())throw Error('Standalone client settings card remains');await page.getByRole('button',{name:'编辑客户端 192.0.2.10',exact:true}).click();
  await page.getByRole('dialog',{name:'客户端设置',exact:true}).waitFor();if(!(await page.getByLabel('客户端 IP',{exact:true}).getAttribute('readonly')!==null))throw Error('Client IP is not locked to selected row');await page.getByLabel('备注名称',{exact:true}).fill('Office PC');await page.getByLabel('查询策略',{exact:true}).selectOption('SURGE');await check('client settings editor '+width);await page.screenshot({path:out+'/client-settings-'+width+'.png',fullPage:true});await page.getByRole('button',{name:'取消',exact:true}).click();
  await go('大屏监控');
  if(await page.getByRole('heading',{name:'按源统计 DNS 流入',exact:true}).count())throw Error('Standalone source panel remains');
  await page.locator('.monitor-source-shares').getByRole('heading',{name:'请求来源占比',exact:true}).waitFor();
  if(await page.locator('.monitor-flow').getByRole('heading',{name:'请求来源占比',exact:true}).count())throw Error('Source shares remain inside the flow card');
  await page.getByRole('img',{name:'DNS 来源到实际处理去向的流向图',exact:true}).waitFor();
  await page.getByRole('img',{name:'客户端到策略或策略组的流向图',exact:true}).waitFor();
  if(width===1440){const a=await page.locator('.monitor-flow').boundingBox(),b=await page.locator('.monitor-client-policy').boundingBox();if(b.y<a.y+a.height)throw Error('Narrow flow cards did not stack');const clipped=await page.locator('.monitor-flow-grid .monitor-chart-scroll').evaluateAll(els=>els.some(el=>el.scrollWidth>el.clientWidth));if(clipped)throw Error('Wide flow charts have clipped labels');}
  await page.getByRole('button',{name:'查看 192.0.2.10 的流向',exact:true}).click();
  await page.getByText('当前统计来源：192.0.2.10',{exact:true}).waitFor();
  await page.locator('.monitor-policies .monitor-policy-name').filter({hasText:'Proxy'}).waitFor();
  if(await page.locator('.monitor-policies .monitor-policy-name').filter({hasText:'DIRECT'}).count())throw Error('Source selection leaked other policies');
  await page.getByRole('button',{name:'历史数据',exact:true}).click();
  await page.getByText('窗口末处理中',{exact:true}).waitFor();await check('source history '+width);
  await page.screenshot({path:out+'/monitoring-history-'+width+'.png',fullPage:true});
  await page.getByLabel('请求来源',{exact:true}).selectOption('');await page.getByRole('button',{name:'实时数据',exact:true}).click();
  await page.getByRole('heading',{name:'DNS 请求流向',exact:true}).waitFor();
  await page.screenshot({path:out+'/monitoring-'+width+'.png',fullPage:true});
  await go('DNS 概览');await page.screenshot({path:out+'/dns-'+width+'.png',fullPage:true});
  await go('设置');await page.locator('.tabs').getByRole('button',{name:'DNS',exact:true}).click();await page.screenshot({path:out+'/dns-settings-'+width+'.png',fullPage:true});
  await page.locator('.tabs').getByRole('button',{name:'Surge',exact:true}).click();await page.getByLabel('启用 Surge 异常重启',{exact:true}).check();await page.getByLabel('Surge DNS 不可用时重启',{exact:true}).check();await check('surge restart '+width);await page.screenshot({path:out+'/surge-restart-'+width+'.png',fullPage:true});
  await page.locator('.tabs').getByRole('button',{name:'缓存',exact:true}).click();await page.screenshot({path:out+'/cache-settings-'+width+'.png',fullPage:true});
  if(width>1000){const boxes=await page.locator('.option-grid > .check').evaluateAll(items=>items.map(el=>el.getBoundingClientRect().y));if(new Set(boxes).size===boxes.length)throw Error('Cache switches did not share rows');}
  await page.locator('.tabs').getByRole('button',{name:'路由',exact:true}).click();await page.screenshot({path:out+'/routing-settings-'+width+'.png',fullPage:true});
  await page.locator('.tabs').getByRole('button',{name:'安全',exact:true}).click();await page.getByLabel('启用管理认证',{exact:true}).uncheck();await page.getByLabel('启用管理认证',{exact:true}).check();
  if(!(await page.getByLabel('新密码（至少 12 字节）',{exact:true}).getAttribute('required')!==null))throw Error('Initial password requirement did not update');
  await page.screenshot({path:out+'/security-'+width+'.png',fullPage:true});
  await go('手动路由');await page.getByRole('button',{name:'＋ 添加',exact:true}).click();
  const required=await page.locator('.modal label[data-missing=true]').count();if(required<2)throw Error('Missing route requirements');
  await check('route modal '+width);await page.screenshot({path:out+'/route-'+width+'.png',fullPage:true});await page.getByRole('dialog').getByRole('button',{name:'关闭',exact:true}).click();
 }
 for(const theme of ['light','dark']){
  await page.setViewportSize({width:3840,height:2160});await go('大屏监控');
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  await page.getByRole('button',{name:'大屏模式',exact:true}).click();
  if(await page.locator('.desktop-sidebar').isVisible())throw Error('Monitoring focus mode still shows navigation');
  const a=await page.locator('.monitor-flow').boundingBox(),b=await page.locator('.monitor-client-policy').boundingBox();if(Math.abs(a.y-b.y)>1||b.x<a.x+a.width)throw Error('4K flow cards are not side by side');
  await check('monitoring 4K '+theme);await page.screenshot({path:out+'/monitoring-4k-'+theme+'.png',fullPage:true});
  await page.keyboard.press('Escape');
  if(!(await page.locator('.desktop-sidebar').isVisible()))throw Error('Escape did not exit monitoring focus mode');
 }
 crowdedPolicyFlows=true;await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();await go('大屏监控');
 for(const width of [1440,3840]){
  await page.setViewportSize({width,height:2160});await page.locator('.monitor-client-policy svg text[x="635"]').filter({hasText:'Hidding'}).waitFor();
  const overlap=await page.locator('.monitor-client-policy svg').evaluate(svg=>{
   const sides={};for(const group of svg.querySelectorAll(':scope > g')){const rect=group.querySelector('rect'),texts=[...group.querySelectorAll(':scope > text')].map(el=>el.getBBox());if(!rect||!texts.length)continue;const key=rect.getAttribute('x');(sides[key]??=[]).push({top:Math.min(...texts.map(b=>b.y)),bottom:Math.max(...texts.map(b=>b.y+b.height))})}
   return Object.values(sides).some(side=>side.sort((a,b)=>a.top-b.top).some((box,i)=>i>0&&box.top<side[i-1].bottom));
  });if(overlap)throw Error('Crowded policy labels overlap at '+width);
  await page.locator('.monitor-client-policy').screenshot({path:out+'/monitoring-crowded-policies-'+width+'.png'});
 }
 crowdedPolicyFlows=false;sourceNames=true;await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();await go('大屏监控');
 for(const width of [1440,3840]){
  await page.setViewportSize({width,height:2160});
  for(const css of ['.monitor-flow','.monitor-client-policy']){
   const label=page.locator(css+' .monitor-source-label[role=button]');await label.waitFor();
   const text=()=>label.evaluate(el=>[...el.querySelectorAll('tspan')].map(t=>t.textContent).join(''));
   if(await text()!==sourceRemark)throw Error('Remark label is truncated or includes IP');
   await label.click();if(await text()!=='192.0.2.10')throw Error('Remark did not toggle to IP');await label.press('Enter');if(await text()!==sourceRemark)throw Error('IP did not toggle back to name');
  }
  await page.screenshot({path:out+'/monitoring-named-'+width+'.png',fullPage:true});
 }
 sourceNames=false;await page.setViewportSize({width:1440,height:1000});
 await page.getByRole('button',{name:'收起侧边栏',exact:true}).click();
 if(!(await page.locator('.desktop-sidebar').evaluate(el=>el.classList.contains('sidebar-icons')&&el.getBoundingClientRect().width===72&&getComputedStyle(el.querySelector('.nav-label')).display==='none')))throw Error('Sidebar icon mode failed');
 await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();if(!(await page.locator('.desktop-sidebar').evaluate(el=>el.classList.contains('sidebar-icons'))))throw Error('Sidebar preference not restored');
 await page.getByRole('button',{name:'隐藏侧边栏',exact:true}).click();if(await page.locator('.desktop-sidebar').isVisible())throw Error('Hidden sidebar remains visible');
 if(await page.locator('main').evaluate(el=>parseFloat(getComputedStyle(el).marginLeft))!==0)throw Error('Hidden sidebar still occupies page space');
 await page.getByRole('button',{name:'显示侧边栏',exact:true}).hover();await page.getByRole('button',{name:'固定侧边栏',exact:true}).waitFor();
 await page.mouse.move(500,400);if(await page.locator('.desktop-sidebar').isVisible())throw Error('Floating sidebar did not hide on mouse leave');
 await page.getByRole('button',{name:'显示侧边栏',exact:true}).hover();await page.getByRole('button',{name:'固定侧边栏',exact:true}).click();
 await go('DNS 概览');
 await page.evaluate(()=>document.documentElement.dataset.theme='dark');await page.screenshot({path:out+'/dns-dark.png',fullPage:true});
 await page.evaluate(()=>{sessionStorage.setItem('routepilot.backend','preview');document.documentElement.dataset.theme='light'});await page.reload();
 await page.getByRole('button',{name:'升级远程应用',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:/远程软件更新/});await dialog.locator('summary').click();
 await dialog.getByRole('heading',{name:'界面优化',exact:true}).waitFor();
 if(await dialog.locator('li').count()!==2||await dialog.locator('strong').count()!==1)throw Error('Release Markdown structure missing');
 if(await page.evaluate(()=>Boolean(window.untrustedRelease))||await dialog.locator('img,script').count())throw Error('Unsafe release content rendered');
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await check('update dialog '+width);
  const rows=await dialog.locator('footer button').evaluateAll(items=>items.map(el=>el.getBoundingClientRect().y));if(new Set(rows).size!==1)throw Error('Update buttons are not horizontal');
  await page.screenshot({path:out+'/update-web-'+width+'.png',fullPage:true});
 }
 await page.setViewportSize({width:1440,height:1000});await page.evaluate(()=>document.documentElement.dataset.theme='dark');await page.screenshot({path:out+'/update-web-dark.png',fullPage:true});
 await dialog.getByRole('button',{name:'稍后升级',exact:true}).click();await page.getByRole('dialog').waitFor({state:'hidden'});
 if(errors.length)throw Error(errors.join('\n'));
 fs.writeFileSync(out+'/verification.json',JSON.stringify({passed:true,widths:[1440,1120,390],pages:pages.length,categories:categories.length,errors},null,2));
 console.log(JSON.stringify({passed:true,pages:pages.length,categories:categories.length,widths:[1440,1120,390],screenshots:out}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
