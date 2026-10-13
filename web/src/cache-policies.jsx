import React from 'react';
import './cache-policies.css';
import {useLocale} from './locale';
import {cacheText} from './cache-i18n';

export function endpointCacheIdentity(endpoint){
 return `${endpoint.protocol}://${endpoint.address}${endpoint.protocol==='doh'?'|'+endpoint.url:endpoint.protocol==='dot'?'|'+endpoint.tls_name:''}`;
}
export function cacheUpstreamRows(dns={},policies={},groups={}){
 const rows=new Map();
 const add=endpoint=>{const key=endpointCacheIdentity(endpoint);if(!rows.has(key))rows.set(key,{key,name:endpoint.name||key})};
 (dns.upstreams||[]).forEach(add);
 (dns.policy_upstreams||[]).forEach(rule=>{(rule.upstreams||[]).forEach(add);(rule.fallback_upstreams||[]).forEach(add)});
 [...Object.keys(policies),...Object.keys(groups)].filter(key=>key!=='other').forEach(key=>{if(!rows.has(key))rows.set(key,{key,name:key})});
 return [...rows.values()];
}
const decisions=['DIRECT','SURGE','REJECT','UNKNOWN'];
export function CacheGroupStatistics({groups,kind='decision'}){
 const [locale]=useLocale(),t=cacheText(locale);
 if(groups==null)return null;
 const rows=kind==='decision'?decisions.map(key=>[key,groups[key]||{}]):Object.entries(groups).sort(([a],[b])=>a.localeCompare(b));
 const columns=['entries','hits','misses','evictions',kind==='response'?'responsePending':'refreshPending','optimisticHits','optimisticCompared','optimisticMismatches','optimisticMismatchRate'];
 return <section><h3>{t(kind==='decision'?'decisionBreakdown':'upstreamBreakdown')}</h3><div className="table-wrap"><table className="responsive-table"><thead><tr><th>{t(kind==='decision'?'decision':'upstream')}</th>{columns.map(key=><th key={key}>{t(key)}</th>)}</tr></thead><tbody>{rows.map(([key,s])=>{
 const values=[s.entries??0,s.hits??0,s.misses??0,s.evictions??0,s.stale_entries??0,s.optimistic_hits??0,s.optimistic_compared??0,s.optimistic_mismatches??0,s.optimistic_compared?`${(s.optimistic_mismatches/s.optimistic_compared*100).toLocaleString(locale,{maximumFractionDigits:2})}%`:'—'];
 return <tr key={key}><td data-label={t(kind==='decision'?'decision':'upstream')} className="cache-reason cache-policy-name">{key==='other'?t('otherUpstreams'):key}</td>{values.map((value,i)=><td key={columns[i]} data-label={t(columns[i])}>{value}</td>)}</tr>;
 })}</tbody></table>{!rows.length&&<p className="empty">{t('noGroupSamples')}</p>}</div><p>{t('groupStatisticsHelp')}</p></section>;
}
export function CachePolicySettings({settings,dns,status={},onChange}){
 const [locale]=useLocale(),t=cacheText(locale),supported=status.selective_cache_policies===true;
 const update=(field,key,name,value)=>onChange({...settings,[field]:{...settings[field],[key]:{...settings[field]?.[key],[name]:value}}});
 const switches=(field,rows)=>rows.map(({key,name})=>{
 const policy=settings[field]?.[key]||{};
 return <div className="cache-policy-row" key={key}><label className="check cache-policy-item" title={t('optimisticEnabled')}><input type="checkbox" aria-label={`${key} ${t('optimisticEnabled')}`} checked={policy.optimistic!==false} onChange={e=>update(field,key,'optimistic',e.target.checked)}/><span className="cache-policy-name">{name}{name!==key&&<small>{key}</small>}</span></label></div>;
 });
 const groups=[
 {master:'optimistic',idle:'optimistic_idle_refresh',field:'decisions',title:'decisionMaster',heading:'decisionPolicies',rows:decisions.filter(key=>key!=='UNKNOWN').map(key=>({key,name:key}))},
 {master:'optimistic_responses',idle:'optimistic_response_idle_refresh',field:'upstreams',title:'responseMaster',heading:'upstreamPolicies',rows:cacheUpstreamRows(dns,settings.upstreams,status.cache?.response?.groups)}
 ];
 return <section className="cache-policy-settings"><div className="cache-policy-groups">{groups.map(group=>{
 const enabled=Boolean(settings[group.master]);
 return <fieldset className="cache-policy-group" aria-label={t(group.heading)} key={group.master}><legend><label className="check" title={t(group.master+'_tooltip')}><input type="checkbox" aria-label={t(group.title)} checked={enabled} onChange={e=>onChange({...settings,[group.master]:e.target.checked})}/>{t(group.title)}</label></legend>{enabled&&<>
 {Object.hasOwn(settings,group.idle)&&<label className="check cache-policy-idle" title={t(group.idle+'_tooltip')}><input type="checkbox" checked={settings[group.idle]!==false} onChange={e=>onChange({...settings,[group.idle]:e.target.checked})}/>{t(group.idle)}</label>}
 {supported?<><h3>{t(group.heading)}</h3>{switches(group.field,group.rows)}{group.field==='upstreams'&&<p>{t('originalUpstreamHelp')}</p>}</>:<p role="status">{t('selectiveUnsupported')}</p>}
 </>}</fieldset>;
 })}</div>{supported&&(settings.optimistic||settings.optimistic_responses)&&<p>{t('selectiveHelp')}</p>}</section>;
}
