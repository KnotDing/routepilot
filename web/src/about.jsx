import React from 'react';
import release from '../../internal/buildinfo/release.json';
import {useLocale} from './locale';
import {AppIcon} from './app-icon';
import {valueLabel} from './i18n';

const text={
 'zh-CN':{about:'关于',description:'DNS 与路由控制器',interface:'网页界面',version:'版本',build:'构建编号',backend:'当前后端',connection:'连接方式',local:'本地服务',remote:'远程服务',address:'管理地址',platform:'运行平台',runtime:'运行环境',state:'服务状态',uptime:'运行时间',goRuntime:'Go 运行情况',goroutines:'Goroutine 数',maxProcs:'最大并行数',heapObjects:'堆对象内存',runtimeMemory:'Go 运行时内存',processRSS:'后台进程 RSS',processRSSUnavailable:'当前后端未提供进程 RSS。',processRSSError:'进程 RSS 读取失败：{error}',gcCycles:'GC 次数',runtimeUnavailable:'当前后端未提供 Go 运行指标；旧版本后端可能不支持。',runtimeNote:'RSS 仅指当前 Go 后台进程，不包括 macOS App 主进程或 WebKit 网页内容进程；Go runtime 指标也不等同于进程 RSS。',unknown:'未提供',legacy:'当前后端未提供版本信息，更新后端后可查看。',language:'页面语言',days:'天',hours:'小时',minutes:'分钟',seconds:'秒'},
 en:{about:'About',description:'DNS and routing controller',interface:'Web interface',version:'Version',build:'Build',backend:'Current backend',connection:'Connection',local:'Local service',remote:'Remote service',address:'Management address',platform:'Platform',runtime:'Runtime',state:'Service state',uptime:'Uptime',goRuntime:'Go runtime',goroutines:'Goroutines',maxProcs:'Max parallelism',heapObjects:'Heap object memory',runtimeMemory:'Go runtime memory',processRSS:'Backend process RSS',processRSSUnavailable:'This backend does not provide process RSS.',processRSSError:'Unable to read process RSS: {error}',gcCycles:'GC cycles',runtimeUnavailable:'This backend does not provide Go runtime metrics; older versions may not support them.',runtimeNote:'RSS covers only the current Go backend process; it excludes the macOS app and WebKit web-content processes. Go runtime metrics are also not equivalent to process RSS.',unknown:'Unavailable',legacy:'This backend does not provide version information. Update the backend to view it.',language:'Page language',days:'d',hours:'h',minutes:'m',seconds:'s'}
};
function uptime(value,t){
 if(!Number.isFinite(value)||value<0)return t.unknown;
 let remaining=Math.floor(value);
 const parts=[];
 for(const [size,key] of [[86400,'days'],[3600,'hours'],[60,'minutes'],[1,'seconds']]){
  const count=Math.floor(remaining/size);remaining%=size;
  if(count||(!parts.length&&size===1))parts.push(`${count} ${t[key]}`);
 }
 return parts.join(' ');
}
function count(value,locale,t){
 return Number.isFinite(value)?new Intl.NumberFormat(locale).format(value):t.unknown;
}
function bytes(value,locale,t){
 if(!Number.isFinite(value)||value<0)return t.unknown;
 const units=['B','KiB','MiB','GiB','TiB'];let amount=value,index=0;
 while(amount>=1024&&index<units.length-1){amount/=1024;index++}
 return `${new Intl.NumberFormat(locale,{maximumFractionDigits:index?1:0}).format(amount)} ${units[index]}`;
}
export function About({status={},backend=null}){
 const [locale]=useLocale(),t=text[locale]||text['zh-CN'];
 const software=status.software||{};
 const go=status.go_runtime;
 const rows=[
  [t.connection,backend?t.remote:t.local],
  [t.address,backend?.url||location.origin],
  [t.version,software.version||t.unknown],
  [t.build,software.build||t.unknown],
  [t.platform,software.os&&software.arch?`${software.os==='darwin'?'macOS':software.os} · ${software.arch}`:t.unknown],
  [t.runtime,software.go_version||t.unknown],
  [t.state,valueLabel(status.state)||t.unknown],
  [t.uptime,uptime(status.uptime_seconds,t)]
 ];
 const goRows=go&&[
  [t.goroutines,count(go.goroutines,locale,t)],
  [t.maxProcs,count(go.max_procs,locale,t)],
  [t.heapObjects,bytes(go.heap_objects_bytes,locale,t)],
  [t.runtimeMemory,bytes(go.memory_total_bytes,locale,t)],
  [t.processRSS,go.process_rss_bytes?bytes(go.process_rss_bytes,locale,t):t.unknown],
  [t.gcCycles,count(go.gc_cycles,locale,t)]
 ];
 return <section className="panel about-panel" aria-label={t.about}>
  <header className="routing-heading"><div className="about-brand"><AppIcon/><div><h2>RoutePilot</h2><p>{t.description}</p></div></div></header>
  <section aria-label={t.interface}><h3>{t.interface}</h3><dl><dt>{t.version}</dt><dd>{release.version}</dd><dt>{t.build}</dt><dd>{release.build}</dd></dl></section>
  <section aria-label={t.backend}><h3>{t.backend}</h3><dl>{rows.map(([label,value])=><React.Fragment key={label}><dt>{label}</dt><dd>{value}</dd></React.Fragment>)}</dl>{!software.version&&<p className="about-note" role="status">{t.legacy}</p>}</section>
  <section aria-label={t.goRuntime}><h3>{t.goRuntime}</h3>{goRows?<dl>{goRows.map(([label,value])=><React.Fragment key={label}><dt>{label}</dt><dd>{value}</dd></React.Fragment>)}</dl>:<p className="about-note" role="status">{t.runtimeUnavailable}</p>}{go?.process_rss_error&&<p className="about-note" role="status">{t.processRSSError.replace('{error}',go.process_rss_error)}</p>}{go&&!go.process_rss_bytes&&!go.process_rss_error&&<p className="about-note" role="status">{t.processRSSUnavailable}</p>}<p className="about-note">{t.runtimeNote}</p></section>
 </section>;
}
