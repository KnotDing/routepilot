import React,{useEffect,useRef,useState} from 'react';
import {request} from './client';
import {useT,message} from './i18n';

const active=phase=>['downloading','validating','restarting'].includes(phase);
const phases={idle:'尚未检查后台更新',downloading:'正在下载后台更新',validating:'正在校验后台更新',restarting:'正在重启后台，请稍候',succeeded:'后台升级已完成',failed:'后台升级失败'};
export function BackendUpdate({backend,capable,software,onReady}){
 const t=useT(),[update,setUpdate]=useState(null),[error,setError]=useState(''),[busy,setBusy]=useState(false),[offline,setOffline]=useState(false);
 const mounted=useRef(true),polling=useRef(false),checked=useRef(false),readyCallback=useRef(onReady);readyCallback.current=onReady;
 useEffect(()=>()=>{mounted.current=false},[]);
 const check=async()=>{if(busy)return;setBusy(true);try{const value=await request('updates/check');if(mounted.current){setUpdate(value);setError('')}}catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}finally{if(mounted.current)setBusy(false)}};
 useEffect(()=>{if(capable&&!checked.current){checked.current=true;check()}},[capable]);
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
 const install=async()=>{
  if(!update?.available||busy||active(update.phase))return;
  if(!window.confirm(t('将远程后台 {name} 更新至 {version}。DNS 和 BGP 会短暂中断，配置与数据库保留。是否继续？',{name:backend.name,version:update.latest})))return;
  setBusy(true);try{const value=await request('updates/install','POST',{version:update.latest});if(mounted.current){setUpdate(value);setError('')}}catch(e){if(mounted.current&&e.name!=='AbortError')setError(message(e.message))}finally{if(mounted.current)setBusy(false)}
 };
 if(!capable&&!update&&!error)return null;
 if(update&&!update.available&&!active(update.phase)&&update.phase!=='failed'&&!update.error&&!error)return null;
 return <section className={'notice backend-update '+(update?.phase==='failed'?'error':'')} aria-label={t('远程后台更新')}>
  <strong>{t('远程后台更新')}</strong><p>{t('当前后台：{version}',{version:software?.version||update?.current||'—'})}{update?.latest&&' · '+t('最新发布：{version}',{version:update.latest})}</p>
  {update&&<p role="status">{offline?t('后台暂时离线，正在等待升级完成并重连…'):active(update.phase)||['succeeded','failed'].includes(update.phase)?t(phases[update.phase]):update.available?t('有可用的后台新版本'):t('后台已是最新版本')}</p>}
  {update?.reason&&<p>{message(update.reason)}</p>}{update?.error&&<p role="alert">{message(update.error)}</p>}{error&&<p role="alert">{error}</p>}
  <div className="toolbar"><button type="button" disabled={busy||active(update?.phase)} onClick={check}>{t(busy?'处理中…':'检查后台更新')}</button>{update?.supported&&update.available&&<button type="button" className="primary" disabled={busy||active(update.phase)} onClick={install}>{t('升级远程后台')}</button>}</div>
  <p>{t('由远端从项目 Release 下载并校验安装包，升级后自动重启服务；首次需在远端安装支持在线升级的后台。')}</p>
 </section>;
}
