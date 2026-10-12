import React,{useEffect,useRef,useState} from 'react';
import {request} from './client';
import {useT,message} from './i18n';

export function ClientSettings({configs,clients=[],refresh,notify}){
 const t=useT(),[draft,setDraft]=useState(null),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const dialog=useRef(null),busyRef=useRef(busy);busyRef.current=busy;
 useEffect(()=>{
  if(!draft)return;const previous=document.activeElement,overflow=document.body.style.overflow;document.body.style.overflow='hidden';dialog.current?.querySelector('input')?.focus();
  const keys=e=>{if(e.key==='Escape'&&!busyRef.current)setDraft(null);if(e.key==='Tab'){const items=[...dialog.current.querySelectorAll('input,select,button')].filter(el=>!el.disabled),first=items[0],last=items.at(-1);if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus()}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus()}}};
  document.addEventListener('keydown',keys);return()=>{document.body.style.overflow=overflow;document.removeEventListener('keydown',keys);previous?.focus()};
 },[Boolean(draft)]);
 if(!Array.isArray(configs))return <div className="notice">{t('此后端暂不支持客户端设置，请升级后端。')}</div>;
 const edit=value=>{setError('');setDraft(value)};
 const save=async e=>{
  e.preventDefault();if(busy)return;setBusy(true);setError('');
  try{const {id,...body}=draft;await request('dns/client-settings'+(id?'/'+id:''),id?'PUT':'POST',body);setDraft(null);notify?.(t('客户端设置已保存'));await refresh?.()}catch(e){setError(message(e.message))}finally{setBusy(false)}
 };
 const remove=async row=>{
  if(busy)return;setBusy(true);setError('');
  try{await request('dns/client-settings/'+row.id,'DELETE');notify?.(t('客户端设置已删除'));await refresh?.()}catch(e){setError(message(e.message))}finally{setBusy(false)}
 };
 return <section className="panel client-settings"><header className="section-heading"><h2>{t('客户端设置')}</h2><p>{t('按源 IP 指定备注名称和查询策略。自动沿用域名与 Surge 规则；强制策略优先，但仍遵守查询校验、环路防护和 Fake-IP 健康门禁。')}</p></header>
  <div className="toolbar"><button type="button" className="primary" disabled={busy} onClick={()=>edit({ip:'',name:'',decision:''})}>{t('添加客户端')}</button><label>{t('从已发现客户端添加')}<select aria-label={t('从已发现客户端添加')} disabled={busy} value="" onChange={e=>{if(e.target.value)edit({ip:e.target.value,name:'',decision:''})}}><option value="">{t('选择客户端 IP')}</option>{clients.filter(c=>!configs.some(s=>s.ip===c.client_ip)).map(c=><option key={c.client_ip} value={c.client_ip}>{c.client_ip}</option>)}</select></label></div>
  {error&&!draft&&<p role="alert">{error}</p>}
  <div className="table-wrap"><table className="responsive-table"><thead><tr>{['客户端 IP','备注名称','查询策略','操作'].map(label=><th key={label}>{t(label)}</th>)}</tr></thead><tbody>{configs.map(row=><tr key={row.id}><td data-label={t('客户端 IP')}>{row.ip}</td><td data-label={t('备注名称')}>{row.name||'—'}</td><td data-label={t('查询策略')}>{row.decision||t('自动')}</td><td data-label={t('操作')}><div className="row-actions"><button disabled={busy} onClick={()=>edit({...row})}>{t('编辑')}</button><button disabled={busy} onClick={()=>remove(row)}>{t('删除')}</button></div></td></tr>)}</tbody></table>{!configs.length&&<div className="empty">{t('尚未配置客户端')}</div>}</div>
  {draft&&<div ref={dialog} className="modal" role="dialog" aria-modal="true" aria-label={t('客户端设置')}><section><header><h2>{t(draft.id?'编辑客户端':'添加客户端')}</h2><button disabled={busy} onClick={()=>setDraft(null)}>{t('关闭')}</button></header><form onSubmit={save}><div className="form-grid"><label>{t('客户端 IP')}<input aria-label={t('客户端 IP')} required maxLength={64} value={draft.ip} onChange={e=>setDraft({...draft,ip:e.target.value})} placeholder="192.168.1.10"/></label><label>{t('备注名称')}<input aria-label={t('备注名称')} maxLength={80} value={draft.name} onChange={e=>setDraft({...draft,name:e.target.value})}/></label><label>{t('查询策略')}<select aria-label={t('查询策略')} value={draft.decision} onChange={e=>setDraft({...draft,decision:e.target.value})}>{['','DIRECT','SURGE','REJECT'].map(value=><option key={value} value={value}>{value||t('自动')}</option>)}</select></label></div><p className="field-hint">{t('客户端 IP 必须为具体的 IPv4 或 IPv6 地址，不支持网段。备注可留空，自动策略仅保存备注。')}</p>{error&&<p role="alert">{error}</p>}<footer><button type="button" disabled={busy} onClick={()=>setDraft(null)}>{t('取消')}</button><button type="submit" className="primary" disabled={busy}>{t(busy?'正在保存…':'保存')}</button></footer></form></section></div>}
 </section>;
}
