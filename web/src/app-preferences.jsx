import React,{useEffect,useState} from 'react';
import {useT,translate,valueLabel} from './i18n';
import {notificationSelected} from './notification-errors';

const defaults={automaticCheck:true,automaticDownload:false,updatePrompts:true,checkIntervalHours:12,desktopNotifications:false,notificationSound:true};
export function nativeApp(){return Boolean(window.webkit?.messageHandlers?.routepilotPreferences)}
export function appPreferences(){
 if(nativeApp())return {...defaults,...window.routepilotNativePreferences};
 try{return {...defaults,...JSON.parse(localStorage.getItem('routepilot.appPreferences')||'{}')}}catch{return {...defaults}}
}
async function savePreferences(value){
 if(nativeApp()){
  window.routepilotNativePreferences=value;
  window.webkit.messageHandlers.routepilotPreferences.postMessage(value);
 }else{
  if(value.desktopNotifications){
   if(!('Notification' in window))throw new Error('此浏览器不支持系统通知');
   if(await Notification.requestPermission()!=='granted')throw new Error('通知未授权');
  }
  localStorage.setItem('routepilot.appPreferences',JSON.stringify(value));
 }
 window.dispatchEvent(new Event('routepilot-preferences-changed'));
}
export function AppPreferencesPanel({kind,notify}){
 const t=useT(),[draft,setDraft]=useState(appPreferences),[busy,setBusy]=useState(false);
 useEffect(()=>{const changed=()=>setDraft(appPreferences());window.addEventListener('routepilot-preferences-changed',changed);return()=>window.removeEventListener('routepilot-preferences-changed',changed)},[]);
 const update=kind==='updates';
 const save=async e=>{e.preventDefault();setBusy(true);try{await savePreferences(draft);notify(t('应用偏好已保存'))}catch(e){notify(e.message,true)}finally{setBusy(false)}};
 const checkbox=(key,label,disabled=false)=><label className="check" key={key}><input type="checkbox" checked={draft[key]} disabled={disabled} onChange={e=>setDraft({...draft,[key]:e.target.checked})}/>{t(label)}</label>;
 return <section className="panel"><h2>{t(update?'软件更新':'本机系统通知')}</h2><p>{t(update?'这些选项控制本机 RoutePilot App，不会自动安装更新或修改远程后台。':'App 可接收本机和当前查看的远程后端异常。浏览器通知需要页面保持打开，后台 Webhook 不受页面影响。')}</p><form onSubmit={save}><div className="form-grid">{update?<>{checkbox('automaticCheck','自动检查新版本',!nativeApp())}{checkbox('automaticDownload','自动下载并校验安装包',!nativeApp()||!draft.automaticCheck)}{checkbox('updatePrompts','发现新版本时提示',!nativeApp())}<label>{t('自动检查间隔（小时）')}<input type="number" min="1" max="168" disabled={!nativeApp()||!draft.automaticCheck} value={draft.checkIntervalHours} onChange={e=>setDraft({...draft,checkIntervalHours:Number(e.target.value)})}/></label></>:<>{checkbox('desktopNotifications','启用本机系统通知')}{checkbox('notificationSound','通知声音',!draft.desktopNotifications)}</>}</div>{update&&!nativeApp()&&<p>{t('自动检查和下载需要在 RoutePilot App 内设置。')}</p>}<div className="toolbar"><button className="primary" disabled={busy||update&&!nativeApp()}>{t('保存应用偏好')}</button>{update&&nativeApp()&&<><button type="button" onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('check')}>{t('立即检查更新')}</button><button type="button" onClick={()=>window.webkit.messageHandlers.routepilotUpdateAction.postMessage('install')}>{t('在线升级')}</button></>}</div></form></section>
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
