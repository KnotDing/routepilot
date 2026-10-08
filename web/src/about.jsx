import React from 'react';
import release from '../../internal/buildinfo/release.json';
import {useLocale} from './locale';
import {AppIcon} from './app-icon';
import {valueLabel} from './i18n';

const text={
 'zh-CN':{about:'关于',description:'DNS 与路由控制器',interface:'网页界面',version:'版本',build:'构建编号',backend:'当前后端',connection:'连接方式',local:'本地服务',remote:'远程服务',name:'服务名称',address:'管理地址',platform:'运行平台',runtime:'运行环境',state:'服务状态',uptime:'运行时间',unknown:'未提供',legacy:'当前后端未提供版本信息，更新后端后可查看。',language:'页面语言',days:'天',hours:'小时',minutes:'分钟',seconds:'秒'},
 en:{about:'About',description:'DNS and routing controller',interface:'Web interface',version:'Version',build:'Build',backend:'Current backend',connection:'Connection',local:'Local service',remote:'Remote service',name:'Service name',address:'Management address',platform:'Platform',runtime:'Runtime',state:'Service state',uptime:'Uptime',unknown:'Unavailable',legacy:'This backend does not provide version information. Update the backend to view it.',language:'Page language',days:'d',hours:'h',minutes:'m',seconds:'s'}
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
export function About({status={},backend=null}){
 const [locale]=useLocale(),t=text[locale]||text['zh-CN'];
 const software=status.software||{};
 const rows=[
  [t.connection,backend?t.remote:t.local],
  [t.name,status.name||backend?.name||'RoutePilot'],
  [t.address,backend?.url||location.origin],
  [t.version,software.version||t.unknown],
  [t.build,software.build||t.unknown],
  [t.platform,software.os&&software.arch?`${software.os==='darwin'?'macOS':software.os} · ${software.arch}`:t.unknown],
  [t.runtime,software.go_version||t.unknown],
  [t.state,valueLabel(status.state)||t.unknown],
  [t.uptime,uptime(status.uptime_seconds,t)]
 ];
 return <section className="panel about-panel" aria-label={t.about}>
  <header className="routing-heading"><div className="about-brand"><AppIcon/><div><h2>RoutePilot</h2><p>{t.description}</p></div></div></header>
  <section aria-label={t.interface}><h3>{t.interface}</h3><dl><dt>{t.version}</dt><dd>{release.version}</dd><dt>{t.build}</dt><dd>{release.build}</dd></dl></section>
  <section aria-label={t.backend}><h3>{t.backend}</h3><dl>{rows.map(([label,value])=><React.Fragment key={label}><dt>{label}</dt><dd>{value}</dd></React.Fragment>)}</dl>{!software.version&&<p className="about-note" role="status">{t.legacy}</p>}</section>
 </section>;
}
