let csrf='',backend='',backendURL='',generation=0;
const pending=new Set();
export function setCSRF(value){if(!backend)csrf=value||''}
export function setLocalCSRF(value){csrf=value||''}
export function isRemote(){return Boolean(backend)}
export function selectBackend(id='',url=''){
 backend=id;backendURL=url;generation++;
 for(const controller of pending)controller.abort();pending.clear();
}
async function fetchJSON(url,method,body,selected,extraHeaders={}){
 const revision=generation,controller=new AbortController();pending.add(controller);
 try{
  const r=await fetch(url,{method,signal:controller.signal,credentials:'same-origin',headers:{...extraHeaders,'Content-Type':'application/json',...(method==='GET'?{}:{'X-CSRF-Token':csrf})},...(body===undefined?{}:{body:JSON.stringify(body)})});
  if(selected&&revision!==generation)throw new DOMException('管理后端已切换','AbortError');
  if(!r.ok){if(selected&&backend&&r.status===401&&r.headers?.get('X-RoutePilot-Backend')!=='remote')window.dispatchEvent(new Event('routepilot-local-session-expired'));const text=(await r.text()).trim();let msg=text,detail;try{detail=JSON.parse(text);msg=detail.error}catch{};const e=new Error(msg||`HTTP ${r.status}`);e.status=r.status;if(typeof detail?.code==='string')e.code=detail.code;if(Array.isArray(detail?.conflicts))e.conflicts=detail.conflicts;throw e}
  const value=await r.json();
  if(selected&&revision!==generation)throw new DOMException('管理后端已切换','AbortError');
  return value;
 }finally{pending.delete(controller)}
}
export function localRequest(path,method='GET',body){return fetchJSON('/api/v1/'+path,method,body,false)}
const deferredPaths=new Set(['dhcp/check','bgp/routes/import-url','dns/hosts/import-surge','routing/verify','test/domain','dns/cache/flush','updates/check']);
let operationSequence=0;
export async function request(path,method='GET',body,headers){
 const revision=generation,base='/api/v1/'+(backend?'management/relay/'+encodeURIComponent(backend)+'/':'');
 const scopedHeaders=backend&&backendURL?{'X-RoutePilot-Expected-Backend':backendURL}:{};
 const deferred=deferredPaths.has(path),token=deferred?String(++operationSequence):'';
 const update=phase=>window.dispatchEvent(new CustomEvent('routepilot-operation',{detail:{token,path,phase}}));
 if(deferred)update('submitting');
 try{
  const value=await fetchJSON(base+path,method,body,true,{...(deferred?{Prefer:'respond-async'}:{}),...headers,...scopedHeaders});
  if(!deferred||!value.operation)return value;
  const id=value.operation.id;
  if(typeof id!=='string'||!id)throw new Error('后台任务编号缺失，请刷新后确认');
  update('running');
  const started=Date.now();
  while(true){
   if(revision!==generation)throw new DOMException('管理后端已切换','AbortError');
   const job=await fetchJSON(base+'operations/status?id='+encodeURIComponent(id),'GET',undefined,true,scopedHeaders);
   if(job.id!==id)throw new Error('后台任务状态不匹配，请刷新后确认');
   if(job.state==='succeeded')return job.result;
   if(job.state==='failed')throw new Error(job.error||'后台任务失败');
   if(Date.now()-started>5*60*1000)throw new Error('后台任务仍在运行，请稍后查看结果；不要重复提交');
   await new Promise(resolve=>setTimeout(resolve,1000));
  }
 }finally{if(deferred)update('finished')}
}
export async function requestOptional(path,fallback){
 try{return await request(path)}catch(e){if((backend&&e.status===404)||(path==='dns/client-settings'&&[404,501].includes(e.status)))return fallback;throw e}
}
export function pretty(v){return v==null?'—':typeof v==='object'?JSON.stringify(v):String(v)}

// Reconnect independently of the save acknowledgement. Failed activation keeps
// the current working listener and is reported by ConfigurationApplyNotice.
export async function reconnectAfterApplication(url){
 const revision=generation;
 let applying=false;
 for(let attempts=0;attempts<300&&revision===generation;attempts++){
  try{
   const state=await request('configuration/status');
   if(state.state==='failed')return;
   if(state.state==='succeeded'){
    await new Promise(resolve=>setTimeout(resolve,500));
    if(revision===generation)location.assign(url);
    return;
   }
   applying=true;
  }catch(e){
   if(e.name==='AbortError'||e.status===401||e.status===403)return;
   // A successfully switched listener closes the old origin. Preflight failure
   // leaves it serving configuration/status with state=failed.
   if(applying){await new Promise(resolve=>setTimeout(resolve,1000));if(revision===generation)location.assign(url);return}
  }
  await new Promise(resolve=>setTimeout(resolve,1000));
 }
}
