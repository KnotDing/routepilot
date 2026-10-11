import React,{useEffect,useRef} from 'react';
import {useT,message,valueLabel} from './i18n';
import {currentLocale} from './locale';

function display(value,t){
 if(value===undefined||value===null||value==='')return '—';
 if(typeof value==='boolean')return t(value?'是':'否');
 return valueLabel(value);
}
function Properties({items}){
 const t=useT();
 return <dl>{items.map(([label,value])=><React.Fragment key={label}><dt>{t(label)}</dt><dd>{display(value,t)}</dd></React.Fragment>)}</dl>;
}
function timestamp(value){
 if(!value||String(value).startsWith('0001-01-01T'))return undefined;
 const date=new Date(value);
 return Number.isNaN(date.getTime())?value:date.toLocaleString(currentLocale());
}
function familyName(family){
 if(!family)return '—';
 const afi={AFI_IP:'IPv4',AFI_IP6:'IPv6'}[family.afi]||family.afi;
 const safi={SAFI_UNICAST:'Unicast',SAFI_MULTICAST:'Multicast'}[family.safi]||family.safi;
 return [afi,safi].filter(Boolean).join(' / ');
}
const capabilityNames={MultiProtocolCapability:'多协议扩展',RouteRefreshCapability:'路由刷新',CarryingLabelInfoCapability:'标签信息',ExtendedNexthopCapability:'扩展下一跳',GracefulRestartCapability:'优雅重启',FourOctetASNCapability:'四字节 ASN',AddPathCapability:'多路径通告',EnhancedRouteRefreshCapability:'增强路由刷新',LongLivedGracefulRestartCapability:'长期优雅重启',RouteRefreshCiscoCapability:'Cisco 路由刷新',FqdnCapability:'主机名',SoftwareVersionCapability:'软件版本',UnknownCapability:'未知能力'};
function capabilitySortKey(capability){
 const fields=Object.fromEntries(Object.entries(capability||{}).filter(([key])=>key!=='@type').sort(([a],[b])=>a<b?-1:a>b?1:0));
 return `${(capability?.['@type']||'').split('.').pop()}\u0000${JSON.stringify(fields)}`;
}
function Capabilities({title,values,established,sort=false}){
 const t=useT();
 const sortedValues=sort&&values?[...values].sort((a,b)=>{const left=capabilitySortKey(a),right=capabilitySortKey(b);return left<right?-1:left>right?1:0}):values;
 return <section className="panel"><h3>{t(title)}</h3>{!established?<p className="muted">{t('会话尚未建立，能力协商信息暂不可用。')}</p>:!sortedValues?.length?<p className="muted">{t('无能力信息')}</p>:<ul className="peer-capabilities">{sortedValues.map((capability,index)=>{
  const type=(capability['@type']||'').split('.').pop(),fields=Object.fromEntries(Object.entries(capability).filter(([key])=>key!=='@type'));
  return <li key={index}><details><summary>{t(capabilityNames[type]||type||'未知能力')}{capability.family&&' · '+familyName(capability.family)}{capability.asn!==undefined&&' · AS'+capability.asn}</summary><pre>{JSON.stringify(fields,null,2)}</pre></details></li>;
 })}</ul>}</section>;
}
export function BGPPeerDetails({peer,live,settings,onClose}){
 const t=useT(),closeButton=useRef(null);
 useEffect(()=>{const previous=document.activeElement;closeButton.current?.focus();return()=>{if(previous?.isConnected)previous.focus()}},[]);
 const detail=live?.details,conf=detail?.conf||{},state=detail?.state||{},transport=detail?.transport||{},timers=detail?.timers||{};
 const timerState=timers.state||{},timerConfig=timers.config||{},established=live?.state==='ESTABLISHED';
 const configuredHold=peer.hold_seconds||settings.bgp?.hold_seconds;
 const configuredKeepalive=peer.hold_seconds?peer.keepalive_seconds:settings.bgp?.keepalive_seconds;
 const messages=state.messages,seconds=value=>value===undefined||value===null?undefined:t('{count} 秒',{count:value});
 return <div className="modal" role="dialog" aria-modal="true" aria-labelledby="peer-detail-title"><section className="peer-details">
  <header><h2 id="peer-detail-title">{t('BGP 邻居详情')} · {peer.address}</h2><button ref={closeButton} type="button" onClick={onClose}>{t('关闭')}</button></header>
  <p className="muted">{t('运行信息随页面自动刷新；发布统计来自本机 BGP，不代表网关已安装路由。')}</p>
  {!live&&<p className="notice">{t(peer.enabled?'尚未取得该邻居的运行信息，请检查 BGP 是否启用及配置是否已生效。':'该邻居已禁用，仅显示保存的配置。')}</p>}
  {live&&!detail&&<p className="notice">{live.details_error?message(live.details_error):t('当前后端未提供完整邻居信息，升级后端后可查看；下面仍显示已有状态与配置。')}</p>}
  <div className="peer-detail-grid">
   <section className="panel"><h3>{t('会话信息')}</h3><Properties items={[
    ['会话状态',live?.state],['管理状态',state.admin_state==='PFX_CT'?t('超过前缀上限'):state.admin_state],['邻居类型',({INTERNAL:'iBGP',EXTERNAL:'eBGP'})[state.type??conf.type]||state.type||conf.type],['本地 ASN',state.local_asn??conf.local_asn],['远端 ASN',state.peer_asn??live?.asn],['远端 Router ID',state.router_id],['在线秒数',live?seconds(live.uptime_seconds):undefined],['最近建立',timestamp(timerState.uptime)],['最近断开',timestamp(timerState.downtime)],['会话中断次数',state.flops],['已发布前缀',live?.prefixes],['输出队列',state.out_q],['最近错误',live?.last_error?message(live.last_error):undefined]
   ]}/></section>
   <section className="panel"><h3>{t('连接信息')}</h3><Properties items={[
    ['本地地址',transport.local_address],['本地绑定端口',transport.local_port===0?t('自动分配'):transport.local_port],['远端地址',transport.remote_address||state.neighbor_address||conf.neighbor_address],['远端连接端口',transport.remote_port||undefined],['绑定接口',transport.bind_interface],['被动连接',transport.passive_mode],['MTU 探测',transport.mtu_discovery],['TCP MSS',transport.tcp_mss],['eBGP 多跳',detail?.ebgp_multihop?.enabled],['多跳 TTL',detail?.ebgp_multihop?.multihop_ttl],['TTL 安全',detail?.ttl_security?.enabled],['最小 TTL',detail?.ttl_security?.ttl_min]
   ]}/></section>
   <section className="panel"><h3>{t('运行计时器')}</h3><Properties items={[
    ['Hold Time',seconds(timerConfig.hold_time)],['Keepalive 间隔',seconds(timerState.keepalive_interval??timerConfig.keepalive_interval)],['协商 Hold Time',established?seconds(timerState.negotiated_hold_time):undefined],['连接重试间隔',seconds(timerConfig.connect_retry)],['最小通告间隔',seconds(timerConfig.minimum_advertisement_interval)],['重置后等待',seconds(timerConfig.idle_hold_time_after_reset)]
   ]}/></section>
   <section className="panel"><h3>{t('保存的邻居配置')}</h3><Properties items={[
    ['邻居地址',peer.address],['启用',peer.enabled],['描述',peer.description],['远端 ASN',peer.asn],['本地 ASN',settings.bgp?.local_asn],['本地 Router ID',settings.bgp?.router_id],['远端端口',peer.port||179],['Hold Time',seconds(configuredHold)],['Keepalive 间隔',seconds(configuredKeepalive)]
   ]}/></section>
  </div>
  {messages&&<section className="panel"><h3>{t('BGP 消息统计')}</h3><div className="table-wrap"><table><thead><tr><th>{t('类型')}</th><th>{t('接收')}</th><th>{t('发送')}</th></tr></thead><tbody>{[['total','总计'],['open','OPEN'],['update','UPDATE'],['keepalive','KEEPALIVE'],['notification','NOTIFICATION'],['refresh','ROUTE-REFRESH'],['discarded','丢弃'],['withdraw_update','撤回 UPDATE'],['withdraw_prefix','撤回前缀']].map(([key,label])=><tr key={key}><td>{t(label)}</td><td>{messages.received?.[key]??'—'}</td><td>{messages.sent?.[key]??'—'}</td></tr>)}</tbody></table></div></section>}
  {detail?.afi_safis?.length>0&&<section className="panel"><h3>{t('地址族与前缀统计')}</h3><div className="table-wrap"><table><thead><tr>{['地址族','启用','接收','接受','GoBGP 通告数','前缀上限'].map(label=><th key={label}>{t(label)}</th>)}</tr></thead><tbody>{detail.afi_safis.map((family,index)=><tr key={index}><td>{familyName(family.state?.family||family.config?.family)}</td><td>{display(family.config?.enabled,t)}</td><td>{family.state?.received??'—'}</td><td>{family.state?.accepted??'—'}</td><td>{established?(family.state?.advertised??'—'):'—'}</td><td>{family.prefix_limits?.max_prefixes??'—'}</td></tr>)}</tbody></table></div><p className="muted">{t('地址族统计为 GoBGP 运行快照；实际导出前缀总数见“已发布前缀”。')}</p></section>}
  {detail&&<><div className="peer-detail-grid"><Capabilities title="本地通告能力" values={state.local_cap} established={established}/><Capabilities title="远端通告能力" values={state.remote_cap} established={established} sort/></div><details className="peer-raw"><summary>{t('完整 GoBGP 信息（已隐藏认证密钥）')}</summary><pre>{JSON.stringify(detail,null,2)}</pre></details></>}
 </section></div>;
}
