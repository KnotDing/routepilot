import React,{useState} from 'react';
import {request} from './client';
import {useT,message} from './i18n';

export function ConfigurationApplyNotice({status,refresh,notify}){
 const t=useT(),[busy,setBusy]=useState(false);
 if(!status||!['pending','applying','failed'].includes(status.state))return null;
 const retry=async()=>{setBusy(true);try{await request('configuration/retry','POST',{});notify(t('已提交重新应用'));refresh()}catch(e){notify(e.message,true)}finally{setBusy(false)}};
 return <section className={status.state==='failed'?'notice error':'notice'} role="status">
  {status.state==='failed'?<><strong>{t('配置已保存，但后台应用失败。')}</strong> {message(status.error||'')} <button disabled={busy} onClick={retry}>{t('重新应用已保存配置')}</button></>:t('配置已保存，后台正在应用；服务重启和路由调整完成后，此提示会自动消失。')}
 </section>;
}
