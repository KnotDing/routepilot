import React,{useEffect,useRef,useState} from 'react';
import {AppIcon} from './app-icon';
import {NavIcon} from './nav-icon';
import {useT} from './i18n';

export function Navigation({pages,sections,page,onSelect,name,backend}){
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
 const brand=<div className="brand"><AppIcon/><div>RoutePilot<small>{t('DNS · 路由控制器')}</small></div></div>;
 const links=<nav aria-label={t('主导航')}>{pages.map(([key,label])=><React.Fragment key={key}>{sections[key]&&<div className="nav-heading">{t(sections[key])}</div>}<button type="button" onClick={()=>choose(key)} className={page===key?'active':''} aria-current={page===key?'page':undefined}><NavIcon page={key}/>{t(label)}</button></React.Fragment>)}</nav>;
 const foot=<div className="aside-foot">{name||'Controller'}<small>{backend?t('远程 · {name}',{name:backend.name}):t('管理连接 · 本地服务')}</small></div>;
 return <><aside className="desktop-sidebar">{brand}{links}{foot}</aside><div className="mobile-bar">{brand}<button type="button" ref={trigger} aria-label={t('打开导航')} aria-haspopup="dialog" aria-expanded={open} aria-controls="mobile-navigation" onClick={()=>setOpen(true)}><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16"/></svg>{t('导航')}</button></div><dialog ref={dialog} id="mobile-navigation" className="mobile-navigation" aria-label={t('主导航')} onClose={()=>setOpen(false)} onClick={e=>{if(e.target===e.currentTarget){const bounds=e.currentTarget.getBoundingClientRect();if(e.clientX<bounds.left||e.clientX>bounds.right||e.clientY<bounds.top||e.clientY>bounds.bottom)setOpen(false)}}}><header>{brand}<button type="button" onClick={()=>setOpen(false)}>{t('关闭')}</button></header>{links}{foot}</dialog></>;
}
