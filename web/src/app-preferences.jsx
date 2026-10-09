import {FieldLabel,FormHint} from './form-fields';
import React,{useEffect,useState} from 'react';
import {useT,translate,valueLabel} from './i18n';
import {notificationSelected} from './notification-errors';

const defaults={automaticCheck:true,automaticDownload:false,automaticInstall:false,assistantReady:false,assistantSupported:false,updatePrompts:true,checkIntervalHours:12,desktopNotifications:false,notificationSound:true};
export function nativeApp(){return Boolean(window.webkit?.messageHandlers?.routepilotPreferences)}
export function appPreferences(){
 if(nativeApp())return {...defaults,...window.routepilotNativePreferences};
 try{return {...defaults,...JSON.parse(localStorage.getItem('routepilot.appPreferences')||'{}')}}catch{return {...defaults}}
}
async function savePreferences(value,onWarning){
 if(nativeApp()){
  if(window.routepilotNativePreferences?.preferencesSaveAcknowledgement){
   const requestID=globalThis.crypto?.randomUUID?.()||Date.now().toString(36)+'-'+Math.random().toString(36).slice(2);
   await new Promise((resolve,reject)=>{
    const cleanup=()=>{clearTimeout(timer);window.removeEventListener('routepilot-preferences-saved',completed)};
    const completed=e=>{if(e.detail?.requestID!==requestID)return;cleanup();e.detail.ok?resolve():reject(new Error('应用偏好保存失败'))};
    const timer=setTimeout(()=>{cleanup();reject(new Error('应用偏好保存结果未确认，请重试'))},10000);
    window.addEventListener('routepilot-preferences-saved',completed);
    try{window.webkit.messageHandlers.routepilotPreferences.postMessage({...value,_requestID:requestID})}catch(e){cleanup();reject(e)}
   });
  }else{
   window.webkit.messageHandlers.routepilotPreferences.postMessage(value);
   window.routepilotNativePreferences=value;
  }
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
export function AppPreferencesPanel({kind,notify}){
 const t=useT(),[draft,setDraft]=useState(appPreferences),[busy,setBusy]=useState(false);
 useEffect(()=>{const changed=()=>setDraft(appPreferences());window.addEventListener('routepilot-preferences-changed',changed);return()=>window.removeEventListener('routepilot-preferences-changed',changed)},[]);
 const update=kind==='updates';
 const save=async e=>{e.preventDefault();setBusy(true);try{await savePreferences(draft,warning=>notify(warning,true));notify(t('应用偏好已保存'))}catch(e){notify(e.message,true)}finally{setBusy(false)}};
 const checkbox=(key,label,disabled=false)=><label className="check" key={key}><input type="checkbox" checked={draft[key]} disabled={disabled} onChange={e=>setDraft({...draft,[key]:e.target.checked})}/>{t(label)}</label>;
 return <section className="panel"><h2>{t(update?'软件更新':'本机系统通知')}</h2><p>{t(update?'这些选项控制本机 RoutePilot App。启用更新助手后可自动安装，远程后台仍需单独升级。':'App 可接收本机和当前查看的远程后端异常。浏览器通知需要页面保持打开，后台 Webhook 不受页面影响。')}</p><form onSubmit={save}><div className="form-grid">{update?<>{checkbox('automaticCheck','自动检查新版本',!nativeApp())}{checkbox('automaticDownload','自动下载并校验安装包',!nativeApp()||!draft.automaticCheck)}{checkbox('automaticInstall','自动安装新版本',!nativeApp()||!draft.automaticCheck||!draft.assistantReady)}{checkbox('updatePrompts','发现新版本时提示',!nativeApp())}<FieldLabel label={t('自动检查间隔（小时）')} required={nativeApp()&&draft.automaticCheck}><input type="number" min="1" max="168" disabled={!nativeApp()||!draft.automaticCheck} value={draft.checkIntervalHours} onChange={e=>setDraft({...draft,checkIntervalHours:e.target.value===''?'':Number(e.target.value)})}/></FieldLabel></>:<>{checkbox('desktopNotifications','启用本机系统通知')}{checkbox('notificationSound','通知声音',!draft.desktopNotifications)}</>}</div>{update&&!nativeApp()&&<p>{t('自动检查和下载需要在 RoutePilot App 内设置。')}</p>}{update&&nativeApp()&&<p>{t(!draft.assistantSupported?'更新 App 后可启用免密码升级和自动安装。':draft.assistantReady?'更新助手已启用，后续升级无需再次输入管理员密码。自动安装会短暂重启本机服务。':'启用更新助手需要一次管理员授权，之后升级无需再次输入管理员密码。自动安装默认关闭。')}</p>}<div className="toolbar"><button className="primary" disabled={busy||update&&!nativeApp()}>{t('保存应用偏好')}</button>{update&&nativeApp()&&<>{draft.assistantSupported&&!draft.assistantReady&&<button type="button" onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('enableAssistant')}>{t('启用更新助手')}</button>}{draft.assistantSupported&&draft.assistantReady&&<button type="button" onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('disableAssistant')}>{t('停用更新助手')}</button>}<button type="button" onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('check')}>{t('立即检查更新')}</button><button type="button" onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('install')}>{t('在线升级')}</button></>}</div></form></section>
}

// Per-backend alarm state survives Workspace switches during this page session.
const alarms=new Map();
export function observeServices({scope,name,health,settings,remote,backendID}){
 if(nativeApp()){
  if(remote)window.webkit.messageHandlers.routepilotServiceHealth?.postMessage({scope,name,health,settings,backendID});
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
 if('Notification' in window&&Notification.permission==='granted')for(const [items,title,suffix] of [[down,'服务异常','down'],[up,'服务已恢复','up']])if(items.length)new Notification(`RoutePilot · ${name} · ${translate(title)}`,{body:items.sort().map(valueLabel).join('、'),tag:`${scope}:${suffix}`,silent:!appPreferences().notificationSound});
}
