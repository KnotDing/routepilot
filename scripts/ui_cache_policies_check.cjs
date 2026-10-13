// Exercise selective cache controls and statistics using isolated API fixtures.
const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'../web/node_modules/playwright');
const fs=require('node:fs');
const source=fs.readFileSync('web/src/full-i18n.test.jsx','utf8');
const fixture=Function(source.slice(source.indexOf('const settings='),source.indexOf('function noUntranslatedText'))+';return fixture;')();
const out='artifacts/cache-policies';fs.mkdirSync(out,{recursive:true});
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000},locale:'zh-CN'}),errors=[],checks=[];
  page.on('pageerror',error=>errors.push(error.message));
  const endpoint={name:'DoH 测试上游',enabled:true,protocol:'doh',address:'192.0.2.53:443',url:'https://dns.example.test/'+('long-upstream-path-'.repeat(12))};
  const identity=`doh://${endpoint.address}|${endpoint.url}`;
  let settings={...fixture('settings'),cache:{...fixture('settings').cache,optimistic:true,optimistic_responses:true},dns:{...fixture('settings').dns,upstreams:[endpoint]}};
  const surgeIdentity="surge://"+settings.surge.dns;
  const sample={entries:2,hits:30,misses:10,evictions:1,stale_entries:1,optimistic_hits:20,optimistic_compared:10,optimistic_mismatches:2};
  await page.route('**/*',async route=>{
   const url=new URL(route.request().url());
   if(url.pathname==='/api/v1/events/stream')return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
   if(url.pathname.startsWith('/api/v1/')){
    const path=url.pathname.slice(8)+url.search;
    let value=fixture(path);
    if(path==='settings'){
     if(route.request().method()==='PUT'){settings=route.request().postDataJSON();value={settings}}else value=settings;
    }
    if(path==='status')value={...value,selective_cache_policies:true,cache:{decision:{...sample,groups:{DIRECT:sample}},response:{...sample,groups:{[identity]:sample,[surgeIdentity]:sample}}}};
    if(path.startsWith('dns/cache?'))value={...value,entries:[],stats:{...sample,groups:{DIRECT:sample}},ttl:settings.cache};
    return route.fulfill({contentType:'application/json',body:JSON.stringify(value)});
   }
   const file=url.pathname==='/'?'web/dist/index.html':'web/dist'+url.pathname;
   if(fs.existsSync(file))return route.fulfill({path:file,contentType:file.endsWith('.css')?'text/css':file.endsWith('.js')?'application/javascript':'text/html'});
   return route.abort();
  });
  const go=async name=>{
   const button=page.getByRole('navigation').getByRole('button',{name,exact:true});
   if(!await button.isVisible())await page.getByRole('button',{name:'打开导航',exact:true}).click();
   await button.click();await page.getByRole('heading',{level:1,name,exact:true}).waitFor();
  };
  const check=async name=>{
   const size=await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth}));
   if(size.scroll>size.width+1)throw Error(name+' overflow '+JSON.stringify(size));
   checks.push({name,...size});
  };
  await page.goto('http://127.0.0.1:18918');await page.getByRole('heading',{name:'仪表盘',exact:true}).waitFor();
  for(const width of [1440,768,390]){
   await page.setViewportSize({width,height:1000});await go('设置');await page.locator('.tabs').getByRole('button',{name:'缓存',exact:true}).click();
   await page.getByRole('heading',{name:'按上游设置缓存',exact:true}).waitFor();
   if(width>1000){const boxes=await page.locator('.cache-policy-group').evaluateAll(items=>items.map(el=>({y:el.getBoundingClientRect().y,x:el.getBoundingClientRect().x})));if(boxes.length!==2||Math.abs(boxes[0].y-boxes[1].y)>1||boxes[0].x===boxes[1].x)throw Error('Cache groups did not appear side by side');}
   await check('cache settings '+width);await page.screenshot({path:`${out}/settings-${width}.png`,fullPage:true});
   await page.locator('.cache-policy-settings').screenshot({path:`${out}/groups-${width}.png`});
   await page.getByLabel('DIRECT 允许乐观缓存',{exact:true}).uncheck();
   await page.getByLabel('乐观决策缓存',{exact:true}).uncheck();
   if(await page.getByLabel('DIRECT 允许乐观缓存',{exact:true}).count())throw Error('Disabled decision master left item switches visible');
   await page.getByLabel('乐观决策缓存',{exact:true}).check();
   if(await page.getByLabel('DIRECT 允许乐观缓存',{exact:true}).isChecked())throw Error('Collapse reset a decision override');
   await page.getByLabel('乐观响应缓存',{exact:true}).uncheck();
   if(await page.getByLabel(identity+' 允许乐观缓存',{exact:true}).count())throw Error('Disabled response master left item switches visible');
   await page.getByLabel('乐观响应缓存',{exact:true}).check();
   await page.getByLabel(identity+' 允许乐观缓存',{exact:true}).uncheck();
   await page.getByLabel(surgeIdentity+' 允许乐观缓存',{exact:true}).uncheck();
   if(await page.getByRole('checkbox',{name:/缓存此项/}).count())throw Error('Ordinary caching control is still visible');
   await page.getByRole('button',{name:'保存设置',exact:true}).click();
   await page.waitForFunction(()=>!document.querySelector('.settings-form fieldset')?.disabled);
   if(settings.cache.decisions?.DIRECT?.optimistic!==false||settings.cache.upstreams?.[identity]?.optimistic!==false||settings.cache.upstreams?.[surgeIdentity]?.optimistic!==false)throw Error('Saved overrides lost explicit false');
   await page.getByLabel(identity+' 允许乐观缓存',{exact:true}).check();
   await page.getByLabel(surgeIdentity+' 允许乐观缓存',{exact:true}).check();
   await page.getByLabel('DIRECT 允许乐观缓存',{exact:true}).check();
   await page.locator('.tabs').getByRole('button',{name:'通知',exact:true}).click();
   if(await page.getByText('本机系统通知',{exact:true}).count()||await page.getByLabel('启用本机系统通知',{exact:true}).count())throw Error('Local notification card is still visible');
   await go('决策缓存');await page.getByRole('heading',{name:'各决策缓存统计',exact:true}).waitFor();await check('decision statistics '+width);await page.screenshot({path:`${out}/decisions-${width}.png`,fullPage:true});
   await go('查询记录');await page.getByRole('heading',{name:'各上游缓存统计',exact:true}).waitFor();await check('upstream statistics '+width);await page.screenshot({path:`${out}/upstreams-${width}.png`,fullPage:true});
  }
  await page.evaluate(()=>localStorage.setItem('routepilot.locale','en'));await page.reload();
  await page.getByRole('heading',{name:'Dashboard',exact:true}).waitFor();
  const navigation=page.getByRole('navigation');if(!await navigation.getByRole('button',{name:'Query history',exact:true}).isVisible())await page.getByRole('button',{name:'Open navigation',exact:true}).click();
  await navigation.getByRole('button',{name:'Query history',exact:true}).click();await page.getByRole('heading',{name:'Cache statistics by upstream',exact:true}).waitFor();
  if(errors.length)throw Error(errors.join('\n'));
  fs.writeFileSync(out+'/verification.json',JSON.stringify({checks,english:true,savedOverrides:true,pageErrors:errors},null,2));
  console.log(`Selective cache UI: ${checks.length} responsive checks, saved false overrides and English statistics passed.`);
 }finally{await browser.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
