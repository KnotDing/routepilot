import React,{useEffect,useRef,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {request,requestOptional,setCSRF,setLocalCSRF,localRequest,selectBackend,isRemote,pretty,reconnectAfterApplication} from './client';
import {saveSettings} from './configuration';
import {FieldLabel,FormHint,numericField,resourceRequired,settingsRequirement} from './form-fields';
import {DNSForwardingOverview} from './dns-overview';
import './style.css';
import {FRRExport} from './frr';
import {RouteImport} from './route-import';
import {BGPPeerDetails} from './bgp-peer-details';
import {DNSResult} from './dns-result';
import {RuleDNS} from './rule-dns';
import {OverridesTable} from './overrides';
import {Navigation} from './navigation';
import {CachePanel,OptimisticReliability} from './cache';
import {Backends} from './backends';
import {remoteCredentialsSupported,restoreRemoteLogin,restoreLocalLogin,rememberManagementBackend,saveRemoteCredential} from './credentials';
import {About} from './about';
import {AppPreferencesPanel,GitHubAccessTokenControls,LocalAppUpdateControls,appPreferences,savePreferences,usePreferenceDraft,observeServices,nativeApp} from './app-preferences';
import {NotificationErrors} from './notification-errors';
import {VersionNotice} from './version-notice';
import {BackendUpdate} from './backend-update';
import {OperationNotice} from './operation-status';
import {ConfigurationApplyNotice} from './configuration-status';
import {SurgeRestartSettings} from './surge-restart';
import {StatusBarSettings} from './status-bar-settings';
import {AutomationSettings} from './automation-settings';
import {DNSSettingsFields} from './dns-settings';
import {PolicyDNSEditor,PolicyDNSResult} from './policy-dns';
import {useLocale,currentLocale} from './locale';
import {useT,translate,message,valueLabel} from './i18n';
import {AppIcon} from './app-icon';
import {LanguageSelector} from './language';
import {cacheText} from './cache-i18n';
import {RoutingPanel,RoutingSettingsFields} from './routing';
import {ClientsPanel} from './clients';
import {MonitoringPanel} from './monitoring';
import {useWindowVisible} from './window-visibility';
export {RoutingPanel} from './routing';

const pages=[['Dashboard','仪表盘'],['Monitor','大屏监控'],['DNS','DNS 概览'],['Queries','查询记录'],['Cache','决策缓存'],['Clients','客户端'],['Interception','DNS 拦截'],['Hosts','本地 DNS'],['Overrides','域名覆盖'],['Rule','规则测试'],['Routing','路由'],['BGP','BGP 概览'],['Peers','BGP 邻居'],['Routes','手动路由'],['Health','健康状态'],['Events','事件'],['Settings','设置']];
const pageRequests={
 Monitor:[],
 Dashboard:['health','bgp/status','routing/status','bgp/peers','events','surge/status'],
 DNS:['surge/status'],
 Queries:['dns/queries'],
 Clients:['dns/clients','dns/client-settings'],
 Cache:[],
 Interception:['health','dns/interception'],
 Hosts:['dns/hosts'],
 Overrides:['overrides','surge/status'],
 Rule:[],
 Routing:['routing/status'],
 BGP:['bgp/status'],
 Peers:['bgp/peers','bgp/status'],
 Routes:['bgp/routes','bgp/status'],
 Health:['health','bgp/status','bgp/peers'],
 Events:['events'],
 Settings:['dns/interception']
};
const descriptions={Monitor:'查看策略命中、DNS 流向与请求占比',Dashboard:'查看 DNS 分流、服务健康与路由运行状态',DNS:'查看查询统计与真实 DNS 上游',Queries:'追踪每次 DNS 查询的策略判断和解析结果',Cache:'查看分流决策缓存并按域名清理',Clients:'查看发起 DNS 请求的客户端及其统计信息',Interception:'管理透明 DNS 拦截的目标地址',Hosts:'管理本地 DNS 条目与 Surge Hosts 导入',Overrides:'管理手动域名策略并查看 Surge always-real-ip 条目',Rule:'查看 Surge 对指定连接的规则求值结果',Routing:'查看路由发布、配置状态与本机连通性诊断',BGP:'查看 BGP 引擎、会话汇总和已发布路由',Peers:'配置 BGP 邻居并导出对端 FRR 配置',Routes:'管理由 BGP 发布的手动路由',Health:'查看服务组件的健康状态和最近探测',Events:'查看配置变更、服务状态及运行事件',Settings:'管理 DNS、Surge、路由及应用偏好'};
const navSections={Dashboard:'总览',DNS:'DNS 与决策',Routing:'路由与连接',Health:'管理'};
const groups={general:'通用',dns:'DNS',surge:'Surge',routing:'路由',cache:'缓存',health:'健康',logging:'日志与隐私',security:'安全',updates:'软件更新',notifications:'通知',statusbar:'状态栏',automation:'CLI 与 skill',about:'关于'};
const labels={automatic_check:'自动检查新版本',check_beta:'自动检查 Beta 版本',automatic_install:'自动安装新版本',check_interval_hours:'自动检查间隔（小时）',update_time:'自动更新时间（本地时间）',update_source:'更新源',aaaa_fallback:'Surge 关闭 IPv6 时的 AAAA 回落',expire_decisions:'按 TTL 定时过期稳定决策',invalidate_on_read_failure:'Surge 配置读取失败时失效旧决策',invalidate_direct_on_selection_change:'策略组选择变化时失效 DIRECT',invalidate_reject_on_selection_change:'策略组选择变化时失效 REJECT',conservative_direct:'DIRECT 保守判断',routing_mode:'路由模式',target:'Surge 分配的 Fake-IP',fake_ip_backend:'Fake-IP 路由方式',dns_backend:'DNS 拦截路由方式',static_fake_ip_configured:'我已在外部网关配置全部 Fake-IP 路由指向 Surge',static_dns_interception:'允许持久的静态 DNS 拦截路由',static_dns_configured:'我已在外部网关配置 DNS 拦截路由指向 Controller',addresses:'IP 地址（每行一项）',ttl:'TTL（秒）',source:'来源',order:'匹配顺序',name:'名称',web_listen:'管理监听地址',mode:'全局模式',theme:'主题',listeners:'DNS 监听地址（每行一项）',transparent_listeners:'透明 DNS 监听地址',interception_enabled:'启用透明拦截',ingress_interfaces:'接收接口（每行一项）',egress_interface:'上游出口接口',source4:'IPv4 出口源地址',source6:'IPv6 出口源地址',gateway4:'IPv4 网关',gateway6:'IPv6 网关',bypass_verified:'已验证网关源地址绕行策略',preserve_original:'保留客户端原始解析器',upstreams:'真实 DNS 上游',timeout_ms:'超时（毫秒）',source_port_first:'源端口范围起点',source_port_last:'源端口范围终点',user_name:'Surge macOS 用户（留空使用当前桌面用户）',cli_path:'Surge CLI 绝对路径',api_url:'Surge HTTP API 地址',api_token:'API 凭据（留空保留）',dns:'Surge DNS 地址',fake_prefixes:'Fake-IP 网段',next_hop4:'Surge IPv4 下一跳',next_hop6:'Surge IPv6 下一跳',enabled:'启用',local_asn:'本地 ASN',router_id:'Router ID',listen_port:'BGP 监听端口',route_limit:'路由数量硬上限',hold_seconds:'Hold time（秒）',keepalive_seconds:'Keepalive（秒）',capacity:'决策缓存容量',response_capacity:'响应缓存容量',workers:'规则求值工作线程',queue:'求值队列容量',direct_ttl_seconds:'DIRECT 缓存 TTL（秒）',surge_ttl_seconds:'SURGE 缓存 TTL（秒）',unknown_ttl_seconds:'UNKNOWN TTL（秒）',error_ttl_seconds:'错误 TTL（秒）',interval_ms:'探测间隔（毫秒）',failures:'连续失败阈值',successes:'连续成功阈值',grace_seconds:'Fake-IP 保留时间（秒）',stabilize_seconds:'路由稳定等待（秒）',probe_domain:'健康探测域名',level:'日志级别',query_history:'内存查询记录',history_capacity:'内存记录容量',log_domains:'保留域名',event_retention:'持久事件容量',authentication:'启用管理认证',username:'用户名',password:'新密码（至少 12 字节）',session_minutes:'会话有效时间（分钟）',allowed_networks:'允许的管理网段',auth_bypass_networks:'免认证 IP 白名单（每行 IP 或 CIDR）',trusted_proxies:'可信反向代理（每行 IP 或 CIDR）',proxy_origins:'反向代理访问地址（每行 HTTP(S) 源地址）',proxy_client_ip_header:'代理客户端 IP 请求头',tls_cert:'TLS 证书路径',tls_key:'TLS 私钥路径',address:'邻居地址',asn:'邻居 ASN',port:'端口',description:'说明',prefix:'网段',next_hop:'下一跳',med:'MED',local_preference:'本地优先级',communities:'Communities（每行 ASN:value）',health_binding:'健康绑定',domain:'域名',match:'匹配方式',decision:'决策',protocol:'协议',tls_name:'TLS 证书名称',url:'DoH HTTPS 地址'};
const detailLabels={persistent:'长期缓存',query_type:'查询类型',client_ip:'客户端 IP',client_port:'客户端端口',original_destination_ip:'原始目标 IP',original_destination_port:'原始目标端口',transport:'传输协议',question:'问题',intercepted:'透明拦截',received_at:'接收时间',confidence:'置信度',matched_rule:'匹配规则',policy:'策略',final_policy:'最终策略',reason:'原因',created_at:'创建时间',expires_at:'过期时间',classification_ms:'规则求值耗时（毫秒）',resolver_ms:'解析耗时（毫秒）',resolver:'解析器',rcode:'响应代码',latency_ms:'总耗时（毫秒）',last_success:'最近成功',last_failure:'最近失败',successes:'连续成功次数',failures:'连续失败次数',last_error:'最近错误',kind:'类型',backend:'路由方式',state:'服务状态',advertised:'已发布前缀'};
const securityHints={allowed_networks:'这些网段允许访问管理接口，但仍需登录；免认证白名单中的 IP 自动获得访问权限。',auth_bypass_networks:'匹配的客户端直接进入管理页面和 CLI，无需用户名密码。留空关闭；写操作仍校验 CSRF。',trusted_proxies:'只填写实际代理的来源地址。非可信连接的转发头会被忽略；缺少真实客户端 IP 时不免认证。',proxy_origins:'例如 https://routepilot.example.com；不包含路径。代理访问域名需要在此登记；HTTPS 终止后仍使用安全 Cookie。',proxy_client_ip_header:'默认 X-Forwarded-For。代理必须改写选中的 IP 头，以及转发的 Host 和 Proto；不从其他 IP 头回退。'};
const securityNetworkFields=['allowed_networks','auth_bypass_networks','trusted_proxies','proxy_origins','proxy_client_ip_header'];
const choices={proxy_client_ip_header:['X-Forwarded-For','X-Real-IP','Forwarded'],update_source:['github','gitea'],aaaa_fallback:['surge','block','upstream'],fake_ip_backend:['bgp','static'],dns_backend:['bgp','static'],mode:['auto','force-direct','force-surge'],theme:['system','light','dark'],health_binding:['none','controller','surge','dns'],match:['exact','suffix'],decision:['DIRECT','SURGE','REJECT'],protocol:['udp','tcp','dot','doh'],level:['debug','info','warn','error']};
export function Field({name,value,onChange,options,label:overrideLabel,autoComplete:autocompleteOverride,required=false,hint,tooltip,disabled=false,...limits}){
 const t=useT();
 if(name==='policy_upstreams')return <PolicyDNSEditor value={value} onChange={onChange}/>;
 if(name==='id'||(name.endsWith('_configured')&&!name.startsWith('static_'))||name==='password_hash')return null;
 const label=overrideLabel||t(labels[name]||detailLabels[name]||name);
 if(options||choices[name])return <FieldLabel data-field={name} label={label} required={required} hint={hint&&t(hint)}><select disabled={disabled} value={value??''} onChange={e=>onChange(e.target.value)}>{(options||choices[name]).map(v=><option key={v} value={v}>{name==='aaaa_fallback'?t(v==='surge'?'沿用原有分流规则':v==='block'?'关闭 IPv6 时拦截 AAAA（成功空响应）':'关闭 IPv6 时直接查询上游'):valueLabel(v)}</option>)}</select></FieldLabel>;
 if(typeof value==='boolean')return <label title={tooltip&&t(tooltip)} className={'check '+(required&&!value?'required-check':'')}><input aria-label={label} disabled={disabled} type="checkbox" required={required} checked={value} onChange={e=>onChange(e.target.checked)}/>{label}{required&&<span className="required-mark" aria-hidden="true">{t('必选')}</span>}</label>;
 if(Array.isArray(value)&&name==='upstreams')return <fieldset className="upstream-pool"><legend>{label}</legend>{value.map((u,i)=><section className="upstream-card" key={i}><header className="section-heading"><h3>{t('DNS 服务器 {number}',{number:i+1})}</h3></header><div className="form-grid">{['protocol','address','name',...(u.protocol==='dot'?['tls_name']:u.protocol==='doh'?['url']:[]),'enabled'].map(k=><Field key={k} name={k} required={['protocol','address','tls_name','url'].includes(k)} label={k==='address'?t('上游服务器地址'):undefined} value={u[k]??(k==='enabled'?true:'')} onChange={v=>onChange(value.map((x,j)=>j===i?{...x,[k]:v}:x))}/>)}</div><div className="toolbar"><button className="danger" type="button" disabled={value.length===1} onClick={()=>onChange(value.filter((_,j)=>j!==i))}>{t('删除上游')}</button></div></section>)}<button type="button" disabled={value.length>=16} onClick={()=>onChange([...value,{name:'',protocol:'udp',address:'',enabled:true}])}>{t('添加上游')}</button></fieldset>;
 if(Array.isArray(value))return <FieldLabel className="field-list" data-field={name} label={label} required={required} hint={hint&&t(hint)}><textarea disabled={disabled} rows={3} value={value.join('\n')} onChange={e=>onChange(e.target.value.split('\n').filter(Boolean))}/></FieldLabel>;
 const secret=name==='password'||name==='api_token'||name==='webhook_url',number=numericField(name)||typeof value==='number';
	return <FieldLabel data-field={name} label={label} required={required} hint={hint&&t(hint)}><input {...limits} disabled={disabled} type={secret?'password':number?'number':name==='url'?'url':name==='update_time'?'time':'text'} autoComplete={autocompleteOverride??(secret&&name!=='webhook_url'?'new-password':'off')} value={value??''} onChange={e=>onChange(number&&e.target.value!==''?Number(e.target.value):e.target.value)}/></FieldLabel>;
}
function Table({rows,columns,onRow,selectable=false,selectedIds=[],onToggle,onToggleAll,rowActionLabel='查看详情'}){
 const t=useT(),all=selectable&&rows.length>0&&rows.every(row=>selectedIds.includes(row.id));
 const label=c=>t(c.label||labels[c.key]||detailLabels[c.key]||c.key);
 const value=(row,column)=>{
  if(column.render)return column.render(row);
  const key=column.key,result=row[key];
  if(['enabled','state','health_binding','match','source','kind','backend','route_state','level'].includes(key))return pretty(valueLabel(result));
  if(key==='last_error')return message(result)||'—';
  if(['last_success','last_failure'].includes(key))return !result||String(result).startsWith('0001-01-01T')?t('从不'):pretty(result);
  if(key==='latency_ms'&&Number.isFinite(result))return Math.round(result);
  return pretty(result);
 };
 return <div className="table-wrap"><table className="responsive-table" role="table">
  <thead role="rowgroup"><tr role="row">
   {selectable&&<th className="selection-head" data-label={t('全选表项')} role="columnheader"><label className="table-selection"><input type="checkbox" aria-label={t('全选表项')} checked={all} onChange={e=>onToggleAll(e.target.checked)}/></label></th>}
   {columns.map(column=><th key={column.key} role="columnheader">{label(column)}</th>)}
  </tr></thead>
  <tbody role="rowgroup">{rows.map((row,index)=><tr role="row" key={row.id||index} onClick={()=>onRow?.(row)} className={onRow?'clickable':''}>
   {selectable&&<td className="selection-cell" data-label={t('选择表项')} role="cell"><label className="table-selection" onClick={e=>e.stopPropagation()}><input type="checkbox" aria-label={t('选择表项 {item}',{item:row.prefix||row.domain||row.address||row.name||row.id})} checked={selectedIds.includes(row.id)} onChange={()=>onToggle(row.id)}/></label></td>}
   {columns.map(column=><td key={column.key} role="cell" data-label={label(column)}><span className="cell-value">{value(row,column)}</span></td>)}
   {onRow&&<td className="mobile-row-action" role="cell"><button type="button" onClick={e=>{e.stopPropagation();onRow(row)}}>{t(rowActionLabel)}</button></td>}
  </tr>)}</tbody>
 </table>{!rows.length&&<div className="empty">{t('暂无记录')}</div>}</div>;
}
function Badge({value}){const t=useT();return <span className={'badge '+(['HEALTHY','UP','ESTABLISHED','DIRECT','Announced',true].includes(value)?'good':['DOWN','ERROR','STOPPING','DEGRADED','DISCONNECTED','REJECT'].includes(value)?'bad':'neutral')}>{valueLabel(value)??'—'}</span>}
const metric=(label,value)=><div className="metric" key={label}><span>{translate(valueLabel(label))}</span><strong>{pretty(value)}</strong></div>;
const templates={Hosts:{domain:'',match:'exact',addresses:[],ttl:60,enabled:true,source:'manual'},Interception:{name:'',prefix:'',enabled:true},Peers:{address:'',asn:64512,hold_seconds:30,keepalive_seconds:10,port:179,enabled:true,description:''},Routes:{prefix:'',next_hop:'',med:0,local_preference:100,communities:[],health_binding:'none',enabled:true,description:''},Overrides:{domain:'',match:'exact',decision:'SURGE',enabled:true,description:''}};
const paths={Hosts:'dns/hosts',Interception:'dns/interception',Peers:'bgp/peers',Routes:'bgp/routes',Overrides:'overrides'};
function useModal(open,close){useEffect(()=>{if(!open)return;const overflow=document.body.style.overflow;document.body.style.overflow='hidden';const escape=e=>{if(e.key==='Escape')close()};document.addEventListener('keydown',escape);return()=>{document.body.style.overflow=overflow;document.removeEventListener('keydown',escape)}},[open])}
export function Resource({page,rows,refresh,notify,bgpRoutes=[],peerStates=[],realIPSnapshot={},settings={}}){const t=useT();const [routeImport,setRouteImport]=useState(false);const [edit,setEdit]=useState(null),[importResult,setImportResult]=useState(null),[selectedIds,setSelectedIds]=useState([]);const [peerDetailsID,setPeerDetailsID]=useState(null);const detailPeer=page==='Peers'?rows.find(row=>row.id===peerDetailsID):null;useModal(Boolean(edit||peerDetailsID),()=>{setEdit(null);setPeerDetailsID(null)});useEffect(()=>{if(peerDetailsID&&!detailPeer)setPeerDetailsID(null)},[peerDetailsID,detailPeer]);useEffect(()=>{setEdit(null);setPeerDetailsID(null);setImportResult(null);setRouteImport(false);setSelectedIds([])},[page]);const path=paths[page];
 const importHosts=async()=>{try{const result=await request("dns/hosts/import-surge","POST");setImportResult(result);refresh();notify(t('已导入 {imported} 项，跳过 {skipped} 项',{imported:result.imported,skipped:result.skipped.length}))}catch(e){notify(e.message,true)}};const save=async e=>{e.preventDefault();try{await request(path+(edit.id?'/'+edit.id:''),edit.id?'PUT':'POST',edit);setEdit(null);refresh();notify(t('已保存'))}catch(e){notify(e.message,true)}};const remove=async row=>{try{await request(path+'/'+row.id,'DELETE');setEdit(null);refresh();notify(t('已删除'))}catch(e){notify(e.message,true)}};const bulkDelete=async()=>{if(!selectedIds.length||!window.confirm(t('确认删除选中的 {count} 项？',{count:selectedIds.length})))return;try{const result=await request('resources/bulk-delete','POST',{resource:path,ids:selectedIds});setSelectedIds([]);refresh();notify(t('已批量删除 {count} 项',{count:result.deleted}))}catch(e){notify(e.message,true)}};
 return <>{page==='Hosts'&&<div className="notice">{t("手动条目优先；Surge 导入为静态快照，按原配置顺序匹配。支持 IPv4/IPv6、* 和 ? 通配符；服务器、别名、脚本和远程规则需在此手动配置。")}{importResult&&<details><summary>{t('导入 {imported} 项 / 跳过 {skipped} 项',{imported:importResult.imported,skipped:importResult.skipped.length})}</summary>{importResult.skipped.map((s,i)=><p key={i}>{t('第 {line} 行：{reason}',{line:s.line,reason:message(s.reason)})}</p>)}</details>}</div>}<div className="toolbar"><span>{t('{count} 项',{count:rows.length+(page==='Overrides'&&Array.isArray(realIPSnapshot.always_real_ip)?realIPSnapshot.always_real_ip.filter(entry=>typeof entry==='string').length:0)})}</span>{selectedIds.length>0&&<button className="danger" onClick={bulkDelete}>{t('删除所选 {count} 项',{count:selectedIds.length})}</button>}{page==='Routes'&&<button onClick={()=>setRouteImport(true)}>{t('从 URL 导入')}</button>}{page==='Hosts'&&<button onClick={importHosts}>{t("从 Surge 导入")}</button>}<button className="primary" onClick={()=>setEdit(structuredClone(templates[page]))}>{t("＋ 添加")}</button></div>{routeImport&&<RouteImport settings={settings} onClose={()=>setRouteImport(false)} refresh={refresh} notify={notify}/>} {page==='Overrides'?<OverridesTable rows={rows} snapshot={realIPSnapshot} onEdit={row=>setEdit(structuredClone(row))} selectedIds={selectedIds} onToggle={id=>setSelectedIds(old=>old.includes(id)?old.filter(x=>x!==id):[...old,id])} onToggleAll={checked=>setSelectedIds(checked?rows.map(row=>row.id):[])}/>:<Table selectable rowActionLabel="编辑" rows={rows} columns={Object.keys(templates[page]).filter(k=>!['description','router_id','keepalive_seconds','hold_seconds','local_asn','communities','med','local_preference','order'].includes(k)).map(key=>({key})).concat(page==='Peers'?[{key:'state',label:t('会话状态'),render:r=><Badge value={r.enabled?(peerStates.find(p=>p.address===r.address)?.state||t('未运行')):t('已禁用')}/>},{key:'uptime_seconds',label:t('在线秒数'),render:r=>peerStates.find(p=>p.address===r.address)?.uptime_seconds??'—'},{key:'prefixes',label:t('已发布前缀'),render:r=>peerStates.find(p=>p.address===r.address)?.prefixes??'—'},{key:'last_error',label:t('最近错误'),render:r=>peerStates.find(p=>p.address===r.address)?.last_error?message(peerStates.find(p=>p.address===r.address).last_error):'—'},{key:'details',label:t('操作'),render:r=><button type="button" onClick={event=>{event.stopPropagation();setPeerDetailsID(r.id)}}>{t('详情')}</button>}]:[]).concat(page==='Interception'?[{key:'bgp_state',label:t('BGP 状态'),render:r=><Badge value={bgpRoutes.find(x=>x.prefix===r.prefix)?.announced?'Announced':'Inactive'}/>}]:[])} onRow={row=>setEdit(structuredClone(row))} selectedIds={selectedIds} onToggle={id=>setSelectedIds(old=>old.includes(id)?old.filter(x=>x!==id):[...old,id])} onToggleAll={checked=>setSelectedIds(checked?rows.map(row=>row.id):[])}/>}{edit&&<div className="modal" role="dialog" aria-label={t("编辑")}><form onSubmit={save}><header><h2>{edit.id?t('编辑'):t('添加')} {t(pages.find(p=>p[0]===page)[1])}</h2><button type="button" onClick={()=>setEdit(null)}>{t("关闭")}</button></header><FormHint/><div className="form-grid">{Object.entries(edit).filter(([k])=>page!=='Hosts'||!['source','order'].includes(k)).map(([k,v])=><Field key={k} name={k} required={resourceRequired(page,k)} value={v} options={page==='Hosts'&&k==='match'?['exact','wildcard']:undefined} onChange={value=>setEdit({...edit,[k]:value})}/>)}</div><footer>{edit.id&&<button className="danger" type="button" onClick={()=>remove(edit)}>{t("删除")}</button>}<button className="primary">{t("保存")}</button></footer></form></div>}{detailPeer&&<BGPPeerDetails peer={detailPeer} live={peerStates.find(peer=>peer.address===detailPeer.address)} settings={settings} onClose={()=>setPeerDetailsID(null)}/>}</>}
const backupOptions=[['general','通用'],['dns','DNS 设置'],['surge','Surge 设置'],['routing','路由设置'],['bgp','BGP 设置'],['cache','缓存'],['health','健康检查'],['logging','日志与隐私'],['notifications','通知'],['security','安全'],['updates','软件更新'],['peers','BGP 邻居配置'],['routes','路由与定时同步'],['route_feeds','路由订阅'],['interception','DNS 拦截目标'],['overrides','域名覆盖'],['hosts','本地 DNS']];
export function Settings({settings,refresh,notify,status={},backend=null,interception=[],onUpdateTarget,onUpdateActionsTarget}){const t=useT();const [locale]=useLocale(),ct=cacheText(locale);const [localPreferences,setLocalPreferences,preferencesSaved]=usePreferenceDraft(['updatePrompts']);const localApp=nativeApp()&&!backend;const [updateActionsTarget,setUpdateActionsTarget]=useState(null);const [category,setCategory]=useState('general'),[draft,setDraft]=useState(null),[saving,setSaving]=useState(false),[saveStage,setSaveStage]=useState(''),[backupSelection,setBackupSelection]=useState(backupOptions.map(([key])=>key)),[restoreBackup,setRestoreBackup]=useState(null),[restoreSelection,setRestoreSelection]=useState([]);useEffect(()=>{if(settings)setDraft(previous=>previous??structuredClone(settings))},[settings]);if(!draft)return null;const save=async e=>{e.preventDefault();if(saving)return;setSaving(true);try{const routing=draft.routing;const submitted=routing&&routing.mode!=='hybrid'?{...draft,routing:{...routing,fake_ip_backend:routing.mode,dns_backend:routing.mode}}:draft;const restart=submitted.surge?.restart;const normalized=restart?{...submitted,surge:{...submitted.surge,restart:{...restart,groups:(restart.groups||[]).map(name=>name.trim()).filter(Boolean),nodes:(restart.nodes||[]).map(name=>name.trim()).filter(Boolean)}}}:submitted;const saved=await saveSettings(normalized,setSaveStage);setDraft(saved.settings);if(category==='updates'&&localApp){if(localPreferences.updatePrompts!==appPreferences().updatePrompts){try{await savePreferences({...appPreferences(),updatePrompts:localPreferences.updatePrompts})}catch(e){notify(t('后端设置已保存，但本机更新提示偏好保存失败：{error}',{error:e.message}),true);refresh();return}}preferencesSaved()}notify(category==='cache'?ct('saved'):t('设置已保存'));if(draft.general.web_listen!==settings.general.web_listen||draft.security.tls_cert!==settings.security.tls_cert){if(isRemote()){notify(t('远程监听地址已变更，请在管理后端中编辑连接地址并重新连接'));return}reconnectAfterApplication((draft.security.tls_cert?'https':'http')+'://'+draft.general.web_listen);refresh();return}refresh()}catch(e){if(e.name!=='AbortError')notify(e.message,true)}finally{setSaving(false);setSaveStage('')}};
 const exportConfig=async()=>{try{if(!backupSelection.length)throw new Error(t('至少选择一个配置分区'));const query=backupSelection.map(section=>'sections='+encodeURIComponent(section)).join('&');const doc=await request('configuration/export?'+query);const blob=new Blob([JSON.stringify(doc,null,2)],{type:'application/json'});const url=URL.createObjectURL(blob);const link=document.createElement('a');link.href=url;link.download='routepilot-backup.json';link.click();URL.revokeObjectURL(url)}catch(e){notify(e.message,true)}};
 const chooseRestoreFile=async e=>{const file=e.target.files?.[0];if(!file)return;try{if(file.size>512*1024)throw new Error(t('配置文件超过 512 KiB'));let backup=JSON.parse(await file.text());if(backup.format_version===undefined&&backup.version===1){backup={format_version:1,config_version:1,sections:backupOptions.map(([key])=>key),configuration:backup}}if(backup.format_version!==1||backup.config_version!==1||!Array.isArray(backup.sections)||!backup.configuration)throw new Error(t('备份格式或配置版本不兼容'));const available=backup.sections.filter(section=>backupOptions.some(([key])=>key===section));if(!available.length)throw new Error(t('备份中没有可恢复的配置分区'));setRestoreBackup(backup);setRestoreSelection(available)}catch(error){setRestoreBackup(null);setRestoreSelection([]);notify(error.message,true)}finally{e.target.value=''}};
 const importConfig=async()=>{if(!restoreBackup||!restoreSelection.length){notify(t('请选择要恢复的配置分区'),true);return}try{const saved=await request('configuration/import','POST',{backup:restoreBackup,sections:restoreSelection});setDraft(saved.settings);setRestoreBackup(null);notify(t('配置已恢复'));refresh();if(saved.settings.general.web_listen!==settings.general.web_listen||saved.settings.security.tls_cert!==settings.security.tls_cert){if(isRemote())notify(t('远程监听地址已变更，请在管理后端中编辑连接地址并重新连接'));else reconnectAfterApplication((saved.settings.security.tls_cert?'https':'http')+'://'+saved.settings.general.web_listen)}}catch(e){notify(e.message,true)}};
 const entries=Object.entries({...draft[category],...(category==='dns'?{policy_upstreams:draft.dns.policy_upstreams??[]}:category==='security'?{password:draft.security.password??''}:category==='surge'?{api_token:draft.surge.api_token??''}:category==='notifications'?{webhook_url:draft.notifications?.webhook_url??'',clear_webhook:draft.notifications?.clear_webhook??false}:{})}).filter(([k])=>(category!=='dns'||k!=='clients')&&(category!=='general'||!['mode','name'].includes(k))&&(category!=='surge'||!['trust_direct','restart'].includes(k))&&(category!=='notifications'||!['errors','surge','bgp','dns','controller'].includes(k)));
 const renderField=(entry,section=category)=>{
  section=typeof section==='string'?section:category;
  const [k,v]=entry,requirements=settingsRequirement(section,k,draft,interception);
  return <Field key={section+'-'+k} name={k} {...requirements} value={k==='update_source'?(v||'github'):k==='aaaa_fallback'?(v||'surge'):k==='proxy_client_ip_header'?(v||'X-Forwarded-For'):['auth_bypass_networks','trusted_proxies','proxy_origins'].includes(k)?(v??[]):v} hint={section==='security'&&securityHints[k]?securityHints[k]:requirements.hint} label={section==='cache'?ct(k):section==='notifications'?t({enabled:'启用服务异常通知',surge:'Surge 异常',bgp:'BGP 与路由异常',dns:'DNS 异常',controller:'控制器与数据库异常',recovery:'服务恢复时通知',cooldown_seconds:'持续异常重复提醒间隔（秒，0 表示不重复）',webhook_url:'Webhook 地址（留空保留已保存地址）',clear_webhook:'删除已保存的 Webhook 地址'}[k]||k):undefined} tooltip={section==='cache'&&typeof v==='boolean'?ct(k+'_tooltip'):undefined} onChange={value=>setDraft({...draft,[section]:{...draft[section],[k]:value,...(section==='updates'&&k==='automatic_check'&&!value?{automatic_install:false}:{})}})}/>;
 };
 return <>
  <div className="tabs">{Object.entries(groups).map(([k,v])=><button key={k} className={k===category?'selected':''} onClick={()=>setCategory(k)}>{t(v)}</button>)}</div>
  {category==='about'?<About status={status} backend={backend}/>:category==='statusbar'?<StatusBarSettings notify={notify}/>:category==='automation'?<AutomationSettings notify={notify} backend={backend}/>:<>
   {category==='notifications'&&<AppPreferencesPanel notify={notify}/>}
   {saving&&<p className="notice" role="status">{t('正在提交配置…')}</p>}
   <form className="settings-form" onSubmit={save}>
    <header className="routing-heading"><h2>{category==='cache'?ct('settingsTitle'):category==='updates'?t('软件更新'):t('{section}设置',{section:t(groups[category])})}</h2>{category==='general'&&<LanguageSelector/>}</header>
    {category==='updates'?<>
     {localApp?<LocalAppUpdateControls preferences={localPreferences} actionsTarget={updateActionsTarget}/>:onUpdateTarget?<div ref={onUpdateTarget}/>:<BackendUpdate backend={backend} capable={status.online_update} software={status.software} authentication={settings?.security?.authentication!==false} onReady={refresh} actionsTarget={updateActionsTarget} showCurrent embedded/>}
    </>:<FormHint/>}
    <fieldset disabled={saving} style={{border:0,padding:0,margin:0,minWidth:0}}>
     {category==='routing'?<>
      <div className="form-grid"><Field name="mode" label={t('分流模式')} required value={draft.general?.mode||'auto'} hint="选择当前后端的全局 DNS 分流策略，保存设置后生效。" onChange={mode=>setDraft({...draft,general:{...draft.general,mode}})}/></div>
      <RoutingSettingsFields draft={draft.routing} onChange={routing=>setDraft({...draft,routing})}/>
      {['bgp','hybrid'].includes(draft.routing?.mode)&&<section className="panel"><h3>BGP</h3><div className="form-grid">{Object.entries(draft.bgp||{}).filter(([,value])=>typeof value!=='boolean').map(entry=>renderField(entry,'bgp'))}</div><div className="option-grid">{Object.entries(draft.bgp||{}).filter(([,value])=>typeof value==='boolean').map(entry=>renderField(entry,'bgp'))}</div></section>}
     </>:category==='dns'?<DNSSettingsFields entries={entries} renderField={renderField}/>:category==='security'?<>
      <section className="panel"><h3>{t('管理认证与 TLS')}</h3><div className="option-grid">{entries.filter(([k,v])=>typeof v==='boolean').map(renderField)}</div><div className="form-grid">{entries.filter(([k,v])=>typeof v!=='boolean'&&!securityNetworkFields.includes(k)).map(renderField)}</div></section>
      <section className="panel"><h3>{t('管理 IP 与反向代理')}</h3><div className="form-grid">{securityNetworkFields.map(key=>entries.find(([k])=>k===key)).filter(Boolean).map(renderField)}</div><p>{t('客户端 IP 按代理链从右向左确认。只将真实客户端加入免认证白名单，不要将代理地址用于免认证。通过本机中继连接远端时，远端看到的是本机 Controller 的出口 IP。')}</p></section>
     </>:<>
      <div className="form-grid">{entries.filter(([,v])=>typeof v!=='boolean').map(entry=><React.Fragment key={entry[0]}>{renderField(entry)}{category==='updates'&&entry[0]==='update_source'&&nativeApp()&&(draft.updates.update_source||'github')==='github'&&<GitHubAccessTokenControls preferences={localPreferences} notify={notify}/>}</React.Fragment>)}</div>
      <div className="option-grid">{entries.filter(([,v])=>typeof v==='boolean').map(renderField)}{category==='updates'&&localApp&&<Field name="updatePrompts" label={t('发现新版本时提示')} value={localPreferences.updatePrompts} onChange={updatePrompts=>setLocalPreferences({...localPreferences,updatePrompts})}/>} {category==='surge'&&<Field name="conservative_direct" value={!draft.surge.trust_direct} onChange={value=>setDraft({...draft,surge:{...draft.surge,trust_direct:!value}})}/>}</div>
     </>}
     {category==='notifications'&&<><NotificationErrors settings={draft.notifications} onChange={errors=>setDraft({...draft,notifications:{...draft.notifications,errors}})}/><p>{t('通知依据健康探测的连续失败阈值触发，未启用的组件不会告警。设置作用于当前后端。')}</p><p>{t('Webhook 由后台独立发送 JSON POST，包含 title、body、state、components 和 at；页面关闭后仍可推送。地址加密保存，不包含在导出备份中。')}</p><p>{draft.notifications?.webhook_configured?t('已保存 Webhook 地址'):t('尚未配置 Webhook 地址；仍可使用本机系统通知。')}</p></>}
     {category==='updates'&&<><p>{t('自动更新设置保存在当前连接的后端，控制其所在 Mac 的完整应用（App、管理页面与网络服务）的自动检查与自动安装。')}</p><p>{t('自动安装每天最多尝试一次；未在计划时间运行的服务会在启动后补装当天的更新。')}</p><p>{t('启用“自动检查 Beta 版本”后，检查会同时比较最新正式版与编号 Beta，版本号相同时优先正式版；测试版可能包含未完成的功能。')}</p>{draft.updates===undefined&&<p role="alert">{t('当前后端版本较旧，不支持自动更新设置；请先升级该后端。')}</p>}</>}
     {category==='cache'&&<p>{ct('help')}</p>}
     {category==='dns'&&<p>{t('仅在确认 Surge 的有效配置关闭 IPv6 时应用；拦截会返回成功空响应（NODATA），上游查询会跳过本地 Hosts、规则判断与 Fake-IP，使用配置的真实 DNS 上游。Surge 开启 IPv6、不可用或配置读取失败时沿用原有规则。')}</p>}
     {category==='surge'&&<><SurgeRestartSettings value={draft.surge.restart} onChange={restart=>setDraft({...draft,surge:{...draft.surge,restart}})} apiURL={draft.surge.api_url} status={status.surge_restart}/><p>{t("开启时，Surge 返回 DIRECT 但连接上下文不充分会记为 UNKNOWN，并交给 Surge。关闭时，采用 Surge 返回的内置 DIRECT 并查询真实 DNS 上游；未来连接仍可能命中不同策略。求值失败或超时仍保持 UNKNOWN。")}</p></>}
    </fieldset>
    <div className={category==='updates'?'toolbar settings-update-actions':'toolbar'} ref={category==='updates'?(localApp?setUpdateActionsTarget:onUpdateActionsTarget||setUpdateActionsTarget):null}><button className="primary" disabled={saving}>{saving?t('正在保存…'):category==='cache'?ct('save'):t('保存设置')}</button></div>
   </form>
   {category==='general'&&<section className="panel"><h2>{t('配置备份与恢复')}</h2><p>{t('选择需要导出或恢复的配置分区。恢复会保留未勾选的现有配置；格式或配置版本不兼容时将拒绝恢复。')}</p><h3>{t('备份范围')}</h3><div className="option-grid">{backupOptions.map(([key,label])=><label key={key}><input type="checkbox" checked={backupSelection.includes(key)} onChange={event=>setBackupSelection(old=>event.target.checked?[...old,key]:old.filter(item=>item!==key))}/>{t(label)}</label>)}</div><div className="toolbar"><button type="button" onClick={exportConfig}>{t('导出所选配置')}</button><label className="file-button">{t('选择备份文件')}<input type="file" accept="application/json" onChange={chooseRestoreFile}/></label></div>{restoreBackup&&<><h3>{t('恢复范围')}</h3><p>{t('备份配置版本：{version}',{version:restoreBackup.config_version})}</p><div className="option-grid">{restoreBackup.sections.map(key=>{const label=backupOptions.find(([option])=>option===key)?.[1]||key;return <label key={key}><input type="checkbox" checked={restoreSelection.includes(key)} onChange={event=>setRestoreSelection(old=>event.target.checked?[...old,key]:old.filter(item=>item!==key))}/>{t(label)}</label>})}</div><div className="toolbar"><button type="button" className="primary" onClick={importConfig}>{t('恢复所选配置')}</button><button type="button" onClick={()=>{setRestoreBackup(null);setRestoreSelection([])}}>{t('取消')}</button></div></>}</section>}
  </>}
 </>;
}
function queryDecisionLabel(result){const t=translate;if(result.source==='state-fallback')return t('回退至真实 DNS');if(result.decision==='UNKNOWN'&&result.final_policy==='DIRECT')return t('UNKNOWN（DIRECT 尚未确认）');if(result.decision==='UNKNOWN'&&/timed out|deadline/.test(result.reason||''))return t('UNKNOWN（求值超时）');return result.decision}
function queryReasonLabel(result){const t=translate;if(result.source==='surge-always-real-ip')return t('命中 Surge always-real-ip，直接查询配置的真实 DNS 上游，跳过 Surge 规则求值与 Fake-IP 查询。');if(result.decision==='UNKNOWN'&&result.final_policy==='DIRECT')return t('Surge 返回 DIRECT，但仍缺少确认未来连接策略所需的上下文；保守交给 Surge 处理。原始原因：')+message(result.reason||'');if(/timed out|deadline/.test(result.reason||''))return t('Surge 规则求值超过配置时限，未取得确定结果；保守交给 Surge 处理。');return message(result.reason)}
export function Queries({rows}){const t=useT();const [filter,setFilter]=useState({domain:'',client:'',destination:'',type:'',decision:'',source:'',ingress:''}),[detail,setDetail]=useState(null);useModal(Boolean(detail),()=>setDetail(null));const shown=rows.filter(r=>{const c=r.context||{},q=c.question||{},d=r.result||{};return (!filter.domain||(q.Name||'').includes(filter.domain))&&(!filter.client||(c.client_ip||'').includes(filter.client))&&(!filter.destination||(c.original_destination_ip||'').includes(filter.destination))&&(!filter.type||String(q.Qtype)===filter.type)&&(!filter.decision||d.decision===filter.decision)&&(!filter.source||(d.source==='cache'?'hit':'miss')===filter.source)&&(!filter.ingress||(c.intercepted?'intercepted':'direct')===filter.ingress)});
 return <><div className="filters">{['domain','client','destination'].map(k=><input key={k} placeholder={k==='domain'?t('域名'):k==='client'?t('客户端'):t('原始解析器')} value={filter[k]} onChange={e=>setFilter({...filter,[k]:e.target.value})}/>)}{Object.entries({type:[['1','A'],['28','AAAA'],['65','HTTPS'],['64','SVCB']],decision:['DIRECT','SURGE','REJECT','UNKNOWN'],source:['hit','miss'],ingress:['intercepted','direct']}).map(([k,opts])=><select aria-label={k} key={k} value={filter[k]} onChange={e=>setFilter({...filter,[k]:e.target.value})}><option value="">{t('全部 {field}',{field:t({type:t('类型'),decision:t('决策'),source:t('缓存'),ingress:t('接入方式')}[k])})}</option>{opts.map(v=><option key={Array.isArray(v)?v[0]:v} value={Array.isArray(v)?v[0]:v}>{Array.isArray(v)?v[1]:valueLabel(v)}</option>)}</select>)}</div><Table rows={shown} columns={[{key:'time',label:t('时间'),render:r=>new Date(r.context.received_at).toLocaleTimeString(currentLocale())},{key:'client',label:t('客户端'),render:r=>r.context.client_ip},{key:'dest',label:t('原始解析器'),render:r=>r.context.original_destination_ip},{key:'domain',label:t('域名'),render:r=>r.context.question.Name},{key:'type',label:t('类型'),render:r=>({1:'A',28:'AAAA',65:'HTTPS',64:'SVCB'}[r.context.question.Qtype]||r.context.question.Qtype)},{key:'decision',label:t('决策'),render:r=><Badge value={queryDecisionLabel(r.result)}/>}]} onRow={setDetail}/>{detail&&<div className="modal" role="dialog" aria-label={t("查询详情")}><section><header><h2>{t("查询详情")}</h2><button onClick={()=>setDetail(null)}>{t("关闭")}</button></header><DNSResult record={detail}/><RuleDNS result={detail.result}/><PolicyDNSResult result={detail.result}/><dl>{Object.entries({...detail.context,...detail.result,classification_ms:detail.classification_ms,resolver_ms:detail.resolver_ms,resolver:detail.resolver,rcode:detail.rcode,latency_ms:detail.latency/1e6}).filter(([k])=>!['rule_dns','rule_notes','policy_dns'].includes(k)).map(([k,v])=><React.Fragment key={k}><dt>{t(labels[k]||detailLabels[k]||k)}</dt><dd>{k==='decision'?(detail.result.source==='state-fallback'?t('回退至真实 DNS（未执行规则判断）'):queryDecisionLabel(detail.result)):k==='reason'?queryReasonLabel(detail.result):k==='confidence'&&v==='configured'?t('按配置采用（上下文未确认）'):k==='resolver'?message(valueLabel(v)):['confidence','source','intercepted'].includes(k)?pretty(valueLabel(v)):pretty(v)}</dd></React.Fragment>)}</dl></section></div>}</>}
function Rule({notify}){const t=useT();const [form,setForm]=useState({domain:'example.com',client_ip:'192.168.20.3',protocol:'tcp',port:443}),[result,setResult]=useState(null);const test=async e=>{e.preventDefault();try{setResult(await request('test/domain','POST',form))}catch(e){notify(e.message,true)}};return <><form onSubmit={test} className="form-grid rule-form">{Object.entries(form).map(([k,v])=><Field name={k} key={k} required value={v} options={k==='protocol'?['tcp','udp']:undefined} onChange={value=>setForm({...form,[k]:value})}/>)}<button className="primary">{t("测试规则")}</button></form>{result&&<section className="panel"><h2>{t("Surge 求值结果")}</h2><div className="policy-chain">{[result.match?.rule,result.match?.['rule-policy'],...(result.match?.steps||[]).map(s=>s.selected),result.match?.final,result.decision?.decision].filter(Boolean).map((v,i)=><span key={i}>{v}<b> → </b></span>)}</div><dl>{Object.entries(result.decision||{}).filter(([key])=>!['rule_dns','rule_notes','policy_dns'].includes(key)).map(([key,value])=><React.Fragment key={key}><dt>{t(labels[key]||detailLabels[key]||key)}</dt><dd>{key==='reason'?queryReasonLabel(result.decision):['confidence','source'].includes(key)?pretty(valueLabel(value)):pretty(value)}</dd></React.Fragment>)}</dl><RuleDNS result={result.decision||{}}/><details><summary>{t('原始求值结果')}</summary><pre>{JSON.stringify(result,null,2)}</pre></details></section>}</>}
function Workspace({backend,onManage,onLocalSession}){const t=useT();const [session,setSession]=useState(null),[page,setPage]=useState('Dashboard'),[data,setData]=useState({}),[toast,setToast]=useState(null),[error,setError]=useState(''),[login,setLogin]=useState({username:'admin',password:''});const notify=(message,bad=false)=>{setToast({message,bad});setTimeout(()=>setToast(null),6000)};
 const [updateTarget,setUpdateTarget]=useState(null),[updateActionsTarget,setUpdateActionsTarget]=useState(null),[monitorFocus,setMonitorFocus]=useState(false);
 useEffect(()=>{if(!monitorFocus)return;const exit=e=>{if(e.key==='Escape')setMonitorFocus(false)};window.addEventListener('keydown',exit);return()=>window.removeEventListener('keydown',exit)},[monitorFocus]);
 useEffect(()=>{if(page!=='Monitor')setMonitorFocus(false)},[page]);
 const alive=useRef(false),initializing=useRef(false),autoLoginAttempted=useRef(false),[signingIn,setSigningIn]=useState(false);
 const init=async()=>{
  if(initializing.current)return;initializing.current=true;
  try{
   let s=await request('session');if(!alive.current)return;setCSRF(s.csrf);
   if(backend&&!s.authenticated&&!autoLoginAttempted.current&&remoteCredentialsSupported()){
    autoLoginAttempted.current=true;setSigningIn(true);setError('');
    try{const restored=await restoreRemoteLogin(backend);if(!alive.current)return;if(restored.session){s={...s,...restored.session};autoLoginAttempted.current=false}}
    catch(e){if(alive.current)setError(e.message)}
    finally{if(alive.current)setSigningIn(false)}
   }else setError('');
   if(!alive.current)return;
   setSession(s);if(!backend)onLocalSession?.(s);
   if(!s.authenticated)window.webkit?.messageHandlers?.routepilotServiceHealth?.postMessage({clear:true});
  }catch(e){if(alive.current&&e.name!=='AbortError')setError(e.message)}finally{initializing.current=false}
 };
 useEffect(()=>{
  alive.current=true;window.routepilotActiveBackend=backend?{id:backend.id,url:backend.url}:null;init();
  return()=>{alive.current=false;if(window.routepilotActiveBackend?.id===backend?.id)window.routepilotActiveBackend=null};
 },[]);

 const refreshing=useRef(false),refreshPending=useRef(false),refreshRef=useRef(null),notificationState=useRef({settings:null,health:[],failures:0}),visible=useWindowVisible();
 const reportHealth=health=>observeServices({scope:backend?.url||location.origin,health,settings:notificationState.current.settings||{},remote:Boolean(backend),backendID:backend?.id});
 const refresh=async()=>{
  if(!session?.authenticated||!visible)return;
  if(refreshing.current){refreshPending.current=true;return}
  refreshing.current=true;
  try{
   const keys=['status','settings',...(pageRequests[page]||[])];
   const optional={'health':[],'bgp/status':{},'routing/status':{},'bgp/peers':[],'bgp/routes':[],'dns/interception':[],'dns/hosts':[],'overrides':[],'events':[],'dns/queries':[],'dns/clients':null,'dns/client-settings':null,'surge/status':{}};
   const values=await Promise.all(keys.map(k=>Object.hasOwn(optional,k)?requestOptional(k,optional[k]):request(k)));
   const next=Object.fromEntries(keys.map((k,i)=>[k.split('?')[0],values[i]]));
   setData(previous=>({...previous,...next}));setError('');
   notificationState.current={...notificationState.current,settings:next.settings?.notifications,failures:0};
   if(next.health){
    const peers=new Set((next['bgp/peers']||[]).filter(p=>p.enabled&&next['bgp/status']?.enabled).map(p=>'BGP peer '+p.address));
    const health=next.health.filter(c=>!c.name.startsWith('BGP peer ')||peers.has(c.name));
    notificationState.current.health=health;
    reportHealth([...health,{name:'Management connection',state:'UP'}]);
   }
  }catch(e){
   if(e.status===401){init()}
   else if(e.name!=='AbortError'){
    setData({});setError(e.message);
    if(!e.status||e.status>=500){
     notificationState.current.failures++;
     if(notificationState.current.failures>=3)reportHealth([...notificationState.current.health,{name:'Management connection',state:'DOWN'}]);
    }
   }
  }finally{
   refreshing.current=false;
   if(refreshPending.current){refreshPending.current=false;queueMicrotask(()=>refreshRef.current?.())}
  }
 };
 refreshRef.current=refresh;
 useEffect(()=>{if(!session?.authenticated||!visible)return;refresh();const timer=setInterval(refresh,3000);const stream=isRemote()?null:new EventSource('/api/v1/events/stream');stream?.addEventListener('update',refresh);return()=>{clearInterval(timer);stream?.close()}},[session?.authenticated,page,visible]);
 useEffect(()=>{document.documentElement.dataset.theme=data.settings?.general.theme||'system'},[data.settings?.general.theme]);
 const signIn=async e=>{
  e.preventDefault();if(signingIn||initializing.current)return;setSigningIn(true);autoLoginAttempted.current=true;
  const credential={...login};
  try{
   const s=await request('login','POST',credential,backend?{'X-RoutePilot-Expected-Backend':backend.url}:undefined);if(!alive.current)return;
   autoLoginAttempted.current=false;setCSRF(s.csrf);setSession({...session,...s});setLogin(previous=>({...previous,password:''}));setError('');
   if(backend&&remoteCredentialsSupported()){
    try{await saveRemoteCredential(backend,credential)}catch(e){if(alive.current)notify(t('登录成功，但凭证未保存')+'：'+message(e.message),true)}
   }else if(!backend){onLocalSession?.({...session,...s});window.webkit?.messageHandlers?.routepilotCredentials?.postMessage(credential)}
   else if(window.webkit?.messageHandlers?.routepilotCredentials)notify(t('更新 App 后可保存远程登录凭证。'),true);
  }catch(e){if(alive.current&&e.name!=='AbortError')setError(e.message)}finally{if(alive.current)setSigningIn(false)}
 };
 const signOut=async()=>{autoLoginAttempted.current=true;try{await request('logout','POST');if(alive.current)await init()}catch(e){if(alive.current)setError(e.message)}};
 if(!session||!session.authenticated)return <div className="login"><AppIcon/><h1>RoutePilot</h1><p>{backend?t('远程后端 · {name}',{name:backend.name}):t('本地服务')}<br/>{backend?.url||location.origin}</p>{session?<form onSubmit={signIn} autoComplete="on"><Field name="username" required autoComplete="username" value={login.username} onChange={v=>setLogin({...login,username:v})}/><Field name="password" required label={t('密码')} autoComplete="current-password" value={login.password} onChange={v=>setLogin({...login,password:v})}/><button className="primary" disabled={signingIn}>{t(signingIn?'正在登录…':'登录')}</button></form>:<p>{t(signingIn?'正在使用已保存的凭证登录…':'正在连接管理服务…')}</p>}{error&&<p role="alert" className="error">{message(error)}</p>}<div className="toolbar"><button disabled={signingIn} onClick={init}>{t("重新连接")}</button>{onManage&&<button onClick={onManage}>{t("管理后端")}</button>}</div></div>;

 const status=data.status||{},c=status.counters||{},cache=data['dns/cache']||{stats:status.cache?.decision,response_stats:status.cache?.response},bgp=data['bgp/status']||{},health=data.health||[],events=data.events||[],responseStats=cache.response_stats,cacheTextForLocale=cacheText(currentLocale());
 const responseMetrics=responseStats&&[['entries',responseStats.entries],['response_capacity',responseStats.capacity],['hits',responseStats.hits],['misses',responseStats.misses],['evictions',responseStats.evictions],['responsePending',responseStats.stale_entries]].map(([label,value])=>[cacheTextForLocale(label),value??0]);
 return <div className={"layout "+(page==='Monitor'?'monitoring-page ':'')+(monitorFocus?'monitor-focus':'')}><Navigation pages={pages} sections={navSections} page={page} onSelect={setPage} backend={backend} mode={status.mode||'auto'} onManage={onManage} onSignOut={signOut}/><main><header className="topbar"><div><small>{t("RoutePilot / 管理控制台")}</small><h1>{t(pages.find(p=>p[0]===page)[1])}</h1><p className="page-description">{t(descriptions[page])}</p></div><Badge value={status.state||(error?'DISCONNECTED':'STARTING')}/></header><div className="page-content">{!monitorFocus&&<VersionNotice status={data.status} backend={backend}/>}{!monitorFocus&&(backend||!nativeApp())&&<BackendUpdate backend={backend} capable={status.online_update} software={status.software} authentication={data.settings?.security?.authentication!==false} onReady={result=>{if(!backend&&!nativeApp()&&result?.phase==='succeeded')location.reload();else refresh()}} target={updateTarget} actionsTarget={updateActionsTarget}/>}{status.warning&&<div className="notice">{message(status.warning)}</div>}{error&&<div role="alert" className="notice error">{message(error)}</div>}
 <OperationNotice/><ConfigurationApplyNotice status={status.configuration_application} refresh={refresh} notify={notify}/>{page==='Dashboard'&&<><div className="metrics">{metric(t('每分钟查询'),status.queries_per_minute??0)}{metric('DIRECT',c.Queries?`${Math.round(c.Direct/c.Queries*100)}%`:'—')}{metric('SURGE',c.Queries?`${Math.round(c.Surge/c.Queries*100)}%`:'—')}{metric('REJECT',c.Queries?`${Math.round(c.Reject/c.Queries*100)}%`:'—')}{metric(t('缓存命中'),cache.stats?.hits+cache.stats?.misses?`${Math.round(cache.stats.hits/(cache.stats.hits+cache.stats.misses)*100)}%`:'—')}</div><div className="grid-two"><section className="panel"><h2>{t("服务状态")}</h2><Table rows={health} columns={[{key:'name',label:t('组件'),render:r=>valueLabel(r.name)},{key:'state',label:t('健康'),render:r=><Badge value={r.state}/>},{key:'latency_ms',label:t('延迟 ms')}]}/></section><section className="panel"><h2>{t("DNS 与路由")}</h2><dl><dt>{t("Surge 配置")}</dt><dd>{data['surge/status']?.snapshot?.profile||'—'}</dd><dt>{t("Surge 模式")}</dt><dd>{data['surge/status']?.snapshot?.mode||'—'}</dd><dt>{t("透明拦截")}</dt><dd>{c.Intercepted||0}</dd><dt>{t("直接请求")}</dt><dd>{c.Addressed||0}</dd><dt>Fake-IP</dt><dd><Badge value={status.fake_ip_enabled}/></dd><dt>{t("BGP 已发布 / 上限")}</dt><dd>{bgp.advertised||0} / {bgp.limit||128}</dd></dl><Table rows={bgp.peers||[]} columns={[{key:'address'},{key:'state',render:r=><Badge value={r.state}/>}]}/></section></div><section className="panel"><h2>{t("最近事件")}</h2><EventTable rows={events.slice(0,8)}/></section></>}
 {page==='Monitor'&&<MonitoringPanel snapshot={status.dns_monitoring} clientSettings={data.settings?.dns?.clients||[]} historyStatus={status.dns_monitoring_history} backendName={backend?.name} backendURL={backend?.url||location.origin} focus={monitorFocus} onFocus={setMonitorFocus} loading={!data.status&&!error} unavailable={Boolean(error)}/>}
 {page==='DNS'&&<><div className="metrics">{Object.entries(c).map(([k,v])=>metric(k,v))}</div><DNSForwardingOverview settings={data.settings} surge={data['surge/status']} Table={Table}/></>}
 {page==='Queries'&&<>{responseMetrics&&<div className="metrics">{responseMetrics.map(([label,value])=>metric(label,value))}</div>}<OptimisticReliability kind="response" stats={responseStats} enabled={data.settings?.cache?.optimistic_responses}/><Queries rows={data['dns/queries']||[]}/></>}
 {page==='Clients'&&<ClientsPanel snapshot={data['dns/clients']} configs={data['dns/client-settings']} refresh={refresh} notify={notify}/>}
 {page==='Cache'&&<CachePanel notify={notify}/>}
 {page==='Interception'&&<div className="notice">UDP/53 · TCP/53 <Badge value={data.settings?.dns.interception_enabled&&health.find(h=>h.name==='Transparent DNS')?.state==='UP'}/>{' · '}{data.settings?.dns.preserve_original?t('保留原始解析器'):t('配置的上游池')}</div>}{paths[page]&&<Resource page={page} rows={data[paths[page]]||[]} refresh={refresh} notify={notify} bgpRoutes={bgp.routes||[]} peerStates={bgp.peers||[]} realIPSnapshot={data['surge/status']?.snapshot||{}} settings={data.settings||{}}/>}
 {page==='Rule'&&<Rule notify={notify}/>}
 {page==='Routing'&&<RoutingPanel settings={data.settings} snapshot={data['routing/status']} refresh={refresh} notify={notify}/>}
 {page==='BGP'&&<><div className="metrics">{metric(t('BGP 引擎'),bgp.enabled?t('运行中'):t('未启用'))}{metric(t('本地 ASN'),data.settings?.bgp.local_asn)}{metric('Router ID',data.settings?.bgp.router_id||t('尚未设置'))}{metric(t('已发布 / 上限'),`${bgp.advertised||0} / ${bgp.limit||128}`)}{metric(t('已建立会话 / 已启用邻居'),`${(bgp.peers||[]).filter(p=>p.state==='ESTABLISHED').length} / ${(data['bgp/peers']||[]).filter(p=>p.enabled).length}`)}</div><div className="toolbar"><span>{t("邻居配置、会话详情与 FRR 导出统一在 BGP 邻居页面管理。")}</span><button onClick={()=>setPage('Peers')}>{t("管理 BGP 邻居")}</button></div>{!bgp.enabled&&<div className="notice">{t("BGP 未启用。DNS 分流继续运行；外部静态路由状态请在“路由”查看。")}</div>}<section className="panel"><h2>{t("BGP 路由")}</h2><Table rows={(bgp.routes||[]).filter(r=>r.backend!=='static')} columns={[{key:'prefix'},{key:'kind'},{key:'next_hop'},{key:'route_state',label:t('路由状态')},{key:'installed',label:t('本地 RIB'),render:r=>r.installed?t('已加入'):t('未加入')},{key:'announced',label:t('BGP 发布'),render:r=><Badge value={r.announced}/>}]}/></section></>}
 {page==='Peers'&&<FRRExport settings={data.settings} peers={data['bgp/peers']||[]} notify={notify}/>}
 {page==='Health'&&<Table rows={health} columns={['name','state','last_success','last_failure','latency_ms','successes','failures','last_error'].map(key=>({key,render:key==='state'?r=><Badge value={r.state}/>:key==='name'?r=>valueLabel(r.name):undefined}))}/>}
 {page==='Events'&&<EventTable rows={events}/>}
 {page==='Settings'&&<Settings onUpdateTarget={setUpdateTarget} onUpdateActionsTarget={setUpdateActionsTarget} settings={data.settings} interception={data['dns/interception']||[]} status={status} backend={backend} refresh={refresh} notify={notify}/>}
 </div><footer className="main-foot">{message(status.warning)||t('DNS 策略由 Surge 求值')}<span>{backend?t('远程管理 · 每 3 秒刷新'):t('实时更新 · SSE')}</span></footer></main>{toast&&<div role="status" className={'toast '+(toast.bad?'error':'')}>{message(toast.message)}</div>}</div>
}
export function App(){const t=useT();
 const [localSession,setLocalSession]=useState(null),[backend,setBackend]=useState(null),[revision,setRevision]=useState(0),[manage,setManage]=useState(false),[error,setError]=useState('');
 const localSessionWithCredentials=async()=>{
  let session=await localRequest('session');setLocalCSRF(session.csrf);setError('');
  if(!session.authenticated){
   try{const restored=await restoreLocalLogin();if(restored.session){session={...session,...restored.session};setLocalCSRF(session.csrf)}}catch(e){if(e.retryable)throw e;setError(e.message)}
  }
  return session;
 };
 const savedBackend=async session=>{
  if(!session.authenticated)return null;
  const profiles=await localRequest('management/backends'),saved=window.routepilotNativePreferences?.managementBackend;
  const sessionID=sessionStorage.getItem('routepilot.backend'),id=sessionID||saved?.id;
  const selected=(profiles||[]).find(profile=>profile.id===id&&(sessionID||profile.url===saved?.url));
  if(id&&!selected){sessionStorage.removeItem('routepilot.backend');rememberManagementBackend(null)}
  return selected||null;
 };
 useEffect(()=>{
  let cancelled=false,timer,attempts=0;
  const bootstrap=async()=>{
   try{
    const session=await localSessionWithCredentials();if(cancelled)return;
    const selected=await savedBackend(session);if(cancelled)return;
    selectBackend(selected?.id||'');rememberManagementBackend(selected);setBackend(selected);setRevision(v=>v+1);setLocalSession(session);
   }catch(e){if(!cancelled){setError(e.message);if(++attempts<6)timer=setTimeout(bootstrap,3000)}}
  };
  bootstrap();return()=>{cancelled=true;clearTimeout(timer)};
 },[]);
 useEffect(()=>{let reconnecting=false;const expired=async()=>{
  if(reconnecting)return;reconnecting=true;window.webkit?.messageHandlers?.routepilotServiceHealth?.postMessage({clear:true});selectBackend('');setBackend(null);setLocalSession(null);setManage(false);setError('');
  try{const session=await localSessionWithCredentials(),selected=await savedBackend(session);selectBackend(selected?.id||'');setBackend(selected);setRevision(v=>v+1);setLocalSession(session)}catch(e){setError(e.message)}finally{reconnecting=false}
 };window.addEventListener('routepilot-local-session-expired',expired);return()=>window.removeEventListener('routepilot-local-session-expired',expired)},[]);
 const switchTo=next=>{window.webkit?.messageHandlers?.routepilotServiceHealth?.postMessage({clear:true});selectBackend(next?.id||'');setBackend(next);setRevision(v=>v+1);setManage(false);rememberManagementBackend(next);if(next)sessionStorage.setItem('routepilot.backend',next.id);else sessionStorage.removeItem('routepilot.backend')};
 if(!localSession)return <div className="login"><h1>RoutePilot</h1><p>{t(window.routepilotNativePreferences?.localCredentialsSupported?'正在恢复已保存的登录状态…':'正在连接本地管理服务…')}</p>{error&&<p role="alert" className="error">{message(error)}</p>}<button onClick={()=>location.reload()}>{t('重新连接')}</button></div>;
 return <>{error&&<div role="alert" className="notice error">{message(error)}</div>}<Workspace key={revision} backend={backend} onLocalSession={session=>{setLocalSession(session);if(session.authenticated)setError('')}} onManage={localSession.authenticated?()=>setManage(true):undefined}/>{manage&&<Backends active={backend} onSelect={switchTo} onClose={()=>setManage(false)}/>}</>;
}
function EventTable({rows}){const t=useT();return <Table rows={rows} columns={[{key:'at',label:t('时间'),render:r=>new Date(r.at).toLocaleString(currentLocale())},{key:'severity',label:t('级别'),render:r=><Badge value={r.severity}/>},{key:'message',label:t('事件'),render:r=>message(r.message)}]}/>}
if(document.getElementById('root'))createRoot(document.getElementById('root')).render(<App/>);
