import React,{useId} from 'react';
import {useT} from './i18n';

export function FieldLabel({label,required=false,hint,children,className='',...props}){
 const t=useT(),id=useId(),control=React.Children.only(children);
 const value=control.props.value;
 const empty=value==null||String(value).trim()==='';
 return <label {...props} className={className} data-required={required?'true':undefined} data-missing={required&&empty?'true':undefined}>
  <span className="field-caption"><span>{label}</span>{required&&<span className="required-mark" aria-hidden="true">{t('必填')}</span>}</span>
  {React.cloneElement(control,{required,'aria-label':control.props['aria-label']||label,...hint?{'aria-describedby':id}:{}})}
  {hint&&<small className="field-hint" id={id}>{hint}</small>}
 </label>;
}
export function FormHint(){const t=useT();return <p className="form-guidance">{t('标有“必填”的项目需要填写；开启相关功能后，必填提示会自动变化。')}</p>}
const numbers=new Set('timeout_ms source_port_first source_port_last local_asn listen_port route_limit hold_seconds keepalive_seconds capacity response_capacity workers queue direct_ttl_seconds surge_ttl_seconds unknown_ttl_seconds error_ttl_seconds interval_ms failures successes grace_seconds stabilize_seconds history_capacity event_retention session_minutes cooldown_seconds asn port ttl med local_preference checkIntervalHours gateway_asn'.split(' '));
export const numericField=name=>numbers.has(name);
export function resourceRequired(page,name){return numericField(name)||({Hosts:['domain','addresses','match'],Interception:['prefix'],Peers:['address'],Routes:['prefix','next_hop'],Overrides:['domain','match','decision']}[page]||[]).includes(name)}
export function settingsRequirement(category,name,draft,interception=[]){
 const section=draft[category]||{},dns=draft.dns||{},routing=draft.routing||{},surge=draft.surge||{},bgp=draft.bgp||{};
 const result={required:numericField(name)};
 const fields={general:['web_listen','mode','theme'],dns:['listeners','aaaa_fallback'],surge:['cli_path','dns','fake_prefixes'],security:['allowed_networks'],health:['probe_domain'],logging:['level']};
 if(fields[category]?.includes(name))result.required=true;
 if(category==='bgp'&&name==='router_id'){result.required=Boolean(bgp.enabled&&routing.mode!=='static');if(result.required)result.hint='启用 BGP 后需要有效的 IPv4 Router ID。'}
 if(category==='security'){
  if(name==='username')result.required=Boolean(section.authentication);
  if(name==='password'){result.required=Boolean(section.authentication&&!section.password_configured&&!section.password_hash);result.minLength=12;result.maxLength=72;if(result.required)result.hint='首次启用认证需要设置密码；已有密码时留空保留。'}
  if(name==='tls_cert'||name==='tls_key'){result.required=Boolean(section.tls_cert||section.tls_key);if(result.required)result.hint='使用 HTTPS 时，证书与私钥需要同时填写。'}
 }
 if(category==='dns'){
  if(['transparent_listeners','ingress_interfaces'].includes(name)){result.required=Boolean(dns.interception_enabled);result.disabled=!dns.interception_enabled;if(result.required)result.hint='启用透明 DNS 拦截后必填。'}
  if(name==='egress_interface'){result.required=Boolean(dns.interception_enabled);if(result.required)result.hint='启用透明 DNS 拦截后必填。'}
  const enabled=interception.filter(row=>row.enabled).map(row=>row.prefix);
  const families=enabled.length?enabled:(dns.transparent_listeners||[]);
  for(const family of [4,6])if(name==='source'+family||name==='gateway'+family){result.required=Boolean((dns.interception_enabled||name.startsWith('source')&&routing.static_dns_configured)&&families.some(value=>value.includes(':')===(family===6)));if(result.required)result.hint='当前拦截地址族需要源地址与网关。'}
  if(name==='bypass_verified')result.required=Boolean(dns.interception_enabled);
 }
 if(category==='surge'&&name==='api_url'&&surge.restart?.enabled&&surge.restart?.api)result.required=true;
 if(category==='surge'&&(name==='next_hop4'||name==='next_hop6')){
  const backend=routing.mode==='hybrid'?routing.fake_ip_backend:(routing.mode||'bgp');
  const active=backend==='bgp'?bgp.enabled:routing.static_fake_ip_configured;
  const family=name.endsWith('6')?6:4;
  result.required=Boolean(active&&(surge.fake_prefixes||[]).some(prefix=>prefix.includes(':')===(family===6)));
  if(result.required)result.hint='当前 Fake-IP 路由地址族需要 Surge 下一跳。';
 }
 return result;
}
