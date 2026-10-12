import {FieldLabel,FormHint} from './form-fields';
import React,{useEffect,useRef,useState} from 'react';
import {createPortal} from 'react-dom';
import {useT,translate,valueLabel,message} from './i18n';
import {notificationSelected} from './notification-errors';
import {DownloadProgress} from './download-progress';

const defaults={assistantReady:false,updatePrompts:true,updateAvailable:false,updateVersion:'',updateDownloading:false,updateDownloadError:'',updateAutomationError:'',githubAccessTokenConfigured:false,desktopNotifications:false,notificationSound:true};
export function nativeApp(){return Boolean(window.webkit?.messageHandlers?.routepilotPreferences)}
export function appPreferences(){
 if(nativeApp())return {...defaults,...window.routepilotNativePreferences};
 try{return {...defaults,...JSON.parse(localStorage.getItem('routepilot.appPreferences')||'{}')}}catch{return {...defaults}}
}
export async function savePreferences(value,onWarning){
 if(nativeApp()){
  const requestID=globalThis.crypto?.randomUUID?.()||Date.now().toString(36)+'-'+Math.random().toString(36).slice(2);
  await new Promise((resolve,reject)=>{
   const cleanup=()=>{clearTimeout(timer);window.removeEventListener('routepilot-preferences-saved',completed)};
   const completed=e=>{if(e.detail?.requestID!==requestID)return;cleanup();e.detail.ok?resolve():reject(Object.assign(new Error(e.detail.error||'应用偏好保存失败'),{code:e.detail.code,conflicts:e.detail.conflicts}))};
   const timer=setTimeout(()=>{cleanup();reject(new Error('应用偏好保存结果未确认，请重试'))},10000);
   window.addEventListener('routepilot-preferences-saved',completed);
   try{window.webkit.messageHandlers.routepilotPreferences.postMessage({...value,_requestID:requestID})}catch(e){cleanup();reject(e)}
  });
 }else{
  localStorage.setItem('routepilot.appPreferences',JSON.stringify(value));
  if(value.desktopNotifications){
   const warn=text=>setTimeout(()=>onWarning?.(text),0);
   try{
    if(!('Notification' in window))throw new Error('此浏览器不支持系统通知');
    Promise.resolve(Notification.requestPermission()).then(permission=>{if(permission!=='granted')warn('通知未授权')}).catch(e=>warn(e.message));
   }catch(e){warn(e.message)}
  }
 }
 window.dispatchEvent(new Event('routepilot-preferences-changed'));
}
// Native status and other preference panels can change while a form is open.
// Refresh metadata and saved fields, retaining only this form's unsaved edits.
export function usePreferenceDraft(keys){
 const [draft,setValue]=useState(appPreferences),owned=useRef(keys),dirty=useRef(new Set());
 useEffect(()=>{
  const changed=()=>setValue(previous=>{const next=appPreferences();for(const key of dirty.current)next[key]=previous[key];return next});
  window.addEventListener('routepilot-preferences-changed',changed);
  window.addEventListener('routepilot-statusbar-health-changed',changed);
  return()=>{window.removeEventListener('routepilot-preferences-changed',changed);window.removeEventListener('routepilot-statusbar-health-changed',changed)};
 },[]);
 const setDraft=value=>setValue(previous=>{const next=typeof value==='function'?value(previous):value;for(const key of owned.current)if(JSON.stringify(next[key])!==JSON.stringify(previous[key]))dirty.current.add(key);return next});
 const saved=()=>{dirty.current.clear();setValue(appPreferences())};
 return [draft,setDraft,saved];
}
export function GitHubAccessTokenControls({preferences,notify}){
 const t=useT(),[token,setToken]=useState(''),[tokenBusy,setTokenBusy]=useState(false);
 const saveToken=async(action='save')=>{
  if(action==='save'&&!token.trim()){return}
  setTokenBusy(true);
  const requestID=globalThis.crypto?.randomUUID?.()||Date.now().toString(36)+'-'+Math.random().toString(36).slice(2);
  try{
   await new Promise((resolve,reject)=>{
    const cleanup=()=>{clearTimeout(timer);window.removeEventListener('routepilot-github-token-saved',completed)};
    const completed=e=>{if(e.detail?.requestID!==requestID)return;cleanup();e.detail.ok?resolve():reject(new Error(e.detail.error||t('GitHub Access token 保存失败')))};
    const timer=setTimeout(()=>{cleanup();reject(new Error(t('GitHub Access token 保存结果未确认，请重试')))},10000);
    window.addEventListener('routepilot-github-token-saved',completed);
    try{window.webkit.messageHandlers.routepilotGitHubToken.postMessage({action,token:action==='save'?token.trim():'',requestID})}catch(e){cleanup();reject(e)}
   });
   setToken('');window.dispatchEvent(new Event('routepilot-preferences-changed'));
   notify?.(t(action==='save'?'GitHub Access token 已保存到 macOS 钥匙串':'GitHub Access token 已从 macOS 钥匙串删除'));
  }catch(e){notify?.(e.message,true)}finally{setTokenBusy(false)}
 };
 return <div className="github-access-token-controls">
   {preferences.githubAccessTokenConfigured&&<p role="status">{t('GitHub Access token 已配置')}</p>}
   <FieldLabel label={t('GitHub Access token')}><input type="password" autoComplete="new-password" maxLength={255} value={token} disabled={tokenBusy} onChange={e=>setToken(e.target.value)} placeholder={t(preferences.githubAccessTokenConfigured?'输入新 token 以替换已保存的 token':'粘贴 GitHub 只读 Access token')}/></FieldLabel>
   <div className="toolbar">
    <button type="button" className="primary" disabled={tokenBusy||!token.trim()} onClick={()=>saveToken('save')}>{t(tokenBusy?'处理中…':'保存 Access token')}</button>
    {preferences.githubAccessTokenConfigured&&<button type="button" disabled={tokenBusy} onClick={()=>saveToken('clear')}>{t('删除已保存的 token')}</button>}
   </div>
  </div>;
}
// These controls only update the App hosting a local management window.
// Remote connections use BackendUpdate and never send native update actions.
export function LocalAppUpdateControls({preferences,actionsTarget=null}){
 const t=useT();
 const installing=Boolean(preferences.updatePhase),busy=preferences.updateChecking||preferences.updateDownloading||installing;
 const phaseLabels={preparing:'正在准备更新…',downloading:'正在下载完整应用更新',validating:'正在校验完整应用更新',restarting:'正在重启后台，请稍候'};
 const actions=<><button type="button" disabled={busy} onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('check')}>{t(preferences.updateChecking?'正在检查更新…':'立即检查更新')}</button>{preferences.updateAvailable&&<button type="button" disabled={busy} onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('installAvailable')}>{t(preferences.updateDownloading?'正在下载更新…':'立即安装')}</button>}</>;
 return <div className="update-status">
  {preferences.updateAvailable&&<p>{t('已发现可用更新')}：{preferences.updateVersion}</p>}
  {preferences.updateDownloadError&&<p role="alert">{t('更新包下载失败')}：{preferences.updateDownloadError}</p>}
  {preferences.updateAutomationError&&<p role="alert">{t('自动更新失败')}：{message(preferences.updateAutomationError)}</p>}
  {installing&&<p role="status">{t(phaseLabels[preferences.updatePhase]||'正在准备更新…')}</p>}
  {preferences.updateDownloading&&<DownloadProgress bytes={preferences.updateDownloadBytes} total={preferences.updateDownloadTotal}/>}
  {actionsTarget?createPortal(actions,actionsTarget):<div className="toolbar">{actions}</div>}
 </div>;
}
export function AppPreferencesPanel({notify}){
 const t=useT(),[draft,setDraft,saved]=usePreferenceDraft(['desktopNotifications','notificationSound']),[busy,setBusy]=useState(false);
 const save=async e=>{e.preventDefault();setBusy(true);try{await savePreferences({...appPreferences(),desktopNotifications:draft.desktopNotifications,notificationSound:draft.notificationSound},warning=>notify(warning,true));saved();notify(t('应用偏好已保存'))}catch(e){notify(e.message,true)}finally{setBusy(false)}};
 const checkbox=(key,label,disabled=false)=><label className="check" key={key}><input type="checkbox" checked={draft[key]} disabled={disabled||busy} onChange={e=>setDraft({...draft,[key]:e.target.checked})}/>{t(label)}</label>;
 return <section className="panel">
  <h2>{t('本机系统通知')}</h2>
  <p>{t('App 可接收本机和当前查看的远程后端异常。浏览器通知需要页面保持打开，后台 Webhook 不受页面影响。')}</p>
  <form onSubmit={save}>
   <div className="option-grid">{checkbox('desktopNotifications','启用本机系统通知')}{checkbox('notificationSound','通知声音',!draft.desktopNotifications)}</div>
   <div className="toolbar"><button className="primary" disabled={busy}>{t('保存应用偏好')}</button></div>
  </form>
 </section>;
}

