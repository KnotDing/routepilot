import React,{useEffect,useRef,useState,lazy,Suspense} from 'react';
const ReleaseNotes=lazy(()=>import('./release-notes').then(module=>({default:module.ReleaseNotes})));
import {request} from './client';
import {createPortal} from 'react-dom';
import {useT,message} from './i18n';
import {DownloadProgress} from './download-progress';

const active=phase=>['downloading','validating','restarting'].includes(phase);
const appPending=phase=>['checking','awaiting_confirmation','restarting'].includes(phase);
const appNotice=phase=>appPending(phase)||['deferred','failed'].includes(phase);
const appPhases={checking:'正在检查远端 App 的运行状态',awaiting_confirmation:'远端 App 正在使用，等待远端用户确认重启',restarting:'正在重启远端 App',deferred:'远端用户选择稍后重启 App；应用文件和网络服务已更新',failed:'远端 App 未能重启，请在远端手动重新打开'};
const phases={idle:'尚未检查软件更新',downloading:'正在下载完整应用更新',validating:'正在校验完整应用更新',restarting:'正在重启后台，请稍候',succeeded:'远程软件升级已完成',failed:'远程软件升级失败'};
export function BackendUpdate({backend=null,capable,software,onReady,target=null,showCurrent=false,embedded=false,authentication=true}){
 // Keep one updater mounted while moving its panel into Settings, so
 // navigation does not restart checks or interrupt installation polling.
 const expanded=showCurrent||Boolean(target),remote=Boolean(backend);
 const restartPhases=remote?appPhases:{checking:'正在检查 App 的运行状态',awaiting_confirmation:'App 正在使用，等待该 Mac 的用户确认重启',restarting:'正在重启 App',deferred:'用户选择稍后重启 App；应用文件和网络服务已更新',failed:'App 未能重启，请在该 Mac 手动重新打开'};
 const title=remote?'远程软件更新':'软件更新';
 const t=useT(),[update,setUpdate]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false),[offline,setOffline]=useState(false),[confirmation,setConfirmation]=useState(null);
 const modal=useRef(null);
 const mounted=useRef(true),polling=useRef(false),checked=useRef(false),readyCallback=useRef(onReady);readyCallback.current=onReady;
 useEffect(()=>{mounted.current=true;return()=>{mounted.current=false}},[]);
 const check=async(manual=true)=>{if(busy)return;setBusy(true);try{const value=await request('updates/check');if(mounted.current){setUpdate(value);setError('');if(manual){if(value.available&&value.supported)setConfirmation({version:value.latest,notes:value.release_notes})}}}catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}finally{if(mounted.current)setBusy(false)}};
 useEffect(()=>{if(capable&&!checked.current){checked.current=true;check(false)}},[capable]);
 useEffect(()=>{
  if(!expanded||!capable||active(update?.phase)||appNotice(update?.app_restart))return;
  let pending=false;
  const timer=setInterval(async()=>{if(pending)return;pending=true;try{const value=await request('updates/status');if(mounted.current)setUpdate(value)}catch{}finally{pending=false}},30000);
  return()=>clearInterval(timer);
 },[expanded,capable,Boolean(active(update?.phase)||appNotice(update?.app_restart))]);
 useEffect(()=>{
  if(!active(update?.phase)&&!appNotice(update?.app_restart))return;
  const started=Date.now();
  const poll=async()=>{
   if(polling.current)return;polling.current=true;
   try{const value=await request('updates/status');if(!mounted.current)return;setUpdate(value);setOffline(false);setError('');if(!active(value.phase)&&!appNotice(value.app_restart))readyCallback.current?.(value)}
   catch(e){if(!mounted.current||e.name==='AbortError')return;setOffline(true);if(e.status===401||e.status===403){setError(t('后台已重启，请重新登录查看升级结果'));setUpdate(previous=>({...previous,phase:'idle',app_restart:undefined}))}else if(Date.now()-started>10*60*1000){setError(t('等待后台恢复超时，请检查远端服务后重新连接'));setUpdate(previous=>({...previous,phase:'idle',app_restart:undefined}))}}
   finally{polling.current=false}
  };
  const timer=setInterval(poll,2000);return()=>clearInterval(timer);
 },[Boolean(active(update?.phase)||appNotice(update?.app_restart))]);
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
 },[confirmation,target]);
 const install=async version=>{
  if(!capable||!authentication||!version||busy||active(update?.phase))return;
  setConfirmation(null);
  setBusy(true);try{const value=await request('updates/install','POST',{version});if(mounted.current){setUpdate(value);setError('')}}catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}finally{if(mounted.current)setBusy(false)}
 };
 const skip=async()=>{
  if(busy)return;
  const version=confirmation.version;setBusy(true);
  try{const value=await request('updates/skip','POST',{version});if(mounted.current){setUpdate(value);setError('');setConfirmation(null)}}
  catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}
  finally{if(mounted.current)setBusy(false)}
 };
 const skipped=update?.skipped_version||'';
 if(!expanded&&!capable&&!update&&!error)return null;
 if(!expanded&&update?.available&&skipped===update.latest&&!confirmation&&!active(update.phase)&&update.phase!=='failed'&&!update.error&&!update.check_error&&!update.automation_error&&!appNotice(update.app_restart)&&!error)return null;
 if(!expanded&&!confirmation&&update&&!update.available&&!active(update.phase)&&update.phase!=='failed'&&!update.error&&!update.check_error&&!update.automation_error&&!appNotice(update.app_restart)&&!error)return null;
 const content=<><section className={(embedded||target?'update-status':'notice')+' backend-update '+(update?.phase==='failed'?'error':'')} aria-label={t(title)}>
  {!embedded&&!target&&<strong>{t(title)}</strong>}<p>{t('当前后台：{version}',{version:software?.version||update?.current||'—'})}{update&&' · '+t(remote?'远端 App：{version}':'App：{version}',{version:update.app_current||t('未安装')})}{update?.latest&&' · '+t('最新发布：{version}',{version:update.latest})}</p>
  {update&&<p role="status">{offline?t('后台暂时离线，正在等待升级完成并重连…'):active(update.phase)||['succeeded','failed'].includes(update.phase)?t(!remote&&update.phase==='succeeded'?'软件升级已完成':!remote&&update.phase==='failed'?'软件升级失败':phases[update.phase]):update.available?t('有可用的软件新版本'):t('软件已是最新版本')}</p>}
  {update?.phase==='downloading'&&!offline&&<DownloadProgress bytes={update.download_bytes} total={update.download_total}/>}
  {appNotice(update?.app_restart)&&<p role="status">{t(restartPhases[update.app_restart])}</p>}
  {update?.app_restart_error&&<p role="alert">{message(update.app_restart_error)}</p>}
  {update?.check_error&&<p role="alert">{t('检查更新失败')}：{message(update.check_error)}</p>}{update?.automation_error&&<p role="alert">{t('自动更新失败')}：{message(update.automation_error)}</p>}{update?.next_retry_at&&!String(update.next_retry_at).startsWith('0001-')&&<p>{t('将在 {time} 后重试',{time:new Date(update.next_retry_at).toLocaleString()})}</p>}{update?.skipped_version&&expanded&&<p>{t('已跳过版本 {version}，不会自动安装',{version:update.skipped_version})}<button type="button" disabled={busy} onClick={async()=>{setBusy(true);try{setUpdate(await request('updates/skip','POST',{version:''}));setError('')}catch(e){setError(message(e.message))}finally{setBusy(false)}}}>{t('取消跳过')}</button></p>}{update?.reason&&<p>{message(update.reason)}</p>}{update?.error&&<p role="alert">{message(update.error)}</p>}{error&&<p role="alert">{error}</p>}
  <div className="toolbar"><button type="button" disabled={!capable||busy||active(update?.phase)} onClick={()=>check()}>{t(busy?'处理中…':'立即检查更新')}</button>{update?.supported&&update.available&&<button type="button" className="primary" disabled={!authentication||busy||active(update.phase)} onClick={()=>setConfirmation({version:update.latest,notes:update.release_notes})}>{t(remote?'升级远程应用':'升级应用')}</button>}</div>
  {!authentication&&<p>{t('请先在“设置 → 安全”启用管理认证，再进行在线升级。')}</p>}
  {!capable&&<p>{t('当前后台尚未支持在线升级，请先在该 Mac 手动更新并安装后台服务。')}</p>}
  {!remote&&<p>{t('更新当前连接的后台所在 Mac 的完整应用，不会更新打开浏览器的设备。管理窗口正在使用时，需要在该 Mac 确认重启。')}</p>}
  {remote&&<p>{t('远端将下载并校验完整 App 安装包，同步更新 App、管理页面和网络服务。未安装 App 时会安装到 /Applications。已运行的 App 在重启前收到通知：管理窗口可见或 App 在前台时须由远端确认，仅后台运行时自动重启。')}</p>}
 </section>{confirmation&&<div className="modal" role="dialog" aria-modal="true" aria-labelledby="backend-update-title" ref={modal}><section className="update-dialog"><header><h2 id="backend-update-title">{t(title)} · {confirmation.version}</h2><button type="button" onClick={()=>setConfirmation(null)}>{t('关闭')}</button></header><p>{t(remote?'将远端 {name} 的完整应用和网络服务更新至 {version}。DNS 和 BGP 会短暂中断，配置与数据库保留。是否继续？':'将当前连接的后台所在 Mac 的完整应用和网络服务更新至 {version}。DNS 和 BGP 会短暂中断，配置与数据库保留。是否继续？',{name:backend?.name||location.origin,version:confirmation.version})}</p><details><summary>{t('更新说明')}</summary><div className="release-notes"><Suspense fallback={<p role="status">{t('处理中…')}</p>}><ReleaseNotes>{confirmation.notes||t('此版本未提供更新说明。')}</ReleaseNotes></Suspense></div></details><footer><button type="button" disabled={busy} onClick={skip}>{t('跳过该版本')}</button><button type="button" data-later onClick={()=>setConfirmation(null)}>{t('稍后升级')}</button><button type="button" className="primary" disabled={!authentication||!capable||busy} onClick={()=>install(confirmation.version)}>{t('立即升级')}</button></footer></section></div>}</>;
 return target?createPortal(content,target):content;

}
