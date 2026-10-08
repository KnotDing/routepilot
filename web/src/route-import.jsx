import React,{useEffect,useState} from 'react';
import {request} from './client';
import {useT,message,valueLabel} from './i18n';

export function RouteImport({settings={},onClose,refresh,notify}){
 const t=useT();
 const [url,setURL]=useState(''),[preview,setPreview]=useState(null),[selected,setSelected]=useState([]),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const [draft,setDraft]=useState({next_hop4:settings.surge?.next_hop4||'',next_hop6:settings.surge?.next_hop6||'',med:0,local_preference:100,communities:[],health_binding:'none',enabled:true,description:''});
 useEffect(()=>{const overflow=document.body.style.overflow;document.body.style.overflow='hidden';return()=>{document.body.style.overflow=overflow}},[]);
 useEffect(()=>{const escape=e=>{if(e.key==='Escape'&&!busy)onClose()};document.addEventListener('keydown',escape);return()=>document.removeEventListener('keydown',escape)},[busy,onClose]);
 const load=async()=>{setBusy(true);setError('');setPreview(null);setSelected([]);try{const result=await request('bgp/routes/import-url','POST',{url});setPreview(result);setSelected(result.prefixes)}catch(e){setError(e.message)}finally{setBusy(false)}};
 const save=async()=>{setBusy(true);setError('');try{const result=await request('bgp/routes/import','POST',{...draft,prefixes:selected});refresh();notify(t('已导入 {imported} 项，跳过 {skipped} 项',{imported:result.imported,skipped:result.skipped}));onClose()}catch(e){setError(e.message)}finally{setBusy(false)}};
 const families={4:preview?.prefixes.filter(p=>!p.includes(':'))||[],6:preview?.prefixes.filter(p=>p.includes(':'))||[]};
 const toggle=(prefix,checked)=>setSelected(old=>checked?[...old,prefix]:old.filter(p=>p!==prefix));
 const toggleFamily=(family,checked)=>setSelected(old=>checked?[...new Set([...old,...families[family]])]:old.filter(p=>!families[family].includes(p)));
 return <div className="modal" role="dialog" aria-modal="true" aria-label={t('从 URL 导入路由')}><form className="route-import" onSubmit={e=>{e.preventDefault();if(!busy)load()}}><header><h2>{t('从 URL 导入路由')}</h2><button type="button" disabled={busy} onClick={onClose}>{t('关闭')}</button></header>
  <label>{t('路由列表 URL')}<input type="url" autoFocus required value={url} placeholder="https://core.telegram.org/resources/cidr.txt" disabled={busy} onChange={e=>{setURL(e.target.value);setPreview(null);setSelected([]);setError('')}}/></label>
  <p className="muted">{t('每行一个 IPv4/IPv6 地址或 CIDR 网段，支持 # 注释。导入为一次性快照。')}</p>
  <div className="toolbar"><button type="submit" disabled={busy||!url.trim()}>{t(busy?'正在处理…':'获取并预览')}</button></div>
  {error&&<div className="notice error" role="alert">{message(error)}</div>}
  {preview&&<><section className="panel"><h3>{t('导入预览')}</h3><p role="status">{t('可导入 {count} 项 · 已选择 {selected} 项 · 跳过 {skipped} 行',{count:preview.prefixes.length,selected:selected.length,skipped:preview.skipped.length})}</p>
   <div className="route-import-families">{[4,6].map(f=><label className="check" key={f}><input type="checkbox" disabled={busy||!families[f].length} checked={families[f].length>0&&families[f].every(p=>selected.includes(p))} onChange={e=>toggleFamily(f,e.target.checked)}/>{t('导入 IPv{family}（{count} 项）',{family:f,count:families[f].length})}</label>)}</div>
   <div className="route-import-list">{preview.prefixes.map(prefix=><label className="check" key={prefix}><input type="checkbox" disabled={busy} checked={selected.includes(prefix)} onChange={e=>toggle(prefix,e.target.checked)}/><code>{prefix}</code></label>)}{!preview.prefixes.length&&<p>{t('暂无可导入条目')}</p>}</div>
   {preview.skipped.length>0&&<details><summary>{t('查看跳过的条目')}</summary>{preview.skipped.map((s,i)=><p key={i}>{t('第 {line} 行：{reason}',{line:s.line,reason:message(s.reason)})}</p>)}</details>}
  </section>
  {preview.prefixes.length>0&&<><div className="form-grid">{[4,6].map(f=>families[f].length>0&&<label key={f}>{t('IPv{family} 下一跳',{family:f})}<input aria-label={t('IPv{family} 下一跳',{family:f})} required={selected.some(p=>p.includes(':')===(f===6))} disabled={busy} value={draft['next_hop'+f]} onChange={e=>setDraft({...draft,['next_hop'+f]:e.target.value})}/></label>)}
   <label>{t('健康绑定')}<select disabled={busy} value={draft.health_binding} onChange={e=>setDraft({...draft,health_binding:e.target.value})}>{['none','controller','surge','dns'].map(v=><option value={v} key={v}>{valueLabel(v)}</option>)}</select></label>
   <label>{t('说明')}<input disabled={busy} value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></label>
   {['med','local_preference'].map(k=><label key={k}>{t(k==='med'?'MED':'本地优先级')}<input type="number" min="0" max="4294967295" disabled={busy} value={draft[k]} onChange={e=>setDraft({...draft,[k]:Number(e.target.value)})}/></label>)}
   <label className="field-list">{t('Communities（每行 ASN:value）')}<textarea rows={3} disabled={busy} value={draft.communities.join('\n')} onChange={e=>setDraft({...draft,communities:e.target.value.split('\n').map(v=>v.trim()).filter(Boolean)})}/></label>
   <label className="check"><input type="checkbox" disabled={busy} checked={draft.enabled} onChange={e=>setDraft({...draft,enabled:e.target.checked})}/>{t('启用')}</label>
  </div><p className="muted">{t('整批导入会检查下一跳和 BGP 路由上限；已有路由保留。')}</p></>}
  <footer><button type="button" disabled={busy} onClick={onClose}>{t('取消')}</button><button type="button" className="primary" disabled={busy||!selected.length||selected.some(p=>!draft[p.includes(':')?'next_hop6':'next_hop4'].trim())} onClick={e=>{if(e.currentTarget.form.reportValidity())save()}}>{t('导入所选 {count} 项',{count:selected.length})}</button></footer></>}
 </form></div>
}