// Per-backend alarm state survives Workspace switches during this page session.
const alarms=new Map();
export function observeServices({scope,health,settings,remote,backendID}){
 if(nativeApp()){
  if(remote)window.webkit.messageHandlers.routepilotServiceHealth?.postMessage({scope,health,settings,backendID});
  else window.webkit.messageHandlers.routepilotServiceHealth?.postMessage({clear:true});
  return;
 }
 if(!appPreferences().desktopNotifications||!settings?.enabled){alarms.delete(scope);return}
 const previous=alarms.get(scope)||new Map(),seen=new Set(),now=Date.now(),cooldown=Math.max(0,settings.cooldown_seconds??300)*1000,down=[],up=[];
 for(const component of health){
  const key=component.name,state=component.state;
  if(!notificationSelected(key,settings)||state==='DISABLED')continue;
  seen.add(key);const alarm=previous.get(key)||{down:false,last:0};
  if(state==='DOWN'&&(!alarm.down||cooldown>0)&&now-alarm.last>=cooldown){down.push(key);alarm.down=true;alarm.last=now}
  else if(state==='UP'){if(alarm.down&&settings.recovery)up.push(key);alarm.down=false}
  previous.set(key,alarm);
 }
 for(const key of previous.keys())if(!seen.has(key))previous.delete(key);
 alarms.set(scope,previous);
 if('Notification' in window&&Notification.permission==='granted')for(const [items,title,suffix] of [[down,'服务异常','down'],[up,'服务已恢复','up']])if(items.length)new Notification(`RoutePilot · ${translate(title)}`,{body:items.sort().map(valueLabel).join('、'),tag:`${scope}:${suffix}`,silent:!appPreferences().notificationSound});
}
