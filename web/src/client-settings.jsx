import React,{useEffect,useRef,useState} from 'react';
import {request} from './client';
import {useT,message} from './i18n';

export function ClientEditor({client,onClose,refresh,notify}){
 const t=useT(),[draft,setDraft]=useState({...client}),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const dialog=useRef(null),busyRef=useRef(busy);busyRef.current=busy;
 useEffect(()=>{
  const previous=document.activeElement,overflow=document.body.style.overflow;document.body.style.overflow='hidden';dialog.current?.querySelector('input:not([readonly])')?.focus();
  const keys=e=>{if(e.key==='Escape'&&!busyRef.current)onClose();if(e.key==='Tab'){const items=[...dialog.current.querySelectorAll('input,select,button')].filter(el=>!el.disabled),first=items[0],last=items.at(-1);if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus()}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus()}}};
  document.addEventListener('keydown',keys);return()=>{document.body.style.overflow=overflow;document.removeEventListener('keydown',keys);previous?.focus()};
 },[]);
 const save=async e=>{
  e.preventDefault();if(busy)return;setBusy(true);setError('');
  try{const {id,ip,name,decision}=draft,body={ip,name,decision};await request('dns/client-settings'+(id?'/'+id:''),id?'PUT':'POST',body);onClose();notify?.(t('客户端设置已保存'));await refresh?.()}catch(e){setError(message(e.message))}finally{setBusy(false)}
 };
 const remove=async()=>{
  if(busy)return;setBusy(true);setError('');
  try{await request('dns/client-settings/'+draft.id,'DELETE');notify?.(t('客户端设置已删除'));onClose();await refresh?.()}catch(e){setError(message(e.message))}finally{setBusy(false)}
 };
 return <div ref={dialog} className="modal" role="dialog" aria-modal="true" aria-label={t('客户端设置')}><section><header><h2>{t('编辑客户端')}</h2><button disabled={busy} onClick={()=>onClose()}>{t('关闭')}</button></header><form onSubmit={save}><div className="form-grid"><label>{t('客户端 IP')}<input aria-label={t('客户端 IP')} readOnly value={draft.ip}/></label><label>{t('备注名称')}<input aria-label={t('备注名称')} maxLength={80} value={draft.name} onChange={e=>setDraft({...draft,name:e.target.value})}/></label><label>{t('查询策略')}<select aria-label={t('查询策略')} value={draft.decision} onChange={e=>setDraft({...draft,decision:e.target.value})}>{['','DIRECT','SURGE','REJECT'].map(value=><option key={value} value={value}>{value||t('自动')}</option>)}</select></label></div><p className="field-hint">{t('按源 IP 指定备注名称和查询策略。自动沿用域名与 Surge 规则；强制策略优先，但仍遵守查询校验、环路防护和 Fake-IP 健康门禁。')}</p>{error&&<p role="alert">{error}</p>}<footer>{draft.id&&<button type="button" disabled={busy} onClick={remove}>{t('清除客户端设置')}</button>}<button type="button" disabled={busy} onClick={()=>onClose()}>{t('取消')}</button><button type="submit" className="primary" disabled={busy}>{t(busy?'正在保存…':'保存')}</button></footer></form></section></div>;
}
