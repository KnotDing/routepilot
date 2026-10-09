// The native vault owns passwords. Page events contain only save results or
// authenticated sessions, never the saved username/password pair.
export function remoteCredentialsSupported(){
 return Boolean(window.webkit?.messageHandlers?.routepilotCredentials&&window.routepilotNativePreferences?.remoteCredentialsSupported);
}
function invoke(action,backend,credential){
 if(!remoteCredentialsSupported())return Promise.resolve({unsupported:true});
 const requestID=crypto.randomUUID();
 return new Promise((resolve,reject)=>{
  const finish=()=>{clearTimeout(timer);window.removeEventListener('routepilot-credentials-result',received)};
  const received=event=>{
   const result=event.detail;
   if(result?.requestID!==requestID||result.backendID!==backend.id||result.backendURL!==backend.url)return;
   finish();
   if(result.localExpired){window.dispatchEvent(new Event('routepilot-local-session-expired'));reject(new Error('请重新登录本机服务'));return}
   if(result.error){let error=result.error;try{error=JSON.parse(error).error||error}catch{}reject(new Error(error));return}
   resolve(result);
  };
  const timer=setTimeout(()=>{finish();reject(new Error('远程登录凭证操作超时，请重试'))},60000);
  window.addEventListener('routepilot-credentials-result',received);
  try{window.webkit.messageHandlers.routepilotCredentials.postMessage({action,requestID,backendID:backend.id,backendURL:backend.url,...credential})}
  catch(error){finish();reject(error)}
 });
}
export function restoreRemoteLogin(backend){return invoke('login',backend)}
export function saveRemoteCredential(backend,credential){return invoke('save',backend,credential)}
export function forgetRemoteCredential(backend){return invoke('forget',backend)}
