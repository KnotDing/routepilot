import React from 'react';
import {FieldLabel} from './form-fields';
import {useT} from './i18n';

const defaults={cache_mismatch_percent:5,cache_window_seconds:60,cache_minimum_compared:100,burst_qps:500,burst_window_seconds:10,sustained_qps:100,sustained_seconds:300};
export function NotificationAnomalies({settings={},supported=false,onChange}){
 const t=useT();
 if(!supported)return <p>{t('当前后端不支持缓存与请求量告警，请升级后端后使用。')}</p>;
 const draft=Object.fromEntries(Object.entries(defaults).map(([key,value])=>[key,settings[key]===undefined||settings[key]===0?value:settings[key]]));
 const fields=[['cache_mismatch_percent','缓存不一致率阈值（%）',.1,100,.1],['cache_window_seconds','缓存核对窗口（秒）',1,3600,1],['cache_minimum_compared','缓存最少核对请求数',1,1000000,1],['burst_qps','突发请求阈值（请求/秒）',1,1000000,1],['burst_window_seconds','突发请求统计窗口（秒）',1,3600,1],['sustained_qps','持续请求阈值（请求/秒）',1,1000000,1],['sustained_seconds','大量请求持续时间（秒）',1,86400,1]];
 return <section className="notification-anomalies"><h3>{t('缓存与请求量告警阈值')}</h3><div className="form-grid">{fields.map(([key,label,min,max,step])=><FieldLabel key={key} label={t(label)} required><input type="number" min={min} max={max} step={step} value={draft[key]} onChange={e=>onChange({...draft,[key]:e.target.value===''?'':Number(e.target.value)})}/></FieldLabel>)}</div><p>{t('缓存按决策与响应分别核对，不一致率以窗口内已核对请求为分母，样本不足时不告警。突发请求按窗口平均速率判断，持续请求按每次实际采样速率及连续持续时间判断。暂停、重启或计数清零后重新采样。')}</p><p>{t('这些阈值只触发通知，不改变分流、路由或自动重启行为。')}</p></section>;
}
