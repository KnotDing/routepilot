import React from 'react';
import {useT} from './i18n';
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
 ['client_ip','客户端 IP'],['queries','DNS 请求数'],['direct','DIRECT'],
 ['surge','SURGE'],['reject','REJECT'],['unknown','UNKNOWN'],
 ['errors','DNS 错误'],['last_seen','最后活动']
];

export function ClientsPanel({snapshot={}}){
 const t=useT();
 if(snapshot===null)return <div className="notice">{t('此远程后端版本暂不支持客户端统计。更新远程后端后即可查看。')}</div>;
 const stats=snapshot.stats||{},clients=Array.isArray(snapshot.clients)?snapshot.clients:[];
 return <>
  <div className="metrics">{summaryMetrics.map(([label,key])=><div className="metric" key={key}><span>{t(label)}</span><strong>{stats[key]??0}</strong></div>)}</div>
  {snapshot.evicted_clients>0&&<p className="field-hint">{t('客户端列表容量为 {capacity} 个；已淘汰 {count} 个不活跃客户端，淘汰前的请求仍计入汇总统计。',{capacity:snapshot.capacity,count:snapshot.evicted_clients})}</p>}
  <section className="panel">
   <h2>{t('客户端列表')}</h2>
   <div className="table-wrap"><table className="responsive-table" role="table">
    <thead role="rowgroup"><tr role="row">{clientColumns.map(([key,label])=><th key={key} role="columnheader">{t(label)}</th>)}</tr></thead>
    <tbody role="rowgroup">{clients.map(client=><tr role="row" key={client.client_ip}>{clientColumns.map(([key,label])=><td key={key} role="cell" data-label={t(label)}><span className="cell-value">{key==='last_seen'?(client[key]?new Date(client[key]).toLocaleString(currentLocale()):'—'):client[key]??0}</span></td>)}</tr>)}</tbody>
   </table>{!clients.length&&<div className="empty">{t('暂无客户端 DNS 请求')}</div>}</div>
  </section>
 </>;
}
