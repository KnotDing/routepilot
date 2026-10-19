import React,{useState} from 'react';
import {FieldLabel} from './form-fields';
import {routingText} from './routing-i18n';
import {useLocale} from './locale';

const presetAuth={openwrt:'ubus',unifi:'header',custom:'none'};
const presetBase={openwrt:'http://192.168.1.1',unifi:'https://192.168.1.1',custom:''};

// HeadersField keeps JSON text locally and commits a parsed object on blur so
// half-finished edits never reach the draft. It remounts when the saved value
// changes after a round trip.
function HeadersField({label,value,onChange,disabled}){
 const t=routingText(useLocale()[0]);
 const [text,setText]=useState(()=>JSON.stringify(value||{},null,2));
 const [error,setError]=useState(false);
 const commit=()=>{
  const trimmed=text.trim();
  if(!trimmed){setError(false);onChange(undefined);return}
  try{
   const parsed=JSON.parse(trimmed);
   if(typeof parsed!=='object'||parsed===null||Array.isArray(parsed)||Object.keys(parsed).length>16)throw new Error('shape');
   setError(false);onChange(parsed);
  }catch{setError(true)}
 };
 return <>
  <FieldLabel label={label}><textarea rows={4} disabled={disabled} aria-invalid={error||undefined} value={text} onChange={e=>setText(e.target.value)} onBlur={commit}/></FieldLabel>
  {error&&<p role="alert">{t('headersInvalid')}</p>}
 </>;
}
function BodyField({label,value,onChange,disabled}){
 const t=routingText(useLocale()[0]);
 const [text,setText]=useState(()=>value||'');
 const [error,setError]=useState(false);
 const commit=()=>{
  const trimmed=text.trim();
  if(!trimmed){setError(false);onChange('');return}
  try{JSON.parse(trimmed.replace(/\{\{\s*([a-z_]+)\s*\}\}/g,'"__var__"'));setError(false);onChange(text)}catch{setError(true)}
 };
 return <>
  <FieldLabel label={label}><textarea rows={4} disabled={disabled} aria-invalid={error||undefined} value={text} onChange={e=>setText(e.target.value)} onBlur={commit}/></FieldLabel>
  {error&&<p role="alert">{t('bodyInvalid')}</p>}
 </>;
}
const templateState=q=>({method:q?.method||'POST',path:q?.path||'',headers:q?.headers,body:q?.body||'',items_path:q?.items_path||'',id_field:q?.id_field||'',name_field:q?.name_field||'',prefix_field:q?.prefix_field||'',next_hop_field:q?.next_hop_field||''});

