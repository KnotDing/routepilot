// Inspect built UI against deterministic API fixtures, without a live backend.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'../web/node_modules/playwright');
const fs=require('node:fs');
const source=fs.readFileSync('web/src/full-i18n.test.jsx','utf8');
const fixture=Function(source.slice(source.indexOf('const settings='),source.indexOf('function noUntranslatedText'))+';return fixture;')();
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
 const page=await browser.newPage({viewport:{width:1440,height:1000},locale:'zh-CN',colorScheme:'light',reducedMotion:'reduce'});
 let crowdedPolicyFlows=false,sourceNames=false,monitorDimensions=false,remoteAppPending=false,appRestartPhase='awaiting_confirmation';
 const appRestartRequests=[];
 const release=JSON.parse(fs.readFileSync('internal/buildinfo/release.json','utf8'));
 const monitoringQueries=[];
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
   if(remoteAppPending&&['updates/check','updates/status','updates/restart-app'].includes(path)){
    if(path==='updates/restart-app'){appRestartRequests.push(route.request().postDataJSON());appRestartPhase='restarting'}
    value={current:release.version,app_current:release.version,latest:'v'+release.version,target:'v'+release.version,available:false,supported:true,phase:'succeeded',app_restart:appRestartPhase,app_restart_force_supported:true};
   }
   if(remoteAppPending&&path==='status')value={...value,software:{version:release.version,build:release.build}};
   if(path==='dns/clients')value={...value,clients:[{client_ip:'192.0.2.10',queries:12,direct:12,surge:0,reject:0,unknown:0,errors:0,last_seen:new Date().toISOString()}],stats:{...value.stats,clients:1,queries:12,direct:12}};
   if(path==='settings')value={...value,cache:{...value.cache,optimistic:false,optimistic_responses:false},notifications:{enabled:true,surge:true,bgp:true,dns:true,controller:true,recovery:true,cooldown_seconds:300,webhook_url:'',webhook_configured:false,errors:[]}};
   if(path==='settings'&&sourceNames)value={...value,dns:{...value.dns,clients:[{id:'named',ip:'192.0.2.10',name:sourceRemark,decision:''}]}};
   if(path==='status'){value.online_update=true;value.software={...value.software,...JSON.parse(fs.readFileSync('internal/buildinfo/release.json'))};}
   if(path==='status'&&crowdedPolicyFlows){
    const pairs=[['192.168.20.131','',1492],['192.168.20.131','Hidding',1061],['192.168.20.131','REJECT',900],['192.168.20.131','US_FallBack',130],['192.168.20.131','LX_US',20],['192.168.20.131','DIRECT',6],['192.168.20.3','REJECT',143],['192.168.20.3','US_FallBack',583],['192.168.20.3','LX_US',135],['192.168.20.3','DIRECT',58],['192.168.20.3','HK_FallBack',14],['192.168.20.3','JP_FallBack',6],['192.168.20.111','JP_FallBack',4],['192.168.20.111','SG_FallBack',5],['192.168.20.246','SG_FallBack',3]];
    value.dns_monitoring.source_policy_flows=pairs.map(([source,policy,queries],i)=>({source,policy,queries,kind:policy?'named':'unnamed',final_kind:policy?'named':'unnamed',final_policy:policy?'DNS Node '+i%10:''}));
   }
   if(path==='status'&&monitorDimensions)value.dns_monitoring.dimensions_supported=true;
   if(path.startsWith('dns/monitoring')&&monitorDimensions){
    monitoringQueries.push(url.search);
    const rows=[{source:'192.0.2.10',domain:'example.test',destination:'cache',code:'NOERROR',queries:4},{source:'192.0.2.20',domain:'example.test',destination:'cache',code:'NOERROR',queries:3},{source:'192.0.2.20',domain:'api.example.test',destination:'surge',code:'NXDOMAIN',queries:3}],selected=rows.filter(row=>['source','domain','destination'].every(key=>!url.searchParams.get(key)||url.searchParams.get(key)===row[key])&&(!url.searchParams.get('response_code')||url.searchParams.get('response_code')===row.code));
    const counts=items=>({started_at:'2026-10-13T00:00:00Z',observed_at:'2026-10-13T00:01:00Z',session_started_at:'dimension-fixture',received:items.reduce((sum,row)=>sum+row.queries,0),completed:items.reduce((sum,row)=>sum+row.queries,0),in_flight:0,unconfirmed:0,latency_ms_sum:10,flows:items.map(row=>({ingress:'addressed',destination:row.destination,queries:row.queries})),policies:items.map(row=>({policy:'Proxy',final_policy:'Node A',destination:row.destination,queries:row.queries})),decisions:{SURGE:items.reduce((sum,row)=>sum+row.queries,0)},response_codes:Object.fromEntries(['NOERROR','NXDOMAIN'].map(code=>[code,items.filter(row=>row.code===code).reduce((sum,row)=>sum+row.queries,0)]))});
    value={...counts(selected),dimensions_supported:true,dimension_completed:counts(selected).completed,completed_only:['domain','destination','policy','final_policy','response_code'].some(key=>url.searchParams.has(key)),filter_options:{destinations:['cache','surge'],policies:[{value:JSON.stringify(['named','Proxy']),name:'Proxy',kind:'named'}],final_policies:[{value:JSON.stringify(['named','Node A']),name:'Node A',kind:'named'}],domains:['api.example.test','example.test'],response_codes:['NOERROR','NXDOMAIN']},domains:[...new Set(selected.map(row=>row.domain))].map(domain=>({domain,queries:selected.filter(row=>row.domain===domain).reduce((sum,row)=>sum+row.queries,0)})),sources:[...new Set(selected.map(row=>row.source))].map(source=>({source,counts:counts(selected.filter(row=>row.source===source))})),source_policy_flows:selected.map(row=>({source:row.source,policy:'Proxy',kind:'named',final_policy:'Node A',final_kind:'named',queries:row.queries}))};
    if(!['domain','destination','policy','final_policy','response_code'].some(key=>url.searchParams.has(key)))value.domain_statistics={started_at:'2026-10-15T00:00:00Z',completed:counts(selected).completed,unrecorded:0};
    if(path.startsWith('dns/monitoring/history'))value={summary:value,sources:value.sources,source_policy_flows:value.source_policy_flows,from:value.started_at,to:value.observed_at,points:[{from:value.started_at,to:value.observed_at,seconds:60,rate_valid:true,counts:value}]};
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
 const categories=['通用','DNS','Surge','路由','缓存','健康','日志与隐私','安全','软件更新','大屏监控','通知','状态栏','CLI 与 skill','关于'];
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
   if(label==='缓存'){
    const decision=page.getByRole('checkbox',{name:'决策缓存',exact:true}),response=page.getByRole('checkbox',{name:'响应缓存',exact:true}),optimistic=page.getByRole('checkbox',{name:'乐观决策缓存',exact:true}),optimisticResponse=page.getByRole('checkbox',{name:'乐观响应缓存',exact:true});
    await optimistic.check();await optimisticResponse.check();
    await page.getByRole('checkbox',{name:'DIRECT 缓存此项',exact:true}).uncheck();
    const item=page.getByRole('checkbox',{name:'DIRECT 允许乐观缓存',exact:true});
    if(!(await item.isDisabled())||await item.isChecked())throw Error('Disabled decision still allows optimistic caching');
    const upstream='udp://223.6.6.6:53';await page.getByRole('checkbox',{name:upstream+' 缓存此项',exact:true}).uncheck();
    const responseItem=page.getByRole('checkbox',{name:upstream+' 允许乐观缓存',exact:true});
    if(!(await responseItem.isDisabled())||await responseItem.isChecked())throw Error('Disabled upstream still allows optimistic caching');
    await decision.uncheck();await response.uncheck();
    if(!(await optimistic.isDisabled())||await optimistic.isChecked()||!(await optimisticResponse.isDisabled())||await optimisticResponse.isChecked())throw Error('Disabled masters still allow optimistic caching');
    await decision.check();await response.check();await optimistic.check();await optimisticResponse.check();
    const panels=await page.locator('.cache-policy-group').evaluateAll(els=>els.map(el=>{const r=el.getBoundingClientRect();return {x:r.x,y:r.y,right:r.right,bottom:r.bottom}}));
    if(panels.length!==4||panels.some(r=>r.x<0||r.right>width))throw Error('Cache groups overflow viewport');
    if(width>900?(panels[0].y!==panels[1].y||panels[2].y!==panels[3].y):panels.some((r,i)=>i&&r.y<panels[i-1].bottom))throw Error('Cache group responsive layout failed');
    await page.screenshot({path:out+'/cache-controls-'+width+'.png',fullPage:true});
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
  const chartHeights=await page.locator('.monitor-flow-grid svg').evaluateAll(svgs=>svgs.map(svg=>svg.getBoundingClientRect().height));if(chartHeights[0]!==chartHeights[1])throw Error('Parallel flow charts have different heights');
  await check('monitoring 4K '+theme);await page.screenshot({path:out+'/monitoring-4k-'+theme+'.png',fullPage:true});
  await page.keyboard.press('Escape');
  if(!(await page.locator('.desktop-sidebar').isVisible()))throw Error('Escape did not exit monitoring focus mode');
 }
 async function flowMetrics(){
  await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  return page.locator('.monitor-flow-grid svg').evaluateAll(svgs=>svgs.map(svg=>({width:svg.getBoundingClientRect().width,height:svg.getBoundingClientRect().height,naturalHeight:Number(svg.dataset.naturalHeight),parallel:svg.closest('.panel').parentElement.children.length===2&&Math.abs(svg.closest('.panel').parentElement.children[0].getBoundingClientRect().y-svg.closest('.panel').parentElement.children[1].getBoundingClientRect().y)<1,nodes:[...svg.querySelectorAll('rect')].map(el=>[Number(el.getAttribute('y')),Number(el.getAttribute('height'))]),labels:[...svg.querySelectorAll('text')].map(el=>({font:getComputedStyle(el).fontSize,y:el.getAttribute('y'),scale:[el.getScreenCTM().a,el.getScreenCTM().d]})),verticalScroll:svg.parentElement.scrollHeight>svg.parentElement.clientHeight+1})));
 }
 function checkFlowResize(before,after){
  for(let i=0;i<before.length;i++){
   const a=before[i],b=after[i];if(a.width===b.width)throw Error('Flow width did not respond to viewport resize');
   if(a.naturalHeight!==b.naturalHeight||JSON.stringify(a.nodes.map(node=>node[1]))!==JSON.stringify(b.nodes.map(node=>node[1]))||JSON.stringify(a.labels.map(({y,...label})=>label))!==JSON.stringify(b.labels.map(({y,...label})=>label)))throw Error('Intrinsic flow height, band thickness or text size changed with viewport width');
   if(a.parallel===b.parallel&&(a.height!==b.height||JSON.stringify(a.nodes)!==JSON.stringify(b.nodes)||JSON.stringify(a.labels)!==JSON.stringify(b.labels)))throw Error('Flow vertical layout changed without a row-layout change');
   const expected=b.parallel?Math.max(...after.map(chart=>chart.naturalHeight)):b.naturalHeight;if(b.height!==expected)throw Error('Side-by-side charts do not match heights or stacked charts retain expanded height');
   if(a.verticalScroll||b.verticalScroll||b.labels.some(label=>label.scale.some(scale=>Math.abs(scale-1)>1e-8)))throw Error('Flow uses internal vertical scrolling or scales SVG text');
  }
 }
 let crowdedMetrics;
 crowdedPolicyFlows=true;await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();await go('大屏监控');
 for(const width of [1440,3840,2100,1440]){
  await page.setViewportSize({width,height:2160});await page.locator('.monitor-client-policy svg .monitor-node-label').filter({hasText:'Hidding'}).waitFor();
  const overlap=await page.locator('.monitor-client-policy svg').evaluate(svg=>{
   const sides={};for(const group of svg.querySelectorAll(':scope > g')){const rect=group.querySelector('rect'),texts=[...group.querySelectorAll(':scope > text')].map(el=>el.getBBox());if(!rect||!texts.length)continue;const key=rect.getAttribute('x');(sides[key]??=[]).push({top:Math.min(...texts.map(b=>b.y)),bottom:Math.max(...texts.map(b=>b.y+b.height))})}
   return Object.values(sides).some(side=>side.sort((a,b)=>a.top-b.top).some((box,i)=>i>0&&box.top<side[i-1].bottom));
  });if(overlap)throw Error('Crowded policy labels overlap at '+width);
  await page.locator('.monitor-client-policy svg').evaluate(svg=>{
   const bands=[...svg.querySelectorAll(':scope > path')].map(path=>{const n=path.getAttribute('d').match(/[-+]?\d*\.?\d+(?:e[-+]?\d+)?/gi).map(Number),count=Number(path.querySelector('title').textContent.match(/: ([\d,.]+) ·/)[1].replaceAll(',',''));return {n,scale:(n[15]-n[1])/count}});
   if(bands.some(b=>Math.abs(b.scale-bands[0].scale)>1e-8))throw Error('Three-stage ribbons do not use the same scale');
   for(const rect of svg.querySelectorAll('g > rect[data-column="policy"]')){const x=Number(rect.getAttribute('x')),y=Number(rect.getAttribute('y')),end=y+Number(rect.getAttribute('height')),incoming=bands.filter(b=>b.n[6]===x&&b.n[7]>=y-1e-8&&b.n[9]<=end+1e-8),outgoing=bands.filter(b=>b.n[0]===x+14&&b.n[1]>=y-1e-8&&b.n[15]<=end+1e-8);for(const [rows,top,bottom] of [[incoming,7,9],[outgoing,1,15]])if(!rows.length||Math.abs(Math.min(...rows.map(b=>b.n[top]))-y)>1e-8||Math.abs(Math.max(...rows.map(b=>b.n[bottom]))-end)>1e-8)throw Error('Ribbons do not align with the middle node');}
  });
  const exact=await page.locator('.monitor-client-policy').evaluate(card=>{
   const table=card.querySelector('tbody'),rows=[...table.rows],count=rows.reduce((sum,row)=>sum+Number(row.cells[3].textContent.replaceAll(',','')),0),percent=rows.reduce((sum,row)=>sum+Number(row.cells[4].textContent.replace('%','')),0);
   return {count,percent,unknown:[...card.querySelectorAll('text,td,title')].some(el=>el.textContent.trim()==='未记录最终策略')};
  });if(exact.count!==3068||Math.abs(exact.percent-100)>.1||exact.unknown)throw Error('Unrecorded final-policy requests remain in client flow totals: '+JSON.stringify(exact));
  const metrics=await flowMetrics();if(metrics.some(chart=>chart.height>600))throw Error('Short crowded labels create an oversized flow chart');if(crowdedMetrics)checkFlowResize(crowdedMetrics,metrics);crowdedMetrics=metrics;
  await page.locator('.monitor-client-policy').screenshot({path:out+'/monitoring-crowded-policies-'+width+'.png'});if(width===3840)await page.locator('.monitor-flow-grid').screenshot({path:out+'/monitoring-equal-heights.png'});
 }
 // A smaller live snapshot must shrink the shared height, rather than
 // feeding the previously expanded SVG height back into the next layout.
 await page.setViewportSize({width:3840,height:2160});crowdedPolicyFlows=false;
 await page.waitForFunction(()=>![...document.querySelectorAll('.monitor-client-policy svg .monitor-node-label')].some(el=>el.textContent.includes('Hidding')));
 const reduced=await flowMetrics();if(reduced.some(chart=>chart.height!==Math.max(...reduced.map(item=>item.naturalHeight)))||Math.max(...reduced.map(chart=>chart.height))>=Math.max(...crowdedMetrics.map(chart=>chart.height)))throw Error('Parallel charts retain the previous taller snapshot height');
 let namedMetrics;
 crowdedPolicyFlows=false;sourceNames=true;await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();await go('大屏监控');
 for(const width of [1440,3840,2100,1440]){
  await page.setViewportSize({width,height:2160});
  for(const css of ['.monitor-flow','.monitor-client-policy']){
   const label=page.locator(css+' .monitor-source-label[role=button]');await label.waitFor();
   const text=()=>label.evaluate(el=>[...el.querySelectorAll('tspan')].map(t=>t.textContent).join(''));
   if(await text()!==sourceRemark)throw Error('Remark label is truncated or includes IP');
   await label.click();if(await text()!=='192.0.2.10')throw Error('Remark did not toggle to IP');await label.press('Enter');if(await text()!==sourceRemark)throw Error('IP did not toggle back to name');
  }
  const metrics=await flowMetrics();if(namedMetrics)checkFlowResize(namedMetrics,metrics);namedMetrics=metrics;
  await page.screenshot({path:out+'/monitoring-named-'+width+'.png',fullPage:true});
 }
 await page.setViewportSize({width:1440,height:1000});await go('设置');await page.locator('.tabs').getByRole('button',{name:'大屏监控',exact:true}).click();
 await page.getByRole('checkbox',{name:'DNS 请求流向',exact:true}).uncheck();await page.getByRole('checkbox',{name:'平均处理耗时',exact:true}).uncheck();await page.getByRole('checkbox',{name:'启用敏感数据保护',exact:true}).check();await page.getByRole('button',{name:'保存设置',exact:true}).click();await page.getByText('大屏监控设置已保存',{exact:true}).waitFor();
 await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();await go('大屏监控');await page.getByText('敏感数据保护已启用',{exact:true}).waitFor();
 if(await page.locator('.monitor-flow,.monitor-metrics').filter({hasText:'平均处理耗时'}).count()||await page.locator('.monitor-flow').count())throw Error('Hidden monitor cards returned after reload');
 const singleFlow=await flowMetrics();if(singleFlow.length!==1||singleFlow[0].height!==singleFlow[0].naturalHeight)throw Error('A single visible flow chart retains shared height');
 const protectedHTML=await page.locator('.layout').innerHTML();for(const secret of ['192.0.2.10','192.0.2.1:1055',sourceRemark,'Proxy','Node A'])if(protectedHTML.includes(secret))throw Error('Sensitive monitor data remains in text, attributes or tooltips: '+secret);
 await page.getByRole('button',{name:'查看 客户端 1 的流向',exact:true}).click();await page.getByText('当前统计来源：客户端 1',{exact:true}).waitFor();await page.getByRole('heading',{name:'客户端策略流向',exact:true}).waitFor();await page.locator('.monitor-policies .monitor-policy-name').filter({hasText:'策略 1'}).waitFor();const selectedHTML=await page.locator('.layout').innerHTML();for(const secret of ['192.0.2.10',sourceRemark,'Proxy','Node A'])if(selectedHTML.includes(secret))throw Error('Selected protected source exposed '+secret);await page.screenshot({path:out+'/monitoring-protected-1440.png',fullPage:true});
 await go('设置');await page.locator('.tabs').getByRole('button',{name:'大屏监控',exact:true}).click();await page.getByRole('checkbox',{name:'DNS 请求流向',exact:true}).check();await page.getByRole('checkbox',{name:'平均处理耗时',exact:true}).check();await page.getByRole('checkbox',{name:'启用敏感数据保护',exact:true}).uncheck();await page.getByRole('button',{name:'保存设置',exact:true}).click();
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
 remoteAppPending=true;await page.reload();await page.getByRole('button',{name:'强制重启远程 App',exact:true}).waitFor();
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await check('remote App reminder '+width);
  const reminder=page.locator('.app-restart-notice');
  if(!(await reminder.evaluate(el=>!el.classList.contains('notice')&&getComputedStyle(el).backgroundColor==='rgba(0, 0, 0, 0)'&&getComputedStyle(el).borderTopStyle==='none')))throw Error('Pending App restart still uses a warning banner');
  await reminder.screenshot({path:out+'/remote-app-reminder-'+width+'.png'});
  await page.getByRole('button',{name:'强制重启远程 App',exact:true}).click();
  const restart=page.getByRole('dialog',{name:'强制重启远程 App',exact:true});
  await restart.getByText(/网络后台、DNS 和 BGP 继续运行/).waitFor();await check('force App restart dialog '+width);
  if(!(await restart.getByRole('button',{name:'取消',exact:true}).evaluate(el=>el===document.activeElement)))throw Error('Forced App restart did not default focus to Cancel');
  await page.screenshot({path:out+'/force-app-restart-'+width+'.png',fullPage:true});
  await restart.getByRole('button',{name:'取消',exact:true}).click();
 }
 if(appRestartRequests.length)throw Error('Cancel submitted an App restart');
 await page.getByRole('button',{name:'强制重启远程 App',exact:true}).click();
 await page.getByRole('dialog',{name:'强制重启远程 App',exact:true}).getByRole('button',{name:'确认强制重启',exact:true}).click();
 await page.getByRole('dialog').waitFor({state:'hidden'});
 if(appRestartRequests.length!==1||appRestartRequests[0].version!=='v'+release.version)throw Error('Wrong App restart target or duplicate request');
 remoteAppPending=false;
 monitorDimensions=true;await page.setViewportSize({width:1440,height:1000});await page.evaluate(()=>{sessionStorage.removeItem('routepilot.backend')});await page.reload();await page.getByRole('heading',{level:1,name:'仪表盘',exact:true}).waitFor();await go('大屏监控');await page.getByRole('button',{name:'筛选域名：example.test',exact:true}).waitFor();await page.locator('.monitor-domain-shares').getByText(/域名统计起点/).waitFor();
 for(const width of [1440,390]){
  await page.setViewportSize({width,height:1000});await check('monitor filters '+width);
  const inline=await page.locator('.monitor-view-controls label').evaluateAll(labels=>labels.every(label=>{const select=label.querySelector('select');if(!select)return true;const range=document.createRange();range.selectNode(label.firstChild);const text=range.getBoundingClientRect(),box=select.getBoundingClientRect();return text.right<=box.left&&Math.abs((text.top+text.bottom-box.top-box.bottom)/2)<2}));if(!inline)throw Error('Monitoring filter names are not before the selects');
  for(const card of ['.monitor-source-shares','.monitor-domain-shares']){const spacing=await page.locator(card+' .monitor-bar-row').evaluateAll(rows=>rows.length>1?rows[1].getBoundingClientRect().y-rows[0].getBoundingClientRect().y:0);if(spacing>32)throw Error('Source/domain ranking rows are not compact');}
  await page.screenshot({path:out+'/monitoring-domain-filters-'+width+'.png',fullPage:true});
 }
 for(const [label,value] of [['请求来源','192.0.2.20'],['DNS 处理去向','cache'],['策略 / 策略组',JSON.stringify(['named','Proxy'])],['最终策略',JSON.stringify(['named','Node A'])],['域名','example.test'],['响应码','NOERROR']])await page.getByRole('combobox',{name:label,exact:true}).selectOption(value);
 await page.waitForFunction(()=>document.querySelector('.monitor-metrics')?.textContent.includes('筛选完成请求3'));if(!monitoringQueries.some(query=>{const params=new URLSearchParams(query);return ['source','destination','policy','final_policy','domain','response_code'].every(key=>params.get(key))}))throw Error('Combined monitoring filters were not sent together');
 await page.getByRole('button',{name:'历史数据',exact:true}).click();await page.getByText('窗口末处理中',{exact:true}).waitFor();if(!(await page.locator('.monitor-metrics').textContent()).includes('筛选完成请求3'))throw Error('Historical monitoring ignores combined filters');
 if(errors.length)throw Error(errors.join('\n'));
 fs.writeFileSync(out+'/verification.json',JSON.stringify({passed:true,widths:[1440,1120,390],pages:pages.length,categories:categories.length,flowWidths:[1440,3840,2100,1440],equalParallelHeights:true,liveHeightReduction:true,combinedMonitoringFilters:true,compactSourceDomainRows:true,inlineFilterNames:true,quietAppRestartReminder:true,confirmedRemoteAppRestart:true,errors},null,2));
 console.log(JSON.stringify({passed:true,pages:pages.length,categories:categories.length,widths:[1440,1120,390],screenshots:out}));
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
