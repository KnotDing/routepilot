import React,{useEffect,useRef,useState} from 'react';
import {request} from './client';
import {useT} from './i18n';
import {currentLocale} from './locale';
import {useWindowVisible} from './window-visibility';
import {MonitoringFilters,emptyMonitoringFilters} from './monitoring-filters';
import {visibleSources} from './monitoring-data';
import {useMonitoringDisplay} from './monitoring-preferences';

export function sourceName(source,t,clients=[]){const name=clients.find(c=>c.ip===source)?.name; if(name)return `${name} (${source})`; return ({unknown:t('无法识别的源'),other:t('超出源明细容量'),legacy:t('启用前未采集来源')})[source]||source}
export function historyRange(snapshot,mode,range,from,to){
 if(mode==='history'&&range==='custom'){
  const start=new Date(from),end=new Date(to);
  if(!Number.isFinite(start.getTime())||!Number.isFinite(end.getTime())||end<=start||end-start>7*86400000)throw Error('请选择有效的时间范围，最长七天。');
  return {from:start.toISOString(),to:end.toISOString()};
 }
 const at=Date.parse(snapshot.observed_at),end=new Date(Math.floor(at/60000)*60000+60000),seconds=mode==='history'?Number(range):360;
 return {from:new Date(end.getTime()-seconds*1000).toISOString(),to:end.toISOString()};
}
export function historicalSamples(history){
 return (history?.points||[]).map((p,i)=>({at:Date.parse(p.to),incoming:p.rate_valid&&p.seconds>0?p.counts.received/p.seconds:null,outgoing:p.rate_valid&&p.seconds>0?p.counts.completed/p.seconds:null,breakBefore:i>0&&Date.parse(p.from)-Date.parse(history.points[i-1].to)>1000}));
}

export function useMonitoringData({snapshot,historyStatus,mode,range,from,to,source,filters=emptyMonitoringFilters}){
 const visible=useWindowVisible(),latest=useRef(snapshot),retry=useRef(()=>{}),[state,setState]=useState({history:null,selected:null,error:'',loading:false});
 latest.current=snapshot;
 const ready=Boolean(snapshot),supported=historyStatus?.supported===true,dimensions=snapshot?.dimensions_supported===true,filterKey=JSON.stringify(filters),key=JSON.stringify([mode,range,from,to,source,supported,dimensions,filters]);
 useEffect(()=>{
  if(!ready||!visible)return;
  let alive=true,busy=false;
  setState({key,history:null,selected:null,error:'',loading:true});
  const load=async()=>{
   if(busy)return;busy=true;
   try{
    const window=historyRange(latest.current,mode,range,from,to),query=new URLSearchParams({...window,...(source?{source}:{}),...Object.fromEntries(Object.entries(filters).filter(([,value])=>value))});
    const historyRequest=supported?request('dns/monitoring/history?'+query).catch(e=>{if(e.status===404||e.status===501)return null;throw e}):Promise.resolve(null);
    const selectedRequest=mode==='live'&&(source||dimensions)?request('dns/monitoring?'+new URLSearchParams({...source?{source}:{},...Object.fromEntries(Object.entries(filters).filter(([,value])=>value))})):Promise.resolve(null);
    const [historyResult,selectedResult]=await Promise.allSettled([historyRequest,selectedRequest]);
    if(selectedResult.status==='rejected')throw selectedResult.reason;
    if(historyResult.status==='rejected'&&mode==='history')throw historyResult.reason;
    const history=historyResult.status==='fulfilled'?historyResult.value:null,selected=selectedResult.value;
    if(history&&(!Array.isArray(history.points)||!history.summary))throw Error('历史监控响应格式无效。');
    if(alive)setState({key,history,selected,error:historyResult.status==='rejected'?historyResult.reason.message:'',loading:false});
   }catch(e){if(alive)setState({key,history:null,selected:null,error:e.message,loading:false})}finally{busy=false}
  };
  retry.current=load;load();
  const timer=mode==='live'?setInterval(load,3000):null;
  return()=>{alive=false;if(timer)clearInterval(timer)};
 },[ready,supported,dimensions,visible,mode,range,from,to,source,filterKey]);
 return {...(state.key===key?state:{history:null,selected:null,error:'',loading:ready}),reload:()=>retry.current()};
}

