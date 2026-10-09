const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'../web/node_modules/playwright');
const fs=require('node:fs');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try {
 const page=await browser.newPage({viewport:{width:1440,height:1000}});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(process.env.CONTROLLER_URL||'http://127.0.0.1:1055');
 await page.getByRole('heading',{name:'仪表盘',exact:true}).waitFor();
 for(const label of ['DNS 概览','查询记录','决策缓存','DNS 拦截','本地 DNS','域名覆盖','规则测试','路由后端','BGP 概览','BGP 邻居','手动路由','健康状态','事件','设置']){
  await page.getByRole('button',{name:label,exact:true}).click();
  await page.getByRole('heading',{name:label,exact:true}).waitFor();
 }
 fs.mkdirSync('artifacts/ui',{recursive:true});
 // Remove only fixtures left by an interrupted run of this exact test.
 await page.evaluate(async()=>{
  const s=await (await fetch('/api/v1/session')).json();
  const rows=await (await fetch('/api/v1/dns/hosts')).json();
  for(const row of rows||[])if(['ui-acceptance.example','*.ui-acceptance.example'].includes(row.domain))await fetch('/api/v1/dns/hosts/'+row.id,{method:'DELETE',headers:{'X-CSRF-Token':s.csrf}});
 });
 await page.getByRole('navigation').getByRole('button',{name:'路由后端',exact:true}).click();
 await page.getByLabel('路由模式',{exact:true}).selectOption('static');
 await page.getByRole('button',{name:'保存路由设置',exact:true}).click();
 await page.getByRole('status').filter({hasText:'路由配置已保存'}).waitFor();
 await page.getByLabel('Fake-IP 路由已在网关配置',{exact:true}).waitFor();
 await page.screenshot({path:'artifacts/ui/routing-static.png',fullPage:true});
 await page.getByLabel('路由模式',{exact:true}).selectOption('hybrid');
 await page.getByLabel('Fake-IP 路由后端',{exact:true}).selectOption('static');
 await page.getByLabel('DNS 拦截路由后端',{exact:true}).selectOption('bgp');
 await page.getByRole('button',{name:'保存路由设置',exact:true}).click();
 await page.waitForTimeout(1000);
 await page.screenshot({path:'artifacts/ui/routing-hybrid.png',fullPage:true});
 await page.getByLabel('路由模式',{exact:true}).selectOption('bgp');
 await page.getByRole('button',{name:'保存路由设置',exact:true}).click();
 await page.waitForTimeout(1000);
 // Validate actual resource CRUD through the UI, with the entry disabled.
 await page.getByRole('button',{name:'DNS 拦截',exact:true}).click();
 await page.getByRole('button',{name:'＋ 添加',exact:true}).click();
 await page.getByLabel('名称',{exact:true}).fill('Docker acceptance (disabled)');
 await page.getByLabel('网段',{exact:true}).fill('8.8.8.8/32');
 await page.getByLabel('启用',{exact:true}).uncheck();
 await page.getByRole('button',{name:'保存',exact:true}).click();
 await page.getByRole('cell',{name:'Docker acceptance (disabled)',exact:true}).waitFor();
 await page.getByRole('cell',{name:'Docker acceptance (disabled)',exact:true}).click();
 await page.getByLabel('名称',{exact:true}).fill('Docker acceptance edited (disabled)');
 await page.getByRole('button',{name:'保存',exact:true}).click();
 await page.getByRole('cell',{name:'Docker acceptance edited (disabled)',exact:true}).waitFor();
 await page.getByRole('cell',{name:'Docker acceptance edited (disabled)',exact:true}).click();
 await page.getByRole('button',{name:'删除',exact:true}).click();
 await page.getByRole('cell',{name:'Docker acceptance edited (disabled)',exact:true}).waitFor({state:'detached'});

 await page.getByRole('button',{name:'本地 DNS',exact:true}).click();
 await page.getByRole('button',{name:'＋ 添加',exact:true}).click();
 await page.getByLabel('域名',{exact:true}).fill('ui-acceptance.example');
 await page.getByLabel('IP 地址（每行一项）',{exact:true}).fill('192.0.2.53\n2001:db8::53');
 await page.getByLabel('TTL（秒）',{exact:true}).fill('120');
 await page.getByRole('button',{name:'保存',exact:true}).click();
 await page.getByRole('cell',{name:'ui-acceptance.example',exact:true}).waitFor();
 await page.screenshot({path:'artifacts/ui/hosts.png',fullPage:true});
 await page.getByRole('cell',{name:'ui-acceptance.example',exact:true}).click();
 await page.getByRole('dialog',{name:'编辑',exact:true}).waitFor();
 await page.getByLabel('匹配方式',{exact:true}).selectOption('wildcard');
 await page.getByLabel('域名',{exact:true}).fill('*.ui-acceptance.example');
 await page.getByRole('button',{name:'保存',exact:true}).click();
 await page.getByRole('cell',{name:'*.ui-acceptance.example',exact:true}).waitFor();
 await page.getByRole('cell',{name:'*.ui-acceptance.example',exact:true}).click();
 await page.getByRole('button',{name:'删除',exact:true}).click();
 await page.getByRole('cell',{name:'*.ui-acceptance.example',exact:true}).waitFor({state:'detached'});
 await page.waitForTimeout(5000);
 await page.getByRole('button',{name:'仪表盘',exact:true}).click();
 fs.mkdirSync('artifacts/ui',{recursive:true});
 await page.screenshot({path:'artifacts/ui/dashboard.png',fullPage:true});
 await page.getByRole('button',{name:'设置',exact:true}).click();
 await page.getByRole('button',{name:'安全',exact:true}).click();
 const password=page.getByLabel('新密码（至少 12 字节）',{exact:true});
 await password.fill('acceptance-dummy-not-secret');
 await page.waitForTimeout(3500);
 if(await password.inputValue()!=='acceptance-dummy-not-secret')throw new Error('settings polling erased password draft');
 if(await password.getAttribute('type')!=='password')throw new Error('password field is unmasked');
 await password.fill('');
 await page.getByRole('button',{name:'Surge',exact:true}).click();
 const token=page.getByLabel('API 凭据（留空保留）',{exact:true});
 await token.fill('acceptance-dummy-not-secret');
 if(await token.inputValue()!=='acceptance-dummy-not-secret'||await token.getAttribute('type')!=='password')throw new Error('token draft/secret input contract');
 await token.fill('');
 await page.getByRole('button',{name:'DNS',exact:true}).click();
 await page.screenshot({path:'artifacts/ui/settings.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});
 await page.getByRole('button',{name:'仪表盘',exact:true}).click();
 await page.screenshot({path:'artifacts/ui/mobile.png',fullPage:true});

 if(errors.length)throw new Error(errors.join('\n'));
 console.log(JSON.stringify({navigation_pages:15,crud:'create/edit/delete via actual API',browser_errors:errors.length,screenshots:'artifacts/ui'}));
 } finally { await browser.close() }
})().catch(e=>{console.error(e.message);process.exitCode=1});
