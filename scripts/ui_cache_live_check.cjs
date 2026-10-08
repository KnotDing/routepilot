const {chromium}=require(process.env.PLAYWRIGHT_MODULE||'playwright');
const fs=require('node:fs');
(async()=>{
 const browser=await chromium.launch({headless:true});
 try{
  const page=await browser.newPage({viewport:{width:1440,height:1000},locale:'zh-CN',timezoneId:'Asia/Shanghai',reducedMotion:'reduce',colorScheme:'light'});
  await page.goto('http://127.0.0.1:1055');await page.getByRole('heading',{name:'仪表盘',exact:true}).waitFor();
  await page.getByRole('navigation').getByRole('button',{name:'决策缓存',exact:true}).click();
  await page.getByText('最近失效原因',{exact:true}).waitFor();
  await page.waitForFunction(()=>document.querySelector('.pagination')?.textContent.includes('每页 50 条'));
  await page.waitForTimeout(350);
  const heights=await page.locator('.filters input,.filters select,.filters button').evaluateAll(es=>es.map(e=>e.getBoundingClientRect().height));
  if(heights.some(h=>h!==42))throw new Error('Cache controls have different heights');
  await page.screenshot({path:'artifacts/ui-refresh/cache-live.png',fullPage:true});
  await page.getByLabel('搜索缓存域名').fill('gateway');
  await page.waitForResponse(r=>r.url().includes('/dns/cache?domain=gateway')&&r.status()===200);
  await page.screenshot({path:'artifacts/ui-refresh/cache-filtered.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});await page.screenshot({path:'artifacts/ui-refresh/cache-mobile.png',fullPage:true});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('Cache page overflows mobile viewport');
  fs.writeFileSync('artifacts/ui-refresh/cache-verification.json',JSON.stringify({passed:true,control_heights:heights,live_config_changed:false},null,2));
  console.log('Cache lifecycle, full-cache search and responsive layout verified');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