const localDateInput=date=>new Date(date.getTime()-date.getTimezoneOffset()*60000).toISOString().slice(0,16);
export function MonitoringControls({mode,onMode,range,onRange,from,onFrom,to,onTo,source,onSource,sources,snapshot,reload,clientSettings=[],filters=emptyMonitoringFilters,onFilters,filterOptions,dimensionsSupported=false}){
 const t=useT(),display=useMonitoringDisplay(),label=key=>display.protected?display.source(key):sourceName(key,t,clientSettings);
 sources=visibleSources(sources);
 const chooseRange=value=>{if(value==='custom'&&!from){const end=new Date(Math.floor(Date.parse(snapshot?.observed_at||new Date().toISOString())/60000)*60000+60000);onFrom(localDateInput(new Date(end-3600000)));onTo(localDateInput(end))}onRange(value)};
 return <div className="monitor-view-controls"><div className="tabs" role="group" aria-label={t('监控数据视图')}>{[['live','实时数据'],['history','历史数据']].map(([key,label])=><button key={key} aria-pressed={mode===key} className={mode===key?'selected':''} onClick={()=>onMode(key)}>{t(label)}</button>)}</div>
  <label>{t('请求来源')}<select aria-label={t('请求来源')} value={display.protected&&source?label(source):source} onChange={e=>onSource(display.protected?(source&&label(source)===e.target.value?source:sources.find(s=>label(s.source)===e.target.value)?.source||''):e.target.value)}><option value="">{t('全部来源')}</option>{sources.map(s=><option key={s.source} value={display.protected?label(s.source):s.source}>{label(s.source)}</option>)}{source&&!sources.some(s=>s.source===source)&&<option value={display.protected?label(source):source}>{label(source)}</option>}</select></label>
  <MonitoringFilters filters={filters} onChange={onFilters} options={filterOptions} supported={dimensionsSupported}/>
  {mode==='history'&&<><label>{t('历史时间范围')}<select aria-label={t('历史时间范围')} value={range} onChange={e=>chooseRange(e.target.value)}>{[['3600','最近一小时'],['21600','最近六小时'],['86400','最近一天'],['604800','最近七天'],['custom','自定义时间']].map(([key,label])=><option key={key} value={key}>{t(label)}</option>)}</select></label>{range==='custom'&&<><label>{t('开始时间')}<input type="datetime-local" aria-label={t('开始时间')} value={from} onChange={e=>onFrom(e.target.value)}/></label><label>{t('结束时间')}<input type="datetime-local" aria-label={t('结束时间')} value={to} onChange={e=>onTo(e.target.value)}/></label></>}<button onClick={reload}>{t('刷新历史')}</button></>}
 </div>;
}

export function SourceStatistics({sources,total,selected,onSelect,clientSettings=[]}){
 const t=useT(),display=useMonitoringDisplay(),label=key=>display.protected?display.source(key):sourceName(key,t,clientSettings),number=n=>n.toLocaleString(currentLocale()),hiddenLegacy=sources?.some(s=>s.source==='legacy'&&(s.counts.received>0||s.counts.completed>0));
 sources=visibleSources(sources);
 total=sources?.reduce((sum,s)=>sum+s.counts.received,0)||0;
 if(!Array.isArray(sources))return <div className="notice">{t('此后端未提供来源统计，请升级后端。')}</div>;
 return <section className="panel monitor-source-shares"><header className="section-heading"><h2>{t('请求来源占比')}</h2><p>{t('按各源流入请求数计算占比，点击来源查看该源的流向和策略命中。')}</p></header>{hiddenLegacy&&<p className="field-hint">{t('来源视图和占比分母均不包含未采集来源的旧请求。')}</p>}<div className="monitor-bars monitor-source-bars">{[...sources].sort((a,b)=>b.counts.received-a.counts.received||a.source.localeCompare(b.source)).map((s,i)=><div key={s.source} className={'monitor-bar-row '+(selected===s.source?'monitor-source-selected':'')}><span className="monitor-rank">{i+1}</span><button className="monitor-source-link monitor-policy-name" title={label(s.source)} aria-label={t('查看 {source} 的流向',{source:label(s.source)})} aria-pressed={selected===s.source} onClick={()=>onSelect(selected===s.source?'':s.source)}>{label(s.source)}</button><div className="monitor-bar-track"><span style={{width:`${total?s.counts.received/total*100:0}%`}}/></div><strong>{number(s.counts.received)}</strong><small>{total?`${(s.counts.received/total*100).toFixed(2)}%`:'—'}</small></div>)}</div>{!sources.length&&<div className="empty">{t('暂无来源统计')}</div>}</section>;
}
