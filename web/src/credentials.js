// The native vault owns passwords. Page events contain only save results or
// authenticated sessions, never the saved username/password pair.
export function remoteCredentialsSupported(){
 return Boolean(window.webkit?.messageHandlers?.routepilotCredentials&&window.routepilotNativePreferences?.remoteCredentialsSupported);
}
function invoke(action,backend,credential){
 const supported=backend?remoteCredentialsSupported():Boolean(window.webkit?.messageHandlers?.routepilotCredentials&&window.routepilotNativePreferences?.localCredentialsSupported);
 if(!supported)return Promise.resolve({unsupported:true});
 const requestID=globalThis.crypto?.randomUUID?.()||Date.now().toString(36)+'-'+Math.random().toString(36).slice(2);
 return new Promise((resolve,reject)=>{
  const finish=()=>{clearTimeout(timer);window.removeEventListener('routepilot-credentials-result',received)};
  const received=event=>{
   const result=event.detail;
   if(result?.requestID!==requestID||backend&&(result.backendID!==backend.id||result.backendURL!==backend.url))return;
   finish();
   if(result.localExpired){window.dispatchEvent(new Event('routepilot-local-session-expired'));reject(new Error('请重新登录本机服务'));return}
   if(result.error){let error=result.error;try{error=JSON.parse(error).error||error}catch{}reject(Object.assign(new Error(error),{retryable:result.retryable===true}));return}
   resolve(result);
  };
  const timer=setTimeout(()=>{finish();reject(new Error(backend?'远程登录凭证操作超时，请重试':'本机登录凭证操作超时，请重试'))},60000);
  window.addEventListener('routepilot-credentials-result',received);
  try{window.webkit.messageHandlers.routepilotCredentials.postMessage({action,requestID,...(backend?{backendID:backend.id,backendURL:backend.url}:{}),...credential})}
  catch(error){finish();reject(error)}
 });
}
export function restoreRemoteLogin(backend){return invoke('login',backend)}
export function restoreLocalLogin(){return invoke('local-login')}
export function rememberManagementBackend(backend){
 window.webkit?.messageHandlers?.routepilotCredentials?.postMessage({action:'select',backendID:backend?.id||'',backendURL:backend?.url||''});
}
export function saveRemoteCredential(backend,credential){return invoke('save',backend,credential)}
export function forgetRemoteCredential(backend){return invoke('forget',backend)}
