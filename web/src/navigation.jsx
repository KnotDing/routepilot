import React,{useEffect,useRef,useState} from 'react';
import {AppIcon} from './app-icon';
import {NavIcon} from './nav-icon';
import {useT} from './i18n';

export function Navigation({pages,sections,page,onSelect,backend,mode='auto',onManage,onSignOut}){
 const t=useT(),[open,setOpen]=useState(false),dialog=useRef(null),trigger=useRef(null);
 useEffect(()=>{
  if(!open)return;
  const sheet=dialog.current,previous=document.body.style.overflow;
  sheet.showModal();document.body.style.overflow='hidden';
  const wide=window.matchMedia('(min-width: 901px)');
  const resize=()=>{if(wide.matches)setOpen(false)};
  wide.addEventListener('change',resize);
  return()=>{wide.removeEventListener('change',resize);sheet.close();document.body.style.overflow=previous;trigger.current?.focus()};
 },[open]);
 const choose=key=>{onSelect(key);setOpen(false);window.scrollTo({top:0,left:0,behavior:'instant'})};
 const modeLabels={auto:t('自动分流'),'force-direct':t('强制直连'),'force-surge':t('全部交给 Surge')};
 const act=action=>{setOpen(false);action?.()};
 const brand=<div className="brand"><AppIcon/><div>RoutePilot<small>{t('DNS · 路由控制器')}</small></div></div>;
 const links=<nav aria-label={t('主导航')}>{pages.map(([key,label])=><React.Fragment key={key}>{sections[key]&&<div className="nav-heading"><span>{t(sections[key])}</span>{key==='Dashboard'&&<span className={'nav-mode '+(mode!=='auto'?'mode-overridden':'')} aria-label={t('当前分流模式')} title={t('分流模式在设置中的路由分类调整')}>{modeLabels[mode]||mode}</span>}</div>}<button type="button" onClick={()=>choose(key)} className={page===key?'active':''} aria-current={page===key?'page':undefined}><NavIcon page={key}/>{t(label)}</button></React.Fragment>)}</nav>;
 const connectionName=backend?.name||t('本地服务');
 const foot=<div className="aside-foot"><button type="button" className="sidebar-connection" disabled={!onManage} onClick={()=>act(onManage)} aria-label={t('{name} · 切换',{name:connectionName})} title={t('管理后端')}><span>{connectionName}</span><span aria-hidden="true">↔</span></button><small className="sidebar-address" title={backend?.url||location.origin}>{backend?.url||location.origin}</small><button type="button" className="sidebar-signout" onClick={()=>act(onSignOut)}>{t('退出')}</button></div>;
 return <><aside className="desktop-sidebar">{brand}{links}{foot}</aside><div className="mobile-bar">{brand}<button type="button" ref={trigger} aria-label={t('打开导航')} aria-haspopup="dialog" aria-expanded={open} aria-controls="mobile-navigation" onClick={()=>setOpen(true)}><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16"/></svg>{t('导航')}</button></div><dialog ref={dialog} id="mobile-navigation" className="mobile-navigation" aria-label={t('主导航')} onClose={()=>setOpen(false)} onClick={e=>{if(e.target===e.currentTarget){const bounds=e.currentTarget.getBoundingClientRect();if(e.clientX<bounds.left||e.clientX>bounds.right||e.clientY<bounds.top||e.clientY>bounds.bottom)setOpen(false)}}}><header>{brand}<button type="button" onClick={()=>setOpen(false)}>{t('关闭')}</button></header>{links}{foot}</dialog></>;
}
