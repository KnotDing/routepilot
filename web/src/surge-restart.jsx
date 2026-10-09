import React from 'react';
import {FieldLabel} from './form-fields';
import {useT,message} from './i18n';
export const restartDefaults={enabled:false,api:false,dns:false,groups:[],nodes:[],probe_url:'https://www.apple.com/library/test/success.html',probe_timeout_ms:3000,failures:5,duration_seconds:15,startup_grace_seconds:60,cooldown_seconds:300,max_per_hour:3};
export function SurgeRestartSettings({value,onChange,apiURL,status}){
 const t=useT(),draft={...restartDefaults,...value};
 for(const key of ['probe_timeout_ms','failures','duration_seconds','startup_grace_seconds','cooldown_seconds','max_per_hour','probe_url'])if(draft[key]===0||draft[key]==null||key==='probe_url'&&draft[key]==='')draft[key]=restartDefaults[key];
 const set=(key,value)=>onChange({...draft,[key]:value});
 const checkbox=(key,label)=><label className="check"><input type="checkbox" checked={draft[key]} onChange={e=>set(key,e.target.checked)}/>{t(label)}</label>;
 const ranges={probe_timeout_ms:[500,15000,'策略探测超时（毫秒）'],failures:[2,100,'连续失败次数'],duration_seconds:[5,3600,'持续异常时间（秒）'],startup_grace_seconds:[10,3600,'启动宽限时间（秒）'],cooldown_seconds:[30,86400,'重启冷却时间（秒）'],max_per_hour:[1,10,'每小时最多重启次数']};
 return <section className="settings-section" aria-label={t('Surge 异常重启')}><h3>{t('Surge 异常重启')}</h3><div className="option-grid">{checkbox('enabled','启用 Surge 异常重启')}</div>{draft.enabled&&<>
 <p>{t('任一选中条件同时达到连续失败次数和持续异常时间后重启；恢复成功会清零该条件的失败记录。')}</p>
 <div className="option-grid">{checkbox('api','HTTP API 不可用时重启')}{checkbox('dns','Surge DNS 不可用时重启')}</div>
 {draft.api&&!apiURL&&<p className="notice error" role="alert">{t('请先填写 Surge HTTP API 地址。')}</p>}
 <div className="form-grid">{[['groups','指定策略组（每行一项）'],['nodes','指定节点（每行一项）']].map(([key,label])=><FieldLabel key={key} label={t(label)} hint={t('使用 Surge 配置中的准确名称；留空不检查。')}><textarea rows="3" value={(draft[key]||[]).join('\n')} onChange={e=>set(key,e.target.value.split('\n'))}/></FieldLabel>)}
 {(draft.groups?.some(name=>name.trim())||draft.nodes?.some(name=>name.trim()))&&<FieldLabel label={t('策略健康探测 URL')} required><input type="url" value={draft.probe_url} onChange={e=>set('probe_url',e.target.value)}/></FieldLabel>}
 {Object.entries(ranges).map(([key,[min,max,label]])=><FieldLabel key={key} label={t(label)} required><input type="number" min={min} max={max} value={draft[key]} onChange={e=>set(key,e.target.value===''?'':Number(e.target.value))}/></FieldLabel>)}
 </div>{!draft.api&&!draft.dns&&!draft.groups?.some(name=>name.trim())&&!draft.nodes?.some(name=>name.trim())&&<p className="notice error" role="alert">{t('请至少选择一个异常判定条件。')}</p>}
 <p>{t('策略组和节点通过 Surge 发起 HTTP HEAD 探测，返回 2xx / 3xx 视为可用。重启使用 restart-engine，会中断现有连接并清理 Surge 自身缓存；CLI 无法响应时记录失败并等待冷却，不强制结束进程。')}</p>
 </>}{status?.last_attempt&&!String(status.last_attempt).startsWith('0001-')&&<p role="status">{t(status.running?'正在重启 Surge 引擎':'最近重启尝试：{time}',{time:new Date(status.last_attempt).toLocaleString()})}{status.last_error&&' · '+t('重启失败')+'：'+message(status.last_error)}{status.attempts_last_hour>0&&' · '+t('最近一小时 {count} 次',{count:status.attempts_last_hour})}</p>}</section>;
}
