import React from 'react';
import {useT,message} from './i18n';

export function SurgeNextHop4Status({value,status}){
 const t=useT();
 if(value)return null;
 if(!status)return <small className="field-hint">{t('当前后端未提供 IPv4 下一跳探测状态。')}</small>;
 if(!status.automatic)return <small className="field-hint">{t('保存设置后将自动探测 Surge VM IPv4 地址。')}</small>;
 if(status.error)return <small className="field-hint" role="alert">{t('IPv4 下一跳自动探测失败：{reason}',{reason:message(status.error)})}</small>;
 if(status.pending)return <small className="field-hint" role="status">{t('已探测到 {address}，等待旧 Fake-IP 响应到期后应用。',{address:status.detected_address})}</small>;
 return <small className="field-hint" role="status">{status.address?t('自动探测的 Surge IPv4 下一跳：{address}',{address:status.address}):t('正在等待 Surge VM IPv4 地址探测。')}</small>;
}
