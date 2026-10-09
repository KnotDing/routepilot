import {FieldLabel,FormHint} from './form-fields';
import React,{useEffect,useState} from 'react';
import {request} from './client';
import {useT,message,valueLabel} from './i18n';

export function RouteImport({settings={},onClose,refresh,notify}){
 const t=useT();
 const [url,setURL]=useState(''),[preview,setPreview]=useState(null),[selected,setSelected]=useState([]),[busy,setBusy]=useState(false),[error,setError]=useState(''),[feeds,setFeeds]=useState([]);
 const [autoUpdate,setAutoUpdate]=useState(false),[intervalMinutes,setIntervalMinutes]=useState(1440);
 const [autoFamilies,setAutoFamilies]=useState({4:false,6:false});
 const [draft,setDraft]=useState({next_hop4:settings.surge?.next_hop4||'',next_hop6:settings.surge?.next_hop6||'',med:0,local_preference:100,communities:[],health_binding:'none',enabled:true,description:''});
 useEffect(()=>{const overflow=document.body.style.overflow;document.body.style.overflow='hidden';return()=>{document.body.style.overflow=overflow}},[]);
 useEffect(()=>{request('bgp/routes/feeds').then(setFeeds).catch(()=>{})},[]);
 useEffect(()=>{const escape=e=>{if(e.key==='Escape'&&!busy)onClose()};document.addEventListener('keydown',escape);return()=>document.removeEventListener('keydown',escape)},[busy,onClose]);
 const load=async()=>{setBusy(true);setError('');setPreview(null);setSelected([]);setAutoFamilies({4:false,6:false});try{const result=await request('bgp/routes/import-url','POST',{url});setPreview(result);setSelected(result.prefixes);setAutoFamilies({4:result.prefixes.some(p=>!p.includes(':')),6:result.prefixes.some(p=>p.includes(':'))})}catch(e){setError(e.message)}finally{setBusy(false)}};
 const save=async()=>{setBusy(true);setError('');try{const result=await request('bgp/routes/import','POST',{...draft,url,auto_update:autoUpdate,interval_minutes:intervalMinutes,sync_ipv4:autoUpdate&&autoFamilies[4],sync_ipv6:autoUpdate&&autoFamilies[6],prefixes:selected});refresh();notify(t('已导入 {imported} 项，跳过 {skipped} 项',{imported:result.imported,skipped:result.skipped}));onClose()}catch(e){setError(e.message)}finally{setBusy(false)}};
 const removeFeed=async id=>{setBusy(true);setError('');try{await request('bgp/routes/feeds/'+encodeURIComponent(id),'DELETE');setFeeds(await request('bgp/routes/feeds'));refresh();notify(t('已停止此 URL 的自动更新，现有路由保留为手动路由'))}catch(e){setError(e.message)}finally{setBusy(false)}};
 const families={4:preview?.prefixes.filter(p=>!p.includes(':'))||[],6:preview?.prefixes.filter(p=>p.includes(':'))||[]};
 const toggle=(prefix,checked)=>setSelected(old=>checked?[...old,prefix]:old.filter(p=>p!==prefix));
 const toggleFamily=(family,checked)=>{setSelected(old=>checked?[...new Set([...old,...families[family]])]:old.filter(p=>!families[family].includes(p)));if(autoUpdate)setAutoFamilies(old=>({...old,[family]:checked}))};
 return <div className="modal" role="dialog" aria-modal="true" aria-label={t('从 URL 导入路由')}><form className="route-import" onSubmit={e=>{e.preventDefault();if(!busy)load()}}><header><h2>{t('从 URL 导入路由')}</h2><button type="button" disabled={busy} onClick={onClose}>{t('关闭')}</button></header>
  {feeds.length>0&&<section className="panel"><h3>{t('定时更新来源')}</h3>{feeds.map(feed=><div className="toolbar" key={feed.id}><span>{feed.url} · {t('每 {hours} 小时',{hours:feed.interval_minutes/60})}{feed.last_error&&<span className="error"> · {t('上次更新失败')}</span>}</span><button type="button" disabled={busy} onClick={()=>removeFeed(feed.id)}>{t('停止自动更新')}</button></div>)}</section>}
  <FormHint/><FieldLabel label={t('路由列表 URL')} required><input type="url" autoFocus required value={url} placeholder="https://core.telegram.org/resources/cidr.txt" disabled={busy} onChange={e=>{setURL(e.target.value);setPreview(null);setSelected([]);setError('')}}/></FieldLabel>
  <p className="muted">{t('每行一个 IPv4/IPv6 地址或 CIDR 网段，支持 # 注释。可选定时同步 URL。')}</p>
  <div className="toolbar"><button type="submit" disabled={busy||!url.trim()}>{t(busy?'正在处理…':'获取并预览')}</button></div>
  {error&&<div className="notice error" role="alert">{message(error)}</div>}
  {preview&&<><section className="panel"><h3>{t('导入预览')}</h3><p role="status">{t('可导入 {count} 项 · 已选择 {selected} 项 · 跳过 {skipped} 行',{count:preview.prefixes.length,selected:selected.length,skipped:preview.skipped.length})}</p>
   <div className="route-import-families">{[4,6].map(f=><label className="check" key={f}><input type="checkbox" disabled={busy||!families[f].length} checked={autoUpdate?autoFamilies[f]:families[f].length>0&&families[f].every(p=>selected.includes(p))} onChange={e=>toggleFamily(f,e.target.checked)}/>{t(autoUpdate?'自动更新 IPv{family}（{count} 项）':'导入 IPv{family}（{count} 项）',{family:f,count:families[f].length})}</label>)}</div>
   <div className="route-import-list">{preview.prefixes.map(prefix=><label className="check" key={prefix}><input type="checkbox" disabled={busy||autoUpdate} checked={selected.includes(prefix)} onChange={e=>toggle(prefix,e.target.checked)}/><code>{prefix}</code></label>)}{!preview.prefixes.length&&<p>{t('暂无可导入条目')}</p>}</div>
   {preview.skipped.length>0&&<details><summary>{t('查看跳过的条目')}</summary>{preview.skipped.map((s,i)=><p key={i}>{t('第 {line} 行：{reason}',{line:s.line,reason:message(s.reason)})}</p>)}</details>}
  </section>
  {preview.prefixes.length>0&&<><div className="form-grid">{[4,6].map(f=>(families[f].length>0&&(!autoUpdate||autoFamilies[f]))&&<FieldLabel key={f} label={t('IPv{family} 下一跳',{family:f})} required={autoUpdate?autoFamilies[f]:selected.some(p=>p.includes(':')===(f===6))}><input aria-label={t('IPv{family} 下一跳',{family:f})} required={selected.some(p=>p.includes(':')===(f===6))} disabled={busy} value={draft['next_hop'+f]} onChange={e=>setDraft({...draft,['next_hop'+f]:e.target.value})}/></FieldLabel>)}
   <FieldLabel label={t('健康绑定')}><select disabled={busy} value={draft.health_binding} onChange={e=>setDraft({...draft,health_binding:e.target.value})}>{['none','controller','surge','dns'].map(v=><option value={v} key={v}>{valueLabel(v)}</option>)}</select></FieldLabel>
   <FieldLabel label={t('说明')}><input disabled={busy} value={draft.description} onChange={e=>setDraft({...draft,description:e.target.value})}/></FieldLabel>
   {['med','local_preference'].map(k=><FieldLabel key={k} label={t(k==='med'?'MED':'本地优先级')} required><input type="number" min="0" max="4294967295" disabled={busy} value={draft[k]} onChange={e=>setDraft({...draft,[k]:e.target.value===''?'':Number(e.target.value)})}/></FieldLabel>)}
   <FieldLabel className="field-list" label={t('Communities（每行 ASN:value）')}><textarea rows={3} disabled={busy} value={draft.communities.join('\n')} onChange={e=>setDraft({...draft,communities:e.target.value.split('\n').map(v=>v.trim()).filter(Boolean)})}/></FieldLabel>
   <label className="check"><input type="checkbox" disabled={busy} checked={draft.enabled} onChange={e=>setDraft({...draft,enabled:e.target.checked})}/>{t('启用')}</label>
   <label className="check"><input type="checkbox" disabled={busy} checked={autoUpdate} onChange={e=>{setAutoUpdate(e.target.checked);if(e.target.checked){setSelected(preview.prefixes);setAutoFamilies({4:families[4].length>0,6:families[6].length>0})}}}/>{t('启用定时自动更新')}</label>
   {autoUpdate&&<FieldLabel label={t('更新间隔')} required><select disabled={busy} value={intervalMinutes} onChange={e=>setIntervalMinutes(Number(e.target.value))}>{[[60,'每小时'],[360,'每 6 小时'],[720,'每 12 小时'],[1440,'每天'],[10080,'每周']].map(([v,label])=><option key={v} value={v}>{t(label)}</option>)}</select></FieldLabel>}
  </div><p className="muted">{t('整批导入会检查下一跳和 BGP 路由上限；自动更新会按 URL 列表同步该来源管理的路由。')}{autoUpdate&&<span> {t('自动更新仅同步已勾选的地址族。')}</span>}</p></>}
  <footer><button type="button" disabled={busy} onClick={onClose}>{t('取消')}</button><button type="button" className="primary" disabled={busy||!selected.length||selected.some(p=>!draft[p.includes(':')?'next_hop6':'next_hop4'].trim())} onClick={e=>{if(e.currentTarget.form.reportValidity())save()}}>{t('导入所选 {count} 项',{count:selected.length})}</button></footer></>}
 </form></div>
}
