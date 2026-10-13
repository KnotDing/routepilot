import React,{useEffect,useRef} from 'react';
import events from '../../internal/config/notification-events.json';
import {useT} from './i18n';

function legacyCategory(name){return name.startsWith('Surge')?'surge':name.startsWith('BGP')||name.includes('routing')?'bgp':name.includes('DNS')?'dns':'controller'}
export function notificationSelected(name,settings={}){
 if(!Array.isArray(settings.errors))return Boolean(settings[legacyCategory(name)]);
 const event=events.find(event=>event.prefix?name.startsWith(event.name):name===event.name);
 return Boolean(event&&settings.errors.includes(event.id));
}
export function anomalyNotification(name){return events.some(event=>event.anomaly&&(event.prefix?name.startsWith(event.name):name===event.name))}
function GroupCheckbox({checked,mixed,onChange,children}){
 const ref=useRef(null);useEffect(()=>{if(ref.current)ref.current.indeterminate=mixed},[mixed]);
 return <label className="check"><input ref={ref} type="checkbox" checked={checked} onChange={onChange}/>{children}</label>;
}
export function NotificationErrors({settings,onChange,supportsAnomalies=true}){
 const t=useT(),available=events.filter(event=>supportsAnomalies||!event.anomaly),selected=new Set(available.filter(event=>notificationSelected(event.name,settings)).map(event=>event.id));
 const change=(ids,enabled)=>{const next=new Set(selected);for(const id of ids)enabled?next.add(id):next.delete(id);onChange(available.filter(event=>next.has(event.id)).map(event=>event.id))};
 return <section className="notification-errors"><h3>{t('发送通知的异常')}</h3><p>{t('只通知勾选的异常，系统通知与 Webhook 使用相同选择。取消全部后不会发送异常或恢复通知。')}</p><div className="toolbar"><button type="button" onClick={()=>onChange(available.map(event=>event.id))}>{t('全选')}</button><button type="button" onClick={()=>onChange([])}>{t('取消全部')}</button><span>{t('已选择 {count} / {total} 项',{count:selected.size,total:available.length})}</span></div><div className="form-grid">{Object.entries({surge:'Surge',bgp:'BGP 与路由',dns:'DNS',controller:'控制器与管理连接'}).map(([group,title])=>{const items=available.filter(event=>event.group===group),count=items.filter(event=>selected.has(event.id)).length;return <fieldset key={group}><legend><GroupCheckbox checked={count===items.length} mixed={count>0&&count<items.length} onChange={e=>change(items.map(event=>event.id),e.target.checked)}>{t(title)}</GroupCheckbox></legend>{items.map(event=><label className="check" key={event.id}><input type="checkbox" checked={selected.has(event.id)} onChange={e=>change([event.id],e.target.checked)}/>{t(event.title)}</label>)}</fieldset>})}</div></section>;
}
