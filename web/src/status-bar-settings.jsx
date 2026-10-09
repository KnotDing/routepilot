import React,{useEffect,useState} from 'react';
import {FieldLabel} from './form-fields';
import {useT} from './i18n';
import {appPreferences,nativeApp,savePreferences} from './app-preferences';
export const statusBarDefaults={animation:true,display:'none',metric:'queries_per_second',health:'Surge API',healthValue:'latency_ms'};
export function StatusBarSettings({notify}){
 const t=useT(),[preferences,setPreferences]=useState(appPreferences),[busy,setBusy]=useState(false);
 const draft={...statusBarDefaults,...preferences.statusBar},supported=nativeApp();
 useEffect(()=>{const changed=()=>setPreferences(appPreferences());window.addEventListener('routepilot-preferences-changed',changed);return()=>window.removeEventListener('routepilot-preferences-changed',changed)},[]);
 const set=(key,value)=>setPreferences({...preferences,statusBar:{...draft,[key]:value}});
 const save=async e=>{e.preventDefault();setBusy(true);try{await savePreferences({...preferences,statusBar:draft});notify(t('应用偏好已保存'))}catch(e){notify(e.message,true)}finally{setBusy(false)}};
 const select=(key,label,options)=><FieldLabel label={t(label)} required><select value={draft[key]} onChange={e=>set(key,e.target.value)}>{Object.entries(options).map(([key,label])=><option key={key} value={key}>{t(label)}</option>)}</select></FieldLabel>;
 return <section className="panel"><h2>{t('状态栏设置')}</h2><p>{t('设置保存在本机 App，显示本机后台的数据；切换远程管理页面不会改变数据来源。')}</p>{!supported&&<p className="notice">{t('请在 RoutePilot App 中配置状态栏。')}</p>}<form onSubmit={save}><fieldset disabled={!supported||busy} className="plain-fieldset"><div className="option-grid"><label className="check"><input type="checkbox" checked={draft.animation} onChange={e=>set('animation',e.target.checked)}/>{t('启用图标动画')}</label></div><p>{t('以当前速度为最低速度，DNS 每秒请求数越高越快，最高为四倍；遵循 macOS 减少动态效果设置。')}</p><div className="form-grid">
 {select('display','图标旁显示',{none:'不显示数据',metric:'指标数值',health_state:'健康检查状态',health_value:'健康检查数值'})}
 {draft.display==='metric'&&select('metric','显示指标',{queries_per_second:'每秒 DNS 请求数',queries:'DNS 请求总数',direct:'DIRECT 请求总数',surge:'SURGE 请求总数',reject:'REJECT 请求总数',errors:'DNS 错误总数',bgp_advertised:'BGP 已发布路由数',uptime_seconds:'后台运行秒数'})}
 {['health_state','health_value'].includes(draft.display)&&<FieldLabel label={t('健康检查名称')} required hint={t('填写健康状态页面中的组件名称；未找到时显示 —。')}><input list="status-bar-health" maxLength="256" value={draft.health} onChange={e=>set('health',e.target.value)}/></FieldLabel>}
 {draft.display==='health_value'&&select('healthValue','健康检查数值',{latency_ms:'探测耗时（毫秒）',failures:'连续失败次数',successes:'连续成功次数'})}
 </div><datalist id="status-bar-health">{(window.routepilotNativePreferences?.statusBarHealth||['Surge API','Surge CLI','Surge Fake-IP DNS','Real DNS','Controller','DNS listener','BGP engine']).map(name=><option key={name} value={name}/>)}</datalist><div className="toolbar"><button className="primary" disabled={busy||!supported}>{t(busy?'正在保存…':'保存状态栏设置')}</button></div></fieldset></form></section>;
}
