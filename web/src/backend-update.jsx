import React,{useEffect,useRef,useState,lazy,Suspense} from 'react';
const ReleaseNotes=lazy(()=>import('./release-notes').then(module=>({default:module.ReleaseNotes})));
import {request} from './client';
import {useT,message} from './i18n';

const active=phase=>['downloading','validating','restarting'].includes(phase);
const phases={idle:'尚未检查软件更新',downloading:'正在下载完整应用更新',validating:'正在校验完整应用更新',restarting:'正在重启后台，请稍候',succeeded:'远程软件升级已完成',failed:'远程软件升级失败'};
export function BackendUpdate({backend,capable,software,onReady}){
 const t=useT(),[update,setUpdate]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false),[offline,setOffline]=useState(false),[confirmation,setConfirmation]=useState(null),[skipped,setSkipped]=useState(()=>{try{return localStorage.getItem('routepilot.skippedBackendVersion:'+backend.id)||''}catch{return ''}});
 const modal=useRef(null);
 const mounted=useRef(true),polling=useRef(false),checked=useRef(false),readyCallback=useRef(onReady);readyCallback.current=onReady;
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false}},[]);
 const check=async(manual=true)=>{if(busy)return;setBusy(true);try{const value=await request('updates/check');if(mounted.current){setUpdate(value);setError('');if(manual){setSkipped('');if(value.available&&value.supported)setConfirmation({version:value.latest,notes:value.release_notes})}}}catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}finally{if(mounted.current)setBusy(false)}};
 useEffect(()=>{if(capable&&!checked.current){checked.current=true;check(false)}},[capable]);
 useEffect(()=>{
  if(!active(update?.phase))return;
  const started=Date.now();
  const poll=async()=>{
   if(polling.current)return;polling.current=true;
   try{const value=await request('updates/status');if(!mounted.current)return;setUpdate(value);setOffline(false);setError('');if(!active(value.phase))readyCallback.current?.()}
   catch(e){if(!mounted.current||e.name==='AbortError')return;setOffline(true);if(e.status===401||e.status===403){setError(t('后台已重启，请重新登录查看升级结果'));setUpdate(previous=>({...previous,phase:'idle'}))}else if(Date.now()-started>10*60*1000){setError(t('等待后台恢复超时，请检查远端服务后重新连接'));setUpdate(previous=>({...previous,phase:'idle'}))}}
   finally{polling.current=false}
  };
  const timer=setInterval(poll,2000);return()=>clearInterval(timer);
 },[Boolean(active(update?.phase))]);
 useEffect(()=>{
  if(!confirmation)return;
  const previous=document.activeElement,overflow=document.body.style.overflow;
  document.body.style.overflow='hidden';
  modal.current?.querySelector('[data-later]')?.focus();
  const keys=e=>{
   if(e.key==='Escape'){e.preventDefault();setConfirmation(null)}
   if(e.key==='Tab'){
    const items=[...modal.current.querySelectorAll('button,summary,a[href]')].filter(el=>!el.disabled&&(!el.closest('details')||el.tagName==='SUMMARY'||el.closest('details').open)),first=items[0],last=items.at(-1);
    if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus()}
    else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus()}
   }
  };
  document.addEventListener('keydown',keys);
  return()=>{document.body.style.overflow=overflow;document.removeEventListener('keydown',keys);previous?.focus()};
 },[confirmation]);
 const install=async version=>{
  if(!version||busy||active(update?.phase))return;
  setConfirmation(null);
  setBusy(true);try{const value=await request('updates/install','POST',{version});if(mounted.current){setUpdate(value);setError('')}}catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}finally{if(mounted.current)setBusy(false)}
 };
 const skip=()=>{const version=confirmation.version;try{localStorage.setItem('routepilot.skippedBackendVersion:'+backend.id,version)}catch{}setSkipped(version);setConfirmation(null)};
 if(!capable&&!update&&!error)return null;
 if(update?.available&&skipped===update.latest&&!confirmation&&!active(update.phase)&&update.phase!=='failed'&&!update.error&&!error)return null;
 if(!confirmation&&update&&!update.available&&!active(update.phase)&&update.phase!=='failed'&&!update.error&&!error)return null;
 return <><section className={'notice backend-update '+(update?.phase==='failed'?'error':'')} aria-label={t('远程软件更新')}>
  <strong>{t('远程软件更新')}</strong><p>{t('当前后台：{version}',{version:software?.version||update?.current||'—'})}{update?.complete_application&&' · '+t('远端 App：{version}',{version:update.app_current||t('未安装')})}{update?.latest&&' · '+t('最新发布：{version}',{version:update.latest})}</p>
  {update&&<p role="status">{offline?t('后台暂时离线，正在等待升级完成并重连…'):active(update.phase)||['succeeded','failed'].includes(update.phase)?t(phases[update.phase]):update.available?t('有可用的软件新版本'):t('软件已是最新版本')}</p>}
  {update?.reason&&<p>{message(update.reason)}</p>}{update?.error&&<p role="alert">{message(update.error)}</p>}{error&&<p role="alert">{error}</p>}
  <div className="toolbar"><button type="button" disabled={busy||active(update?.phase)} onClick={()=>check()}>{t(busy?'处理中…':'检查远程更新')}</button>{update?.supported&&update.available&&<button type="button" className="primary" disabled={busy||active(update.phase)} onClick={()=>setConfirmation({version:update.latest,notes:update.release_notes})}>{t(update.complete_application?'升级远程应用':'升级远程后台')}</button>}</div>
  <p>{t(update?.complete_application?'远端将下载并校验完整 App 安装包，同步更新 App、管理页面和网络服务。未安装 App 时会安装到 /Applications；已打开的远端 App 需重新打开。':'此后台仅支持服务升级，请先升级一次，再重新检查更新以同步完整 App。')}</p>
 </section>{confirmation&&<div className="modal" role="dialog" aria-modal="true" aria-labelledby="backend-update-title" ref={modal}><section className="update-dialog"><header><h2 id="backend-update-title">{t('远程软件更新')} · {confirmation.version}</h2><button type="button" onClick={()=>setConfirmation(null)}>{t('关闭')}</button></header><p>{t(update?.complete_application?'将远端 {name} 的完整应用和网络服务更新至 {version}。DNS 和 BGP 会短暂中断，配置与数据库保留。是否继续？':'将远程后台 {name} 更新至 {version}。DNS 和 BGP 会短暂中断，配置与数据库保留。是否继续？',{name:backend.name,version:confirmation.version})}</p><details><summary>{t('更新说明')}</summary><div className="release-notes"><Suspense fallback={<p role="status">{t('处理中…')}</p>}><ReleaseNotes>{confirmation.notes||t('此版本未提供更新说明。')}</ReleaseNotes></Suspense></div></details><footer><button type="button" onClick={skip}>{t('跳过该版本')}</button><button type="button" data-later onClick={()=>setConfirmation(null)}>{t('稍后升级')}</button><button type="button" className="primary" onClick={()=>install(confirmation.version)}>{t('立即升级')}</button></footer></section></div>}</>;

}
