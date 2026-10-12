import React,{createContext,useContext,useEffect,useRef,useState} from 'react';
import {useT,message} from './i18n';
import {appPreferences,nativeApp,savePreferences} from './app-preferences';

const storageKey='routepilot.monitoringPreferences',changedEvent='routepilot-monitoring-preferences-changed';
export const monitoringCards=[['received','DNS 流入总数'],['upstream','上游处理请求'],['completed','已生成响应'],['inflight','处理中'],['unconfirmed','重启前未确认'],['latency','平均处理耗时'],['dnsFlow','DNS 请求流向'],['clientFlow','客户端策略流向'],['policies','策略命中与占比'],['sources','请求来源占比'],['trend','DNS 流入与响应速率趋势'],['decisions','分流决策占比'],['rcodes','DNS 响应码占比']];
const cardIDs=new Set(monitoringCards.map(([key])=>key));
const normalize=value=>({hiddenCards:Array.isArray(value?.hiddenCards)?[...new Set(value.hiddenCards.filter(key=>cardIDs.has(key)))]:[],protectSensitive:value?.protectSensitive===true});
export function readMonitoringPreferences(){
 try{
  if(nativeApp()){if(!window.routepilotNativePreferences?.monitoring)throw Error('更新 App 后可保存大屏监控设置。');return {...normalize(appPreferences().monitoring),error:''}}
  return {...normalize(JSON.parse(localStorage.getItem(storageKey)||'{}')),error:''};
 }catch(error){return {hiddenCards:[],protectSensitive:true,error:error.message}}
}
export async function saveMonitoringPreferences(value){
 if(nativeApp()){if(!window.routepilotNativePreferences?.monitoring)throw Error('更新 App 后可保存大屏监控设置。');await savePreferences({...appPreferences(),monitoring:normalize(value)})}
 else localStorage.setItem(storageKey,JSON.stringify(normalize(value)));
 window.dispatchEvent(new Event(changedEvent));
}
export function useMonitoringPreferences(){
 const [value,setValue]=useState(readMonitoringPreferences);
 useEffect(()=>{const changed=event=>{if(event.type!=='storage'||event.key===storageKey||event.key===null)setValue(readMonitoringPreferences())};window.addEventListener(changedEvent,changed);window.addEventListener('routepilot-preferences-changed',changed);window.addEventListener('storage',changed);return()=>{window.removeEventListener(changedEvent,changed);window.removeEventListener('routepilot-preferences-changed',changed);window.removeEventListener('storage',changed)}},[]);
 return value;
}
export function MonitoringSettings({notify}){
 const t=useT(),saved=useMonitoringPreferences(),[draft,setDraft]=useState(saved),[busy,setBusy]=useState(false),dirty=useRef(false);
 useEffect(()=>{if(!dirty.current)setDraft(saved)},[saved]);
 const change=next=>{dirty.current=true;setDraft(next)};
 const save=async event=>{event.preventDefault();if(busy)return;setBusy(true);try{await saveMonitoringPreferences(draft);dirty.current=false;setDraft(readMonitoringPreferences());notify(t('大屏监控设置已保存'))}catch(error){notify(t('大屏监控设置保存失败：{error}',{error:message(error.message)}),true)}finally{setBusy(false)}};
 return <section className="panel monitoring-settings"><h2>{t('大屏监控')}</h2><p>{t('这些显示设置保存在当前管理界面，重新打开后保留。')}</p>{saved.error&&<p role="alert" className="notice error">{t('大屏监控设置读取失败：{error}',{error:message(saved.error)})}</p>}<form onSubmit={save}><fieldset disabled={busy}>
  <h3>{t('显示的卡片')}</h3><div className="option-grid">{monitoringCards.map(([key,label])=><label className="check" key={key}><input type="checkbox" checked={!draft.hiddenCards.includes(key)} onChange={event=>change({...draft,hiddenCards:event.target.checked?draft.hiddenCards.filter(id=>id!==key):[...draft.hiddenCards,key]})}/>{t(label)}</label>)}</div>
  <h3>{t('敏感数据保护')}</h3><label className="check"><input type="checkbox" checked={draft.protectSensitive} onChange={event=>change({...draft,protectSensitive:event.target.checked})}/>{t('启用敏感数据保护')}</label><p>{t('使用匿名标识替代客户端 IP、备注及策略名称，隐藏后端名称和地址；悬停提示与明细同样保护。')}</p>
  <div className="toolbar"><button className="primary" disabled={busy}>{t(busy?'正在保存…':'保存设置')}</button></div>
 </fieldset></form></section>;
}

export const MonitoringDisplayContext=createContext(null);
export function useMonitoringDisplay(){
 const t=useT();return useContext(MonitoringDisplayContext)||{protected:false,source:key=>key,policy:name=>name,redact:value=>value,t};
}
// Display aliases preserve source identities and counts in data requests. They
// never turn a cached response into traffic sent to an upstream.
export function createMonitoringDisplay(protectedMode,t,aliases,sourceKeys=[],sensitive=[]){
 const alias=(map,key,label)=>{if(!map.has(key))map.set(key,map.size+1);return t(label,{number:map.get(key)})};
 for(const key of [...new Set(sourceKeys)].sort())if(!['unknown','other','legacy','remaining-sources'].includes(key))alias(aliases.sources,key,'客户端 {number}');
 const source=key=>({'unknown':t('无法识别的源'),other:t('超出源明细容量'),legacy:t('启用前未采集来源'),'remaining-sources':t('其余来源')})[key]||alias(aliases.sources,key,'客户端 {number}');
 const redact=value=>{
  if(!protectedMode)return value;
  let text=String(value||'');for(const secret of [...new Set(sensitive.filter(Boolean))].sort((a,b)=>b.length-a.length))text=text.split(secret).join(t('敏感信息已隐藏'));
  return text.replace(/https?:\/\/[^\s<>]+/gi,t('地址已隐藏')).replace(/\b(?:\d{1,3}\.){3}\d{1,3}\b/g,t('地址已隐藏')).replace(/(^|[^\w:])(?:[a-f\d]{0,4}:){2,}[a-f\d:.]*(?:%[\w-]+)?/gi,(_,prefix)=>prefix+t('地址已隐藏'));
 };
 return {protected:protectedMode,source:key=>protectedMode?source(key):key,policy:(name,final=false)=>protectedMode?alias(final?aliases.finals:aliases.policies,name,final?'最终策略 {number}':'策略 {number}'):name,redact,t};
}
