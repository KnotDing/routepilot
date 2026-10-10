import React,{useEffect,useRef,useState} from 'react';
import {useT} from './i18n';
import {appPreferences,nativeApp,savePreferences} from './app-preferences';

const defaults={cliEnabled:false,automaticSkillInstall:false,skillTargets:['codex']};
const agents=[{id:'codex',name:'Codex'},{id:'claude',name:'Claude Code'},{id:'hermes',name:'Hermes'}];
const entryLabel=id=>id==='cli'?'CLI':(agents.find(agent=>agent.id===id)?.name||id)+' skill';
export function AutomationSettings({notify}){
 const t=useT(),[preferences,setPreferences]=useState(appPreferences),[busy,setBusy]=useState(false),[conflicts,setConflicts]=useState(null),[choices,setChoices]=useState({});
 const modal=useRef(null),pending=useRef(null);
 const draft={...defaults,...preferences.automation},supported=nativeApp()&&preferences.automationSupported===true&&Array.isArray(preferences.automation?.skillTargets),status=preferences.automationStatus||{},entries=status.entries||[];
 const missingTarget=draft.automaticSkillInstall&&!draft.skillTargets.length;
 useEffect(()=>{const changed=()=>setPreferences(appPreferences());window.addEventListener('routepilot-preferences-changed',changed);return()=>window.removeEventListener('routepilot-preferences-changed',changed)},[]);
 useEffect(()=>{
  if(!conflicts)return;
  const focus=document.activeElement,overflow=document.body.style.overflow;document.body.style.overflow='hidden';modal.current?.querySelector('[data-continue]')?.focus();
  const key=e=>{
   if(e.key==='Escape'&&!busy){e.preventDefault();setConflicts(null)}
   if(e.key==='Tab'){const items=[...modal.current.querySelectorAll('button,input')].filter(item=>!item.disabled),first=items[0],last=items.at(-1);if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus()}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus()}}
  };
  document.addEventListener('keydown',key);return()=>{document.body.style.overflow=overflow;document.removeEventListener('keydown',key);focus?.focus()};
 },[Boolean(conflicts),busy]);
 const set=(key,value)=>setPreferences({...preferences,automation:{...draft,[key]:value}});
 const commit=async(value,resolutions={})=>{
  if(busy||!supported)return;
  if(value.automation.automaticSkillInstall&&!value.automation.skillTargets.length){notify(t('至少选择一个 agent。'),true);return}
  setBusy(true);
  try{await savePreferences({...value,_automation:true,_automationChoices:resolutions});setConflicts(null);pending.current=null;notify(t('CLI 与 skill 设置已保存'))}
  catch(e){if(e.code==='automation_conflict'&&Array.isArray(e.conflicts)&&e.conflicts.length){pending.current=value;setConflicts(e.conflicts);setChoices(Object.fromEntries(e.conflicts.map(item=>[item.id,'overwrite'])))}else notify(e.message,true)}
  finally{setBusy(false)}
 };
 const save=e=>{e.preventDefault();commit({...preferences,automation:draft})};
 return <><section className="panel"><h2>{t('CLI 与 AI agent skill')}</h2><p>{t('设置作用于运行此管理窗口的本机 App，不会修改当前查看的远程后台或浏览器设备。')}</p>{!supported&&<p className="notice">{t(nativeApp()?'请更新完整 App，以使用 CLI 和 skill 管理功能。':'请在 RoutePilot App 中启用 CLI 和自动安装 skill。')}</p>}
  <form onSubmit={save}><fieldset disabled={!supported||busy} className="plain-fieldset"><div className="option-grid">
   <label className="check"><input type="checkbox" checked={draft.cliEnabled} onChange={e=>set('cliEnabled',e.target.checked)}/>{t('启用 CLI')}</label>
   <label className="check"><input type="checkbox" checked={draft.automaticSkillInstall} onChange={e=>set('automaticSkillInstall',e.target.checked)}/>{t('自动安装 AI agent skill')}</label>
  </div><p>{t('启用后安装 ~/.local/bin/routepilot；禁用后拒绝本机 CLI 控制请求，并移除本 App 管理的命令行入口。帮助和版本查询仍可使用。')}</p>
  {draft.automaticSkillInstall&&<fieldset className="plain-fieldset" aria-describedby="automation-target-hint"><legend className="field-caption"><span>{t('安装到哪些 agent（可多选）')}</span><span className="required-mark">{t('必填')}</span></legend><div className="option-grid">{agents.map(agent=><label className="check" key={agent.id}><input type="checkbox" checked={draft.skillTargets.includes(agent.id)} onChange={e=>set('skillTargets',e.target.checked?[...draft.skillTargets,agent.id]:draft.skillTargets.filter(id=>id!==agent.id))}/>{agent.name}</label>)}</div><p id="automation-target-hint" className={missingTarget?'notice error':'field-hint'}>{t(missingTarget?'至少选择一个 agent。':'可同时安装到多个 agent；取消勾选并保存会移除对应集成，恢复覆盖前的内容。')}</p></fieldset>}
  <p>{t('skill 自动安装后随 App 升级同步更新；关闭自动安装会移除本 App 管理的 skill，并恢复覆盖前的内容。CLI 默认关闭，需要先启用才能控制服务。')}</p>
  <div className="toolbar"><button className="primary" disabled={!supported||busy||missingTarget}>{t(busy?'正在保存…':'保存 CLI 与 skill 设置')}</button></div></fieldset></form>
  {entries.length>0&&<dl>{entries.map(entry=><React.Fragment key={entry.id}><dt>{entryLabel(entry.id)}</dt><dd><code>{entry.path}</code>{' · '}{t(entry.installed?'已安装':entry.skipped?'已跳过，保留原内容':'未安装')}</dd></React.Fragment>)}</dl>}
  {status.error&&<p className="notice error" role="alert">{status.error}</p>}
  <p>{t('请将 ~/.local/bin 加入 PATH；覆盖前会备份原内容，禁用时恢复。')}</p>{status.backupDirectory&&<p>{t('备份目录：{path}',{path:status.backupDirectory})}</p>}
 </section>{conflicts&&<div className="modal" role="dialog" aria-modal="true" aria-labelledby="automation-conflict-title" ref={modal}><form className="update-dialog" onSubmit={e=>{e.preventDefault();commit(pending.current,choices)}}><header><h2 id="automation-conflict-title">{t('CLI 或 skill 文件冲突')}</h2><button type="button" disabled={busy} onClick={()=>setConflicts(null)}>{t('关闭')}</button></header><p>{t('已有同名文件或目录。默认覆盖，覆盖前自动备份；也可以跳过并保留原内容。')}</p>{conflicts.map(item=><fieldset key={item.id} disabled={busy}><legend>{entryLabel(item.id)}</legend><p><code>{item.path}</code></p><div className="option-grid">{['overwrite','skip'].map(value=><label className="check" key={value}><input type="radio" name={'resolution-'+item.id} checked={choices[item.id]===value} onChange={()=>setChoices({...choices,[item.id]:value})}/>{t(value==='overwrite'?'覆盖':'跳过')}</label>)}</div></fieldset>)}<footer><button type="button" disabled={busy} onClick={()=>setConflicts(null)}>{t('取消')}</button><button className="primary" data-continue disabled={busy}>{t(busy?'正在保存…':'继续')}</button></footer></form></div>}</>;
}
