import React,{useState} from 'react';
import {request} from './client';
import {saveSettings} from './configuration';
import {useT} from './i18n';
import {routingText} from './routing-i18n';
import {useLocale} from './locale';

const canConfirm=routing=>Boolean(routing?.static_dns_interception&&(routing.mode==='static'||routing.mode==='hybrid'&&routing.dns_backend==='static'));

export function DNSInterceptionConfirmation({settings,refresh,notify}){
 const [locale]=useLocale(),t=routingText(locale),ui=useT();
 const [saving,setSaving]=useState(false);
 if(!canConfirm(settings?.routing))return null;
 const save=async()=>{
  if(saving||settings.routing.static_dns_configured)return;
  setSaving(true);
  try{
   // Read the connected backend's current settings before changing this flag.
   const latest=await request('settings');
   if(!canConfirm(latest.routing))throw new Error(t('dnsConfigurationChanged'));
   const saved=await saveSettings({...latest,routing:{...latest.routing,static_dns_configured:true}});
   if(!saved.settings.routing.static_dns_configured)throw new Error(t('confirmReset'));
   notify(t('dnsConfirmationSaved'));
   await refresh();
  }catch(e){if(e.name!=='AbortError')notify(e.message,true)}finally{setSaving(false)}
 };
 return <div className="notice dns-interception-confirmation"><button type="button" className="primary" disabled={saving||Boolean(settings.routing.static_dns_configured)} onClick={save}>{t(settings.routing.static_dns_configured?'dnsConfirmed':'dnsConfirmAction')}</button><p className="muted">{t('dnsConfirmationHint')}{' '}{t('staticRisk')}</p>{saving&&<span role="status">{ui('正在保存…')}</span>}</div>;
}
