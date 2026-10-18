import {MetricValue} from './metric-value';
import React,{useState} from 'react';
import {ClientEditor} from './client-settings';
import {useT,message} from './i18n';
import {currentLocale} from './locale';

const summaryMetrics=[
 ['已跟踪客户端数','clients'],
 ['DNS 请求总数','queries'],
 ['DIRECT 请求数','direct'],
 ['SURGE 请求数','surge'],
 ['REJECT 请求数','reject'],
 ['UNKNOWN 请求数','unknown'],
 ['DNS 错误数','errors']
];
const clientColumns=[
 ['client_ip','客户端 IP'],['name','备注 / 主机名'],['decision','查询策略'],['queries','DNS 请求数'],['direct','DIRECT'],
 ['surge','SURGE'],['reject','REJECT'],['unknown','UNKNOWN'],
 ['errors','DNS 错误'],['last_seen','最后活动']
];

export function ClientsPanel({snapshot={},configs=[],refresh,notify}){
 const t=useT(),[editing,setEditing]=useState(null);
 if(snapshot===null)return <div className="notice">{t('此远程后端版本暂不支持客户端统计。更新远程后端后即可查看。')}</div>;
 const stats=snapshot.stats||{},observed=Array.isArray(snapshot.clients)?snapshot.clients:[],seen=new Set(observed.map(c=>c.client_ip)),settings=new Map((configs||[]).map(c=>[c.ip,c])),clients=[...observed,...(configs||[]).filter(c=>!seen.has(c.ip)).map(c=>({client_ip:c.ip}))],supported=Array.isArray(configs),columns=supported?[...clientColumns,['actions','操作']]:clientColumns;
 return <>
  {snapshot.storage_error&&<p role="alert" className="notice error">{t('客户端统计保存失败')}: {message(snapshot.storage_error)}</p>}
  <div className="metrics">{summaryMetrics.map(([label,key])=><div className="metric" key={key}><span>{t(label)}</span><MetricValue>{stats[key]??0}</MetricValue></div>)}</div>
  {snapshot.evicted_clients>0&&<p className="field-hint">{t('客户端列表容量为 {capacity} 个；已淘汰 {count} 个不活跃客户端，淘汰前的请求仍计入汇总统计。',{capacity:snapshot.capacity,count:snapshot.evicted_clients})}</p>}
  <section className="panel">
   <h2>{t('客户端列表')}</h2>
   {!supported&&<p className="notice">{t('此后端暂不支持客户端设置，请升级后端。')}</p>}
   <div className="table-wrap"><table className="responsive-table" role="table">
    <thead role="rowgroup"><tr role="row">{columns.map(([key,label])=><th key={key} role="columnheader">{t(label)}</th>)}</tr></thead>
    <tbody role="rowgroup">{clients.map(raw=>{const setting=settings.get(raw.client_ip);const client={...raw,name:setting?.name?.trim()||raw.hostname||'—',decision:setting?.decision||t('自动')};return <tr role="row" key={client.client_ip}>{columns.map(([key,label])=><td key={key} role="cell" data-label={t(label)}><span className="cell-value">{key==='actions'?<button type="button" aria-label={t('编辑客户端 {client}',{client:client.client_ip})} onClick={()=>setEditing({...setting,ip:client.client_ip,name:setting?.name||'',decision:setting?.decision||''})}>{t('编辑')}</button>:key==='name'?<>{client.name}{!setting?.name?.trim()&&raw.hostname&&<small className="field-hint">{t('反向查询')}</small>}{!setting?.name?.trim()&&raw.hostname_error&&<small className="field-hint" title={message(raw.hostname_error)}>{t('主机名探测失败')}</small>}</>:key==='last_seen'?(client[key]?new Date(client[key]).toLocaleString(currentLocale()):'—'):client[key]??'—'}</span></td>)}</tr>})}</tbody>
   </table>{!clients.length&&<div className="empty">{t('暂无客户端 DNS 请求')}</div>}</div>
  </section>
  {editing&&<ClientEditor key={editing.ip} client={editing} onClose={()=>setEditing(null)} refresh={refresh} notify={notify}/>}
 </>;
}

// Reuse existing monitoring names and privacy handling without changing IP identities.
export function clientDisplaySettings(configs=[],clients=[]){
 const names=new Map((configs||[]).map(row=>[row.ip,row]));
 for(const client of clients||[])if(client.hostname&&!names.get(client.client_ip)?.name?.trim())names.set(client.client_ip,{...names.get(client.client_ip),ip:client.client_ip,name:client.hostname});
 return [...names.values()];
}
