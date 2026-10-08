import React from 'react';
import {useLocale} from './locale';
import {message} from './i18n';

const texts={
 'zh-CN':{title:'策略专用 DNS 上游',help:'按名称精确匹配，final_policy 优先于 policy。未匹配时使用 Surge；DIRECT、REJECT、always-real-ip 保持原有行为。优先查询专用上游，失败后使用所选备用。NXDOMAIN 和正常空响应不会触发回退。',add:'添加策略上游',remove:'删除策略',field:'匹配字段',policy:'策略名称',enabled:'启用策略上游',upstream:'DNS 服务器',protocol:'协议',address:'服务器 IP:端口',tls:'TLS 服务器名称',url:'DoH HTTPS 地址',addServer:'添加服务器',removeServer:'删除服务器',result:'策略 DNS 覆盖',used:'本次查询使用此策略配置的专用 DNS 上游。',fallback:'查询失败后的备用选项',none:'不回退，返回解析失败',real:'全局真实 DNS 上游',surge:'Surge',upstreams:'其他 DNS 上游',backup:'备用 DNS 服务器',backupHelp:'主上游和备用各自使用 DNS 超时时间。Surge 回退仍受 Fake-IP 路由与健康状态约束。',fallbackUsed:'主上游查询失败，本次查询已尝试备用选项。',primary:'主上游',stage:'查询阶段',resolver:'实际解析器',status:'响应状态',latency:'耗时（毫秒）',error:'失败原因',cache:'响应缓存',hit:'命中',miss:'未命中'},
 en:{title:'DNS upstreams by policy',help:'Match exact names; final_policy takes priority over policy. Unmatched policies use Surge. DIRECT, REJECT and always-real-ip retain their existing behavior. Query the dedicated upstream first, then use the selected backup on failure. NXDOMAIN and successful empty answers do not trigger fallback.',add:'Add policy upstream',remove:'Remove policy',field:'Match field',policy:'Policy name',enabled:'Enable policy upstream',upstream:'DNS server',protocol:'Protocol',address:'Server IP:port',tls:'TLS server name',url:'DoH HTTPS URL',addServer:'Add server',removeServer:'Remove server',result:'Policy DNS override',used:'This query used the dedicated DNS upstream configured for this policy.',fallback:'Backup on lookup failure',none:'No fallback; return resolution failure',real:'Global real DNS upstreams',surge:'Surge',upstreams:'Other DNS upstreams',backup:'Backup DNS servers',backupHelp:'The primary and backup each receive the DNS timeout window. Surge fallback still requires healthy Fake-IP routing.',fallbackUsed:'The primary lookup failed. This query attempted the selected backup.',primary:'Primary upstream',stage:'Lookup stage',resolver:'Actual resolver',status:'Response status',latency:'Duration (ms)',error:'Failure reason',cache:'Response cache',hit:'Hit',miss:'Miss'}
};
const server=()=>({protocol:'udp',address:'',enabled:true,name:''});
function ServerPool({value,onChange,t,title}){
 const rows=value||[];
 return <fieldset className="policy-dns-pool"><legend>{title}</legend>{rows.map((ep,j)=>{
  const update=next=>onChange(rows.map((v,k)=>k===j?next:v));
  return <fieldset key={j}><legend>{t.upstream} {j+1}</legend><div className="form-grid"><label>{t.protocol}<select value={ep.protocol} onChange={e=>update({...ep,protocol:e.target.value})}>{['udp','tcp','dot','doh'].map(p=><option key={p}>{p}</option>)}</select></label><label>{t.address}<input required placeholder="223.6.6.6:53" value={ep.address} onChange={e=>update({...ep,address:e.target.value})}/></label>{ep.protocol==='dot'&&<label>{t.tls}<input required value={ep.tls_name||''} onChange={e=>update({...ep,tls_name:e.target.value})}/></label>}{ep.protocol==='doh'&&<label>{t.url}<input required type="url" value={ep.url||''} onChange={e=>update({...ep,url:e.target.value})}/></label>}</div><button type="button" disabled={rows.length===1} onClick={()=>onChange(rows.filter((_,k)=>k!==j))}>{t.removeServer}</button></fieldset>
 })}<button type="button" disabled={rows.length>=16} onClick={()=>onChange([...rows,server()])}>{t.addServer}</button></fieldset>
}
export function PolicyDNSEditor({value=[],onChange}){
 const [locale]=useLocale(),t=texts[locale]||texts['zh-CN'];
 const rows=Array.isArray(value)?value:[];
 const change=(i,next)=>onChange(rows.map((row,j)=>i===j?next:row));
 return <fieldset className="policy-dns-editor"><legend>{t.title}</legend><p>{t.help}</p>{rows.map((row,i)=><section className="policy-dns-card" key={i} aria-label={`${t.title} ${i+1}`}>
  <div className="form-grid"><label>{t.field}<select value={row.field} onChange={e=>change(i,{...row,field:e.target.value})}><option value="final_policy">final_policy</option><option value="policy">policy</option></select></label><label>{t.policy}<input required maxLength={256} value={row.policy} onChange={e=>change(i,{...row,policy:e.target.value})}/></label><label className="check"><input type="checkbox" checked={row.enabled} onChange={e=>change(i,{...row,enabled:e.target.checked})}/>{t.enabled}</label></div>
  <ServerPool title={t.primary} t={t} value={row.upstreams} onChange={upstreams=>change(i,{...row,upstreams})}/>
  <div className="form-grid"><label className="policy-dns-fallback">{t.fallback}<select value={row.fallback||'none'} onChange={e=>change(i,{...row,fallback:e.target.value,...e.target.value==='upstreams'&&!row.fallback_upstreams?.length?{fallback_upstreams:[server()]}:{}})}>{['none','upstreams','surge','real'].map(option=><option key={option} value={option}>{t[option]}</option>)}</select></label></div>
  {row.fallback==='upstreams'&&<ServerPool title={t.backup} t={t} value={row.fallback_upstreams} onChange={fallback_upstreams=>change(i,{...row,fallback_upstreams})}/>}
  {row.fallback&&row.fallback!=='none'&&<p>{t.backupHelp}</p>}
  <div className="toolbar"><button className="danger" type="button" onClick={()=>onChange(rows.filter((_,j)=>j!==i))}>{t.remove}</button></div>
 </section>)}<button type="button" disabled={rows.length>=128} onClick={()=>onChange([...rows,{field:'final_policy',policy:'',enabled:true,upstreams:[server()]}])}>{t.add}</button></fieldset>
}
export function PolicyDNSResult({result}){
 const [locale]=useLocale(),t=texts[locale]||texts['zh-CN'];
 if(!result.policy_dns)return null;
 const match=result.policy_dns;
 return <section className="panel"><h3>{t.result}</h3><p>{match.fallback_used?t.fallbackUsed:t.used}</p><dl><dt>{t.field}</dt><dd>{match.field}</dd><dt>{t.policy}</dt><dd>{match.policy}</dd>{match.fallback&&match.fallback!=='none'&&<><dt>{t.fallback}</dt><dd>{t[match.fallback]||match.fallback}</dd></>}</dl>{match.attempts?.length>0&&<div className="table-wrap"><table><thead><tr>{['stage','resolver','status','latency','cache','error'].map(label=><th key={label}>{t[label]}</th>)}</tr></thead><tbody>{match.attempts.map((attempt,i)=><tr key={i}><td>{t[attempt.source==='primary'?'primary':attempt.source]||attempt.source}</td><td>{message(attempt.resolver,locale)||'—'}</td><td>{{0:'NOERROR',1:'FORMERR',2:'SERVFAIL',3:'NXDOMAIN',4:'NOTIMP',5:'REFUSED'}[attempt.rcode]??attempt.rcode??'—'}</td><td>{Number(attempt.latency_ms||0).toFixed(2)}</td><td>{attempt.cache_hit?t.hit:t.miss}</td><td>{message(attempt.error,locale)||'—'}</td></tr>)}</tbody></table></div>}</section>
}