export function RoutingAPISettings({draft={},onChange,disabled=false}){
 const [locale]=useLocale(),t=routingText(locale);
 const preset=draft.preset||'';
 const auth=draft.auth||presetAuth[preset]||'none';
 const edit=(key,value)=>onChange({...draft,[key]:value});
 const editPreset=value=>{
  const next={...draft,preset:value,auth:presetAuth[value]||'none'};
  if(value&&!next.base_url)next.base_url=presetBase[value];
  onChange(next);
 };
 const editTemplate=(role,key,value)=>onChange({...draft,[role]:{...templateState(draft[role]),[key]:value}});
 const ubus=auth==='ubus',header=auth==='header';
 const methodOptions=['GET','POST','PUT','PATCH','DELETE'];
 const templateEditor=(role,title)=>{
  const q=templateState(draft[role]);
  return <details className="panel routing-api-template" open={role==='add'}>
   <summary>{t(title)}</summary>
   <div className="form-grid">
    <FieldLabel label={t('method')} required><select aria-label={t('method')} value={q.method} onChange={e=>editTemplate(role,'method',e.target.value)}>{methodOptions.map(m=><option key={m}>{m}</option>)}</select></FieldLabel>
    <FieldLabel label={t('path')} required><input aria-label={t('path')} required value={q.path} onChange={e=>editTemplate(role,'path',e.target.value)} placeholder="/api/routes"/></FieldLabel>
   </div>
   <HeadersField key={role+'-headers-'+JSON.stringify(q.headers)} label={t('headersText')} value={q.headers} onChange={headers=>editTemplate(role,'headers',headers)} disabled={disabled}/>
   <BodyField key={role+'-body-'+(draft[role]?.body||'')} label={t('bodyText')} value={q.body} onChange={body=>editTemplate(role,'body',body)} disabled={disabled}/>
   {role==='list'&&<div className="form-grid">
    <FieldLabel label={t('itemsPath')} hint={t('itemsPathHint')}><input aria-label={t('itemsPath')} value={q.items_path} onChange={e=>editTemplate(role,'items_path',e.target.value)} placeholder="data.routes"/></FieldLabel>
    <FieldLabel label={t('idField')}><input aria-label={t('idField')} value={q.id_field} onChange={e=>editTemplate(role,'id_field',e.target.value)} placeholder="id"/></FieldLabel>
    <FieldLabel label={t('nameField')}><input aria-label={t('nameField')} value={q.name_field} onChange={e=>editTemplate(role,'name_field',e.target.value)} placeholder="name"/></FieldLabel>
    <FieldLabel label={t('prefixField')}><input aria-label={t('prefixField')} value={q.prefix_field} onChange={e=>editTemplate(role,'prefix_field',e.target.value)} placeholder="network"/></FieldLabel>
    <FieldLabel label={t('nextHopField')}><input aria-label={t('nextHopField')} value={q.next_hop_field} onChange={e=>editTemplate(role,'next_hop_field',e.target.value)} placeholder="nextHop"/></FieldLabel>
   </div>}
  </details>;
 };
 return <section className="panel"><h3>{t('apiSectionTitle')}</h3>
  <div className="form-grid">
   <FieldLabel label={t('preset')} required><select aria-label={t('preset')} required value={preset} onChange={e=>editPreset(e.target.value)}>
    <option value="">{t('choose')}</option>
    <option value="openwrt">OpenWrt</option>
    <option value="unifi">UniFi OS</option>
    <option value="custom">{t('customPreset')}</option>
   </select></FieldLabel>
   <FieldLabel label={t('baseURL')} required hint={t('baseURLHint')}><input aria-label={t('baseURL')} required type="url" value={draft.base_url||''} onChange={e=>edit('base_url',e.target.value)} placeholder="http://192.168.1.1"/></FieldLabel>
  </div>
  {preset==='custom'&&<div className="form-grid"><FieldLabel label={t('authMode')}><select aria-label={t('authMode')} value={auth} onChange={e=>edit('auth',e.target.value)}>
   <option value="none">{t('authNone')}</option>
   <option value="header">{t('authHeader')}</option>
   <option value="ubus">{t('authUbus')}</option>
  </select></FieldLabel></div>}
  {ubus&&<div className="form-grid">
   <FieldLabel label={t('username')} required><input aria-label={t('username')} required autoComplete="off" value={draft.username||''} onChange={e=>edit('username',e.target.value)}/></FieldLabel>
   <FieldLabel label={t('password')} hint={draft.password_configured?t('secretConfigured'):undefined}><input aria-label={t('password')} type="password" autoComplete="new-password" value={draft.password||''} onChange={e=>edit('password',e.target.value)}/></FieldLabel>
  </div>}
  {header&&<div className="form-grid">
   <FieldLabel label={t('headerName')} required><input aria-label={t('headerName')} required value={draft.header_name||''} onChange={e=>edit('header_name',e.target.value)} placeholder="X-API-KEY"/></FieldLabel>
   <FieldLabel label={t('headerValue')} hint={draft.header_configured?t('secretConfigured'):undefined}><input aria-label={t('headerValue')} type="password" autoComplete="new-password" value={draft.header_value||''} onChange={e=>edit('header_value',e.target.value)}/></FieldLabel>
  </div>}
  <div className="option-grid">
   <label className="check"><input type="checkbox" checked={Boolean(draft.verify_tls)} onChange={e=>edit('verify_tls',e.target.checked)}/>{t('verifyTLS')}</label>
   {ubus&&draft.password_configured&&<label className="check"><input type="checkbox" checked={Boolean(draft.clear_password)} onChange={e=>edit('clear_password',e.target.checked)}/>{t('clearPassword')}</label>}
   {header&&draft.header_configured&&<label className="check"><input type="checkbox" checked={Boolean(draft.clear_header)} onChange={e=>edit('clear_header',e.target.checked)}/>{t('clearHeader')}</label>}
  </div>
  {preset==='openwrt'&&<p>{t('openwrtHint')}</p>}
  {preset==='unifi'&&<p>{t('unifiHint')}</p>}
  {preset==='custom'&&<><p>{t('customHint')}</p><p className="routing-api-vars">{t('variables')}：<code>{'{{name}} {{prefix}} {{next_hop}} {{target}} {{netmask}} {{route_type}} {{values}} {{id}} {{section}} {{session}} {{api_key}}'}</code></p>
   {templateEditor('add','addTemplate')}
   {templateEditor('remove','removeTemplate')}
   {templateEditor('list','listTemplate')}
  </>}
  <p className="routing-limit">{t('apiLimit')}</p>
 </section>;
}
