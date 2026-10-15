import React from 'react';
import {currentLocale} from './locale';
import {useT} from './i18n';
import {destinationLabels} from './monitoring-data';
import {useMonitoringDisplay} from './monitoring-preferences';

export const emptyMonitoringFilters={destination:'',policy:'',final_policy:'',domain:'',response_code:''};
export function MonitoringFilters({filters,onChange,options,supported}){
 const t=useT(),display=useMonitoringDisplay(),policy=(row,final)=>row.kind==='unnamed'?t(final?'未记录最终策略':'未记录策略'):display.policy(row.name,final);
 const fields=[['destination','DNS 处理去向',(options?.destinations||[]).map(value=>({value,label:t(destinationLabels[value]||value)})),false],['policy','策略 / 策略组',(options?.policies||[]).map(row=>({value:row.value,label:policy(row,false)})),true],['final_policy','最终策略',(options?.final_policies||[]).map(row=>({value:row.value,label:policy(row,true)})),true],['domain','域名',(options?.domains||[]).map(value=>({value,label:display.domain(value)})),true],['response_code','响应码',(options?.response_codes||[]).map(value=>({value,label:value})),false]];
 return fields.map(([key,title,choices,sensitive])=>{
  const protectedValue=display.protected&&sensitive,selected=choices.find(row=>row.value===filters[key]),shown=protectedValue?selected?.label||'':filters[key];
  return <label key={key}>{t(title)}<select aria-label={t(title)} disabled={!supported} value={shown} onChange={event=>onChange({...filters,[key]:protectedValue?choices.find(row=>row.label===event.target.value)?.value||'':event.target.value})}><option value="">{t('全部')}</option>{choices.map(row=><option key={protectedValue?row.label:row.value} value={protectedValue?row.label:row.value}>{row.label}</option>)}{filters[key]&&!selected&&!protectedValue&&<option value={filters[key]}>{filters[key]}</option>}</select></label>;
 });
}

export function DomainStatistics({domains,selected,onSelect,unrecorded=false,statistics=null}){
 const t=useT(),display=useMonitoringDisplay(),number=value=>value.toLocaleString(currentLocale()),total=domains?.reduce((sum,row)=>sum+row.queries,0)||0;
 const label=row=>row.other?t(statistics?'未记录域名':'超出明细容量'):row.domain?display.domain(row.domain):t('未记录域名'),sorted=(domains||[]).filter(row=>!row.rest&&!row.other).sort((a,b)=>b.queries-a.queries||a.domain.localeCompare(b.domain)),top=sorted.slice(0,100),rest=[...sorted.slice(100),...(domains||[]).filter(row=>row.rest)].reduce((sum,row)=>sum+row.queries,0);
 const rows=[...top,...(rest?[{domain:'',rest:true,queries:rest}]:[]),...(domains||[]).filter(row=>row.other)];
 return <section className="panel monitor-domain-shares"><header className="section-heading"><h2>{t('请求域名占比')}</h2><p>{t(statistics?'按域名独立累计完成请求，不受组合明细容量限制。':'按已记录组合明细中的完成请求计算域名占比。')}</p></header>{statistics&&<p className="field-hint">{t('域名统计起点：{time}；升级前缺失的明细不倒填。',{time:new Date(statistics.started_at).toLocaleString(currentLocale())})}</p>}{statistics?.unrecorded>0&&<p className="field-hint">{t('部分请求因暂存容量或存储故障未保留域名，单独计入未记录域名。')}</p>}{!statistics&&domains?.some(row=>row.other)&&<p className="field-hint">{t('此范围使用旧版组合明细，超出容量的域名无法还原。')}</p>}{!Array.isArray(domains)?<div className="notice">{t(unrecorded?'此时段尚未采集域名统计。':'此后端未提供域名统计，请升级后端。')}</div>:<div className="monitor-bars monitor-source-bars">{rows.map((row,i)=><div key={row.rest?'rest':JSON.stringify([row.domain,row.other])} className={'monitor-bar-row '+(row.domain&&selected===row.domain?'monitor-source-selected':'')}><span className="monitor-rank">{i+1}</span>{row.domain&&!row.other&&!row.rest?<button className="monitor-source-link monitor-policy-name" title={label(row)} aria-label={t('筛选域名：{domain}',{domain:label(row)})} aria-pressed={selected===row.domain} onClick={()=>onSelect(selected===row.domain?'':row.domain)}>{label(row)}</button>:<span className="monitor-policy-name">{row.rest?t('其余域名'):label(row)}</span>}<div className="monitor-bar-track"><span style={{width:`${total?row.queries/total*100:0}%`}}/></div><strong>{number(row.queries)}</strong><small>{total?`${(row.queries/total*100).toFixed(2)}%`:'—'}</small></div>)}</div>}{Array.isArray(domains)&&!domains.length&&<div className="empty">{t('暂无域名统计')}</div>}</section>;
}
