import React,{useEffect,useRef,useState} from 'react';
import {AppIcon} from './app-icon';
import {NavIcon} from './nav-icon';
import {useT} from './i18n';

export function Navigation({pages,sections,page,onSelect,backend,mode:routingMode='auto',onManage,onSignOut,notify,protectSensitive=false}){
 const t=useT(),[open,setOpen]=useState(false),[mode,setMode]=useState(()=>{const value=localStorage.getItem('routepilot.sidebar');return ['expanded','icons','hidden'].includes(value)?value:'expanded'}),[peek,setPeek]=useState(false),dialog=useRef(null),trigger=useRef(null);
 const changeMode=value=>{setMode(value);setPeek(false);try{localStorage.setItem('routepilot.sidebar',value)}catch(error){notify?.(t('侧边栏偏好保存失败：{error}',{error:error.message}),true)}};
 useEffect(()=>{
  if(!open)return;
  const sheet=dialog.current,previous=document.body.style.overflow;
  sheet.showModal();document.body.style.overflow='hidden';
  const wide=window.matchMedia('(min-width: 901px)');
  const resize=()=>{if(wide.matches)setOpen(false)};
  wide.addEventListener('change',resize);
  return()=>{wide.removeEventListener('change',resize);sheet.close();document.body.style.overflow=previous;trigger.current?.focus()};
 },[open]);
 useEffect(()=>{const resize=()=>setPeek(false);window.addEventListener('resize',resize);return()=>window.removeEventListener('resize',resize)},[]);
 const choose=key=>{onSelect(key);setOpen(false);setPeek(false);window.scrollTo({top:0,left:0,behavior:'instant'})};
 const modeLabels={auto:t('自动分流'),'force-direct':t('强制直连'),'force-surge':t('全部交给 Surge')};
 const act=action=>{setOpen(false);action?.()};
 const brand=<div className="brand"><AppIcon/><div>RoutePilot<small>{t('DNS · 路由控制器')}</small></div></div>;
 const links=<nav aria-label={t('主导航')}>{pages.map(([key,label])=><React.Fragment key={key}>{sections[key]&&<div className="nav-heading"><span>{t(sections[key])}</span>{key==='Dashboard'&&<span className={'nav-mode '+(routingMode!=='auto'?'mode-overridden':'')} aria-label={t('当前分流模式')} title={t('分流模式在设置中的路由分类调整')}>{modeLabels[routingMode]||routingMode}</span>}</div>}<button type="button" onClick={()=>choose(key)} className={page===key?'active':''} aria-current={page===key?'page':undefined} aria-label={t(label)} title={t(label)}><NavIcon page={key}/><span className="nav-label">{t(label)}</span></button></React.Fragment>)}</nav>;
 const connectionName=protectSensitive?t('当前后端'):backend?.name||t('本地服务');
 const collapseLabel=t(mode==='icons'?'展开侧边栏':'收起侧边栏'),hideLabel=t(mode==='hidden'?'固定侧边栏':'隐藏侧边栏');
 const collapseButton=<button type="button" className="sidebar-collapse" aria-label={collapseLabel} title={collapseLabel} onClick={()=>changeMode(mode==='icons'?'expanded':'icons')}><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d={mode==='icons'?'m9 5 7 7-7 7':'m15 5-7 7 7 7'}/></svg></button>;
 const foot=desktop=><div className="aside-foot"><button type="button" className="sidebar-connection" disabled={!onManage} onClick={()=>act(onManage)} aria-label={t('{name} · 切换',{name:connectionName})} title={t('管理后端')}><span className="nav-label">{connectionName}</span><span aria-hidden="true">↔</span></button>{!protectSensitive&&<small className="sidebar-address" title={backend?.url||location.origin}>{backend?.url||location.origin}</small>}<div className="sidebar-footer-actions"><button type="button" className="sidebar-signout" aria-label={t('退出')} title={t('退出')} onClick={()=>act(onSignOut)}><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d="M10 4H4v16h6M9 12h12m-4-4 4 4-4 4"/></svg><span className="nav-label">{t('退出')}</span></button>{desktop&&collapseButton}</div></div>;
 return <><aside className={'desktop-sidebar '+(mode==='icons'?'sidebar-icons':'')+(mode==='hidden'?' sidebar-floating':'')} hidden={mode==='hidden'&&!peek} onMouseLeave={()=>{if(mode==='hidden')setPeek(false)}} onBlur={event=>{if(mode==='hidden'&&!event.currentTarget.contains(event.relatedTarget))setPeek(false)}} onKeyDown={event=>{if(mode==='hidden'&&event.key==='Escape')setPeek(false)}}>{brand}<div className="sidebar-controls"><button type="button" aria-label={hideLabel} title={hideLabel} onClick={()=>changeMode(mode==='hidden'?'expanded':'hidden')}><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d={mode==='hidden'?'M8 3h8l-1 6 3 4H6l3-4-1-6M12 13v8':'M3 4h18v16H3zM8 4v16m7-12-3 4 3 4'}/></svg></button></div>{links}{foot(true)}</aside>{mode==='hidden'&&<button type="button" className="sidebar-reveal" aria-label={t('显示侧边栏')} aria-expanded={peek} onMouseEnter={()=>setPeek(true)} onMouseLeave={event=>{if(!event.relatedTarget?.closest?.('.desktop-sidebar'))setPeek(false)}} onFocus={()=>setPeek(true)} onClick={()=>setPeek(true)}/>}<div className="mobile-bar">{brand}<button type="button" ref={trigger} aria-label={t('打开导航')} aria-haspopup="dialog" aria-expanded={open} aria-controls="mobile-navigation" onClick={()=>setOpen(true)}><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16"/></svg>{t('导航')}</button></div><dialog ref={dialog} id="mobile-navigation" className="mobile-navigation" aria-label={t('主导航')} onClose={()=>setOpen(false)} onClick={e=>{if(e.target===e.currentTarget){const bounds=e.currentTarget.getBoundingClientRect();if(e.clientX<bounds.left||e.clientX>bounds.right||e.clientY<bounds.top||e.clientY>bounds.bottom)setOpen(false)}}}><header>{brand}<button type="button" onClick={()=>setOpen(false)}>{t('关闭')}</button></header>{links}{foot(false)}</dialog></>;
}
