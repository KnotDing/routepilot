import React,{useEffect,useRef,useState} from 'react';
import {request} from './client';
import {useT} from './i18n';
import {currentLocale} from './locale';
import {useWindowVisible} from './window-visibility';

export function sourceName(source,t){return ({unknown:t('无法识别的源'),other:t('超出源明细容量'),legacy:t('启用前未采集来源')})[source]||source}
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

export function useMonitoringData({snapshot,historyStatus,mode,range,from,to,source}){
 const visible=useWindowVisible(),latest=useRef(snapshot),retry=useRef(()=>{}),[state,setState]=useState({history:null,selected:null,error:'',loading:false});
 latest.current=snapshot;
 const ready=Boolean(snapshot),supported=historyStatus?.supported===true,key=JSON.stringify([mode,range,from,to,source,supported]);
 useEffect(()=>{
  if(!ready||!visible)return;
  let alive=true,busy=false;
  setState({key,history:null,selected:null,error:'',loading:true});
  const load=async()=>{
   if(busy)return;busy=true;
   try{
    const window=historyRange(latest.current,mode,range,from,to),query=new URLSearchParams({...window,...(source?{source}:{})});
    const historyRequest=supported?request('dns/monitoring/history?'+query).catch(e=>{if(e.status===404||e.status===501)return null;throw e}):Promise.resolve(null);
    const selectedRequest=mode==='live'&&source?request('dns/monitoring?'+new URLSearchParams({source})):Promise.resolve(null);
    const [history,selected]=await Promise.all([historyRequest,selectedRequest]);
    if(history&&(!Array.isArray(history.points)||!history.summary))throw Error('历史监控响应格式无效。');
    if(alive)setState({key,history,selected,error:'',loading:false});
   }catch(e){if(alive)setState({key,history:null,selected:null,error:e.message,loading:false})}finally{busy=false}
  };
  retry.current=load;load();
  const timer=mode==='live'?setInterval(load,3000):null;
  return()=>{alive=false;if(timer)clearInterval(timer)};
 },[ready,supported,visible,mode,range,from,to,source]);
 return {...(state.key===key?state:{history:null,selected:null,error:'',loading:ready}),reload:()=>retry.current()};
}

const localDateInput=date=>new Date(date.getTime()-date.getTimezoneOffset()*60000).toISOString().slice(0,16);
export function MonitoringControls({mode,onMode,range,onRange,from,onFrom,to,onTo,source,onSource,sources,snapshot,reload}){
 const t=useT();
 const chooseRange=value=>{if(value==='custom'&&!from){const end=new Date(Math.floor(Date.parse(snapshot?.observed_at||new Date().toISOString())/60000)*60000+60000);onFrom(localDateInput(new Date(end-3600000)));onTo(localDateInput(end))}onRange(value)};
 return <div className="monitor-view-controls"><div className="tabs" role="group" aria-label={t('监控数据视图')}>{[['live','实时数据'],['history','历史数据']].map(([key,label])=><button key={key} aria-pressed={mode===key} className={mode===key?'selected':''} onClick={()=>onMode(key)}>{t(label)}</button>)}</div>
  <label>{t('请求来源')}<select aria-label={t('请求来源')} value={source} onChange={e=>onSource(e.target.value)}><option value="">{t('全部来源')}</option>{sources.map(s=><option key={s.source} value={s.source}>{sourceName(s.source,t)}</option>)}{source&&!sources.some(s=>s.source===source)&&<option value={source}>{sourceName(source,t)}</option>}</select></label>
  {mode==='history'&&<><label>{t('历史时间范围')}<select aria-label={t('历史时间范围')} value={range} onChange={e=>chooseRange(e.target.value)}>{[['3600','最近一小时'],['21600','最近六小时'],['86400','最近一天'],['604800','最近七天'],['custom','自定义时间']].map(([key,label])=><option key={key} value={key}>{t(label)}</option>)}</select></label>{range==='custom'&&<><label>{t('开始时间')}<input type="datetime-local" aria-label={t('开始时间')} value={from} onChange={e=>onFrom(e.target.value)}/></label><label>{t('结束时间')}<input type="datetime-local" aria-label={t('结束时间')} value={to} onChange={e=>onTo(e.target.value)}/></label></>}<button onClick={reload}>{t('刷新历史')}</button></>}
 </div>;
}

export function SourceStatistics({sources,total,selected,onSelect}){
 const t=useT(),number=n=>n.toLocaleString(currentLocale());
 if(!Array.isArray(sources))return <div className="notice">{t('此后端未提供来源统计，请升级后端。')}</div>;
 return <section className="panel monitor-sources"><header className="section-heading"><h2>{t('按源统计 DNS 流入')}</h2><p>{t('来源按客户端 IP 区分。选择单个源后，流向、策略和趋势均使用该源的数据，占比以该源的请求为分母。')}</p></header><div className="table-wrap monitor-source-table"><table><thead><tr><th>{t('请求来源')}</th><th>{t('DNS 流入总数')}</th><th>{t('流入占比')}</th><th>{t('已生成响应')}</th><th>{t('操作')}</th></tr></thead><tbody>{sources.map(s=><tr key={s.source} className={selected===s.source?'monitor-source-selected':''}><td>{sourceName(s.source,t)}</td><td>{number(s.counts.received)}</td><td>{total?`${(s.counts.received/total*100).toFixed(2)}%`:'—'}</td><td>{number(s.counts.completed)}</td><td><button aria-label={t('查看 {source} 的流向',{source:sourceName(s.source,t)})} onClick={()=>onSelect(s.source)}>{t('查看流向')}</button></td></tr>)}</tbody></table>{!sources.length&&<div className="empty">{t('暂无来源统计')}</div>}</div><p className="field-hint">{t('最多保留 512 个源和每源 64 种策略流向，超出部分单独合并，汇总请求数保留。')}</p></section>;
}
