let csrf='',backend='',generation=0;
const pending=new Set();
export function setCSRF(value){if(!backend)csrf=value||''}
export function setLocalCSRF(value){csrf=value||''}
export function isRemote(){return Boolean(backend)}
export function selectBackend(id=''){
 backend=id;generation++;
 for(const controller of pending)controller.abort();pending.clear();
}
async function fetchJSON(url,method,body,selected,extraHeaders={}){
 const revision=generation,controller=new AbortController();pending.add(controller);
 try{
  const r=await fetch(url,{method,signal:controller.signal,credentials:'same-origin',headers:{...extraHeaders,'Content-Type':'application/json',...(method==='GET'?{}:{'X-CSRF-Token':csrf})},...(body===undefined?{}:{body:JSON.stringify(body)})});
  if(selected&&revision!==generation)throw new DOMException('管理后端已切换','AbortError');
  if(!r.ok){if(selected&&backend&&r.status===401&&r.headers?.get('X-RoutePilot-Backend')!=='remote')window.dispatchEvent(new Event('routepilot-local-session-expired'));const text=(await r.text()).trim();let msg=text;try{msg=JSON.parse(text).error}catch{};const e=new Error(msg||`HTTP ${r.status}`);e.status=r.status;throw e}
  const value=await r.json();
  if(selected&&revision!==generation)throw new DOMException('管理后端已切换','AbortError');
  return value;
 }finally{pending.delete(controller)}
}
export function localRequest(path,method='GET',body){return fetchJSON('/api/v1/'+path,method,body,false)}
export function request(path,method='GET',body,headers){return fetchJSON('/api/v1/'+(backend?'management/relay/'+encodeURIComponent(backend)+'/':'')+path,method,body,true,headers)}
export function pretty(v){return v==null?'—':typeof v==='object'?JSON.stringify(v):String(v)}
