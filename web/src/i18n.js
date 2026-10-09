import english from '../../localization/ui-en.json';
import errors from '../../localization/errors-zh.json';
import {currentLocale,useLocale} from './locale';

export function translate(text,params={},locale=currentLocale()){
 if(typeof text!=='string')return text;
 const value=locale==='en'?(english[text]||text):text;
 return value.replace(/\{(\w+)\}/g,(match,key)=>Object.hasOwn(params,key)?String(params[key]):match);
}
export function useT(){const [locale]=useLocale();return (text,params)=>translate(text,params,locale)}

const reverse=Object.fromEntries(Object.entries(english).map(([zh,en])=>[en,zh]));
function compileTemplate(template){
 const keys=[];
 const parts=template.split(/(\{\w+\})/g).map(part=>{
  if(/^\{\w+\}$/.test(part)){keys.push(part.slice(1,-1));return '([\\s\\S]*?)'}
  return part.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
 });
 if(!keys.length)return null;
 return {keys,expression:new RegExp('^'+parts.join('')+'$')};
}
function fill(template,params){return template.replace(/\{(\w+)\}/g,(match,key)=>params[key]??match)}
const templates=Object.entries(english).filter(([zh])=>/\{\w+\}/.test(zh)).map(([zh,en])=>({zh,en,zhPattern:compileTemplate(zh),enPattern:compileTemplate(en)}));
const errorTemplates=Object.entries(errors).filter(([en])=>/\{\w+\}/.test(en)).map(([en,zh])=>({zh,en,enPattern:compileTemplate(en)}));
function matchTemplate(pattern,text){const match=pattern.expression.exec(text);return match?Object.fromEntries(pattern.keys.map((key,i)=>[key,match[i+1]])):null}

// Translate application diagnostics; domain names, rules and raw DNS data stay intact.
export function message(text,locale=currentLocale(),depth=0){
 if(typeof text!=='string')return text;
 if(locale==='en'&&english[text])return english[text];
 if(locale!=='en'&&(errors[text]||reverse[text]))return errors[text]||reverse[text];
 for(const {zh,en,zhPattern,enPattern} of templates){
  const params=matchTemplate(locale==='en'?zhPattern:enPattern,text);
  if(params){if(params.reason&&depth<3)params.reason=message(params.reason,locale,depth+1);return fill(locale==='en'?en:zh,params)}
 }
 if(locale!=='en')for(const {zh,enPattern} of errorTemplates){
  const params=matchTemplate(enPattern,text);if(params){if(params.reason&&depth<3)params.reason=message(params.reason,locale,depth+1);return fill(zh,params)}
 }
 return text;
}
const values={
 auto:['自动分流','Automatic'], 'force-direct':['强制直连','Force direct'], 'force-surge':['全部交给 Surge','Send all to Surge'],
 system:['跟随系统','System'],light:['浅色','Light'],dark:['深色','Dark'],
 none:['无','None'],controller:['Controller','Controller'],surge:['Surge','Surge'],dns:['DNS','DNS'],
 exact:['精确匹配','Exact'],suffix:['后缀匹配','Suffix'],wildcard:['通配符','Wildcard'],
 debug:['调试','Debug'],info:['信息','Info'],warn:['警告','Warning'],error:['错误','Error'],
 STARTING:['启动中','Starting'],RECOVERING:['恢复中','Recovering'],HEALTHY:['健康','Healthy'],SURGE_DEGRADED:['Surge 降级','Surge degraded'],DIRECT_FALLBACK:['真实 DNS 回退','Real DNS fallback'],STOPPING:['停止中','Stopping'],
 UP:['正常','Up'],DOWN:['故障','Down'],ERROR:['错误','Error'],DEGRADED:['降级','Degraded'],DISABLED:['已禁用','Disabled'],DISCONNECTED:['未连接','Disconnected'],ESTABLISHED:['已建立','Established'],IDLE:['空闲','Idle'],ACTIVE:['连接中','Active'],CONNECT:['正在连接','Connecting'],OPENSENT:['已发送 OPEN','Open sent'],OPENCONFIRM:['确认 OPEN','Open confirm'],
 Announced:['已发布','Announced'],Inactive:['未发布','Inactive'],
 block:['关闭 IPv6 时拦截 AAAA（成功空响应）','Block AAAA when IPv6 is disabled (NODATA)'],upstream:['关闭 IPv6 时直接查询上游','Query upstream directly when IPv6 is disabled'],'surge-ipv6-fallback':['Surge IPv6 回落','Surge IPv6 fallback'],
 'local-dns':['本地 DNS','Local DNS'],'surge-always-real-ip':['Surge always-real-ip','Surge always-real-ip'],cache:['缓存','Cache'],manual:['手动','Manual'],'surge-import':['Surge 导入','Surge import'],override:['域名覆盖','Domain override'],mode:['全局模式','Global mode'],safety:['安全检查','Safety check'],'state-fallback':['状态回退','State fallback'],'dns-type':['DNS 类型','DNS type'],
 Controller:['控制器','Controller'],'Real DNS':['真实 DNS','Real DNS'],'Surge CLI':['Surge CLI','Surge CLI'],'Surge Fake-IP DNS':['Surge Fake-IP DNS','Surge Fake-IP DNS'],'Managed routes':['受管理路由','Managed routes'],'Transparent DNS':['透明 DNS','Transparent DNS'],'Local DNS':['本地 DNS','Local DNS'],'DNS listener':['DNS 监听','DNS listener'],'BGP engine':['BGP 引擎','BGP engine'],'Fake-IP routing':['Fake-IP 路由','Fake-IP routing'],'DNS interception routing':['DNS 拦截路由','DNS interception routing'],
 Queries:['总查询','Queries'],Direct:['DIRECT','DIRECT'],Surge:['SURGE','SURGE'],Reject:['REJECT','REJECT'],Unknown:['UNKNOWN','UNKNOWN'],Intercepted:['透明拦截','Intercepted'],Addressed:['直接请求','Addressed'],Errors:['错误','Errors'],
 hit:['命中','Hit'],miss:['未命中','Miss'],intercepted:['透明拦截','Intercepted'],direct:['直接请求','Addressed']
 ,certain:['确定','Certain'],configured:['按配置采用','Configured'],uncertain:['未确定','Uncertain'],bgp:['BGP','BGP'],static:['静态','Static'],hybrid:['混合','Hybrid'],
 'fake-ip':['Fake-IP','Fake-IP'],'dns-interception':['DNS 拦截','DNS interception'],'bgp-announced':['BGP 已发布','BGP announced'],'bgp-installed':['已加入 BGP','BGP installed'],'bgp-pending':['等待 BGP','BGP pending'],'operator-confirmed':['操作员已确认','Operator confirmed'],'declared-unverified':['声明但未验证','Declared, unverified'],Database:['数据库','Database'],'Surge API':['Surge API','Surge API']
};
export function valueLabel(value,locale=currentLocale()){
 if(typeof value==='boolean')return translate(value?'启用':'禁用',{},locale);
 if(typeof value==='string'&&value.startsWith('BGP peer '))return locale==='en'?value:'BGP 邻居 '+value.slice(9);
 return values[value]?.[locale==='en'?1:0]??value;
}
