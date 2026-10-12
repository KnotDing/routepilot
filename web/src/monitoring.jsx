import './monitoring.css';
import {FlowSourceLabel,useSourceLabels,wrapSourceLabel} from './monitoring-source-label';
import React,{useEffect,useMemo,useRef,useState} from 'react';
import {useT,message} from './i18n';
import {MonitoringControls,SourceStatistics,useMonitoringData,historicalSamples,sourceName} from './monitoring-history';
import {currentLocale} from './locale';
import {appendSample,destinationLabels,ingressLabels,flowLayout,sourceFlows,policyDistribution,share} from './monitoring-data';

const colors={addressed:'var(--accent)',intercepted:'#8872db',local:'var(--success-text)',cache:'#12a5a0',optimistic_cache:'#d08b31',real:'var(--accent)',surge:'#8872db',blocked:'var(--warning-text)',unavailable:'var(--danger-text)'};
const number=n=>Number(n).toLocaleString(currentLocale(),{maximumFractionDigits:1});
const percent=(n,total)=>total?`${(share(n,total)*100).toLocaleString(currentLocale(),{minimumFractionDigits:1,maximumFractionDigits:2})}%`:'—';

function FlowChart({snapshot,sources,selected,clientSettings=[]}){
 const t=useT(),labels=useSourceLabels(clientSettings),classified=sourceFlows(snapshot,sources,selected),flows=classified?.flows||snapshot.flows,total=classified?flows.reduce((sum,f)=>sum+f.queries,0):snapshot.completed,labelLines=Math.max(1,...(classified?.keys||[]).map(key=>labels.lines(key).length)),{left,right,edges,svgHeight}=flowLayout(flows,total,classified?.keys,undefined,labelLines);
 const label=key=>classified?labels.label(key):t(ingressLabels[key]);
 const hasFlows=flows.some(f=>f.queries>0);
 return <section className="panel monitor-flow"><header className="section-heading"><h2>{t('DNS 请求流向')}</h2><p>{t('流带宽度按请求数同比例绘制；每条请求只归入一个最终处理去向，占比以展示来源的已完成请求为分母。')}</p></header>
  {!hasFlows?<div className="empty">{t(snapshot.completed?'暂无可展示的来源流向':'暂无已完成请求')}</div>:<>
   <div className="monitor-chart-scroll" tabIndex="0" aria-label={t('DNS 请求流向')}><svg viewBox={`0 0 980 ${svgHeight}`} className="monitor-sankey" role="img" aria-label={t(classified?'DNS 来源到实际处理去向的流向图':'DNS 接入方式到实际处理去向的流向图')}>
    <text x="20" y="22" className="monitor-svg-heading">{t('DNS 流入')}</text><text x="750" y="22" className="monitor-svg-heading">{t('DNS 处理去向')}</text>
    {edges.map(e=><path key={JSON.stringify([e.source,e.target])} d={e.path} fill={colors[e.target]} fillOpacity=".25"><title>{label(e.source)} → {t(destinationLabels[e.target])}: {number(e.count)} · {percent(e.count,total)}</title></path>)}
    {left.map(n=><g key={n.key}><rect x="210" y={n.y} width="14" height={n.height} fill={classified?'var(--accent)':colors[n.key]} rx="2"/><FlowSourceLabel node={n} labels={classified?labels:{...labels,label,lines:key=>[label(key)],named:()=>false,title:label}} value={`${number(n.count)} · ${percent(n.count,total)}`}/></g>)}
    {right.map(n=><g key={n.key}><rect x="716" y={n.y} width="14" height={n.height} fill={colors[n.key]} rx="2"/><text x="750" y={n.y+n.height/2-5}>{t(destinationLabels[n.key])}</text><text x="750" y={n.y+n.height/2+16} className="monitor-svg-value">{number(n.count)} · {percent(n.count,total)}</text></g>)}
   </svg></div>
   <details className="monitor-detail"><summary>{t('查看精确流向数据')}</summary><div className="table-wrap"><table><thead><tr><th>{t(classified?'请求来源':'接入方式')}</th><th>{t('DNS 处理去向')}</th><th>{t('DNS 请求数')}</th><th>{t('占已完成请求')}</th></tr></thead><tbody>{flows.map((f,i)=><tr key={i}><td>{label(f.ingress)}</td><td>{t(destinationLabels[f.destination]||f.destination)}</td><td>{number(f.queries)}</td><td>{percent(f.queries,total)}</td></tr>)}</tbody></table></div></details>
  </>}
  {!classified&&<p className="field-hint">{t('此后端未提供来源流向明细，暂按接入方式展示。')}</p>}
 </section>;
}

function PolicyNodeLabel({node,x,label,total}){
 const lines=wrapSourceLabel(label),center=node.y+node.height/2;
 return <><text x={x} y={center-(lines.length-1)*11-5} className="monitor-node-label">{lines.map((line,i)=><tspan key={i} x={x} dy={i?22:0}>{line}</tspan>)}<title>{label}</title></text><text x={x} y={center+(lines.length-1)*11+16} className="monitor-svg-value monitor-node-label">{number(node.count)} · {percent(node.count,total)}</text></>;
}

function ClientPolicyChart({rows,clientSettings=[]}){
 const t=useT(),labels=useSourceLabels(clientSettings),three=Array.isArray(rows)&&rows.every(row=>Boolean(row.final_kind));
 const labelOf=(kind,name,final=false)=>kind==='unnamed'?t(final?'未记录最终策略':'未记录策略'):kind==='overflow'?t('超出明细容量'):kind==='rest'?t(final?'其余最终策略':'其余策略'):name;
 const policyKey=row=>JSON.stringify([row.kind,row.policy]),finalKey=row=>JSON.stringify([row.final_kind,row.final_policy]),policies=new Map(),finals=new Map();
 for(const row of rows||[]){policies.set(policyKey(row),labelOf(row.kind,row.policy));if(three)finals.set(finalKey(row),labelOf(row.final_kind,row.final_policy,true))}
 const flows=(rows||[]).map(row=>({ingress:row.source,destination:policyKey(row),queries:row.queries})),finalFlows=three?(rows||[]).map(row=>({ingress:policyKey(row),destination:finalKey(row),queries:row.queries})):[],total=flows.reduce((sum,f)=>sum+f.queries,0);
 const rank=(list,field)=>[...new Set(list.map(f=>f[field]))].sort((a,b)=>list.filter(f=>f[field]===b).reduce((sum,f)=>sum+f.queries,0)-list.filter(f=>f[field]===a).reduce((sum,f)=>sum+f.queries,0)||a.localeCompare(b));
 const sourceKeys=rank(flows,'ingress'),policyKeys=rank(flows,'destination'),finalKeys=rank(finalFlows,'destination'),labelLines=Math.max(1,...sourceKeys.map(key=>labels.lines(key).length),...[...policies.values(),...finals.values()].map(label=>wrapSourceLabel(label).length)),nodeCount=Math.max(sourceKeys.length,policyKeys.length,finalKeys.length),middleX=three?600:716;
 const first=flowLayout(flows,total,sourceKeys,policyKeys,labelLines,{sourceX:210,targetX:middleX,nodeCount}),second=flowLayout(finalFlows,total,policyKeys,finalKeys,labelLines,{sourceX:600,targetX:1040,nodeCount});
 const palette=['var(--accent)','#8872db','#12a5a0','var(--success-text)','#d08b31','var(--danger-text)'],color=(key,keys)=>palette[keys.indexOf(key)%palette.length];
 return <section className="panel monitor-client-policy"><header className="section-heading"><h2>{t('客户端策略流向')}</h2><p>{t('按实际请求记录展示客户端、命中策略与最终策略，每一层的流带和节点共用请求数比例尺。')}</p></header>
  {!Array.isArray(rows)?<div className="notice">{t('此后端暂不支持客户端策略流向，请升级后端。')}</div>:!total?<div className="empty">{t('暂无已完成请求')}</div>:<>
   {!three&&<p className="field-hint">{t('此后端未提供最终策略流向，暂展示客户端到命中策略。')}</p>}
   <div className="monitor-chart-scroll" tabIndex="0" aria-label={t('客户端策略流向')}><svg viewBox={`0 0 ${three?1280:980} ${first.svgHeight}`} className={'monitor-sankey '+(three?'monitor-three-stage':'')} role="img" aria-label={t('客户端到策略或策略组的流向图')}>
    <text x="20" y="22" className="monitor-svg-heading">{t('客户端')}</text><text x={three?600:750} y="22" className="monitor-svg-heading">{t('策略 / 策略组')}</text>{three&&<text x="1075" y="22" className="monitor-svg-heading">{t('最终策略')}</text>}
    {first.edges.map(e=><path key={JSON.stringify([e.source,e.target])} d={e.path} fill={color(e.target,policyKeys)} fillOpacity=".25"><title>{labels.title(e.source)} → {policies.get(e.target)}: {number(e.count)} · {percent(e.count,total)}</title></path>)}
    {three&&second.edges.map(e=><path key={JSON.stringify([e.source,e.target])} d={e.path} fill={color(e.target,finalKeys)} fillOpacity=".25"><title>{policies.get(e.source)} → {finals.get(e.target)}: {number(e.count)} · {percent(e.count,total)}</title></path>)}
    {first.left.map(n=><g key={n.key}><rect x="210" y={n.y} width="14" height={n.height} fill="var(--accent)" rx="2"/><FlowSourceLabel node={n} labels={labels} value={`${number(n.count)} · ${percent(n.count,total)}`}/></g>)}
    {first.right.map(n=><g key={n.key}><rect x={middleX} y={n.y} width="14" height={n.height} fill={color(n.key,policyKeys)} rx="2"/><PolicyNodeLabel node={n} x={three?635:750} label={policies.get(n.key)} total={total}/></g>)}
    {three&&second.right.map(n=><g key={n.key}><rect x="1040" y={n.y} width="14" height={n.height} fill={color(n.key,finalKeys)} rx="2"/><PolicyNodeLabel node={n} x="1075" label={finals.get(n.key)} total={total}/></g>)}
   </svg></div>
   <details className="monitor-detail"><summary>{t('查看客户端策略流向明细')}</summary><div className="table-wrap monitor-policy-table"><table><thead><tr>{['请求来源','策略 / 策略组',...(three?['最终策略']:[]),'DNS 请求数','占已完成请求'].map(label=><th key={label}>{t(label)}</th>)}</tr></thead><tbody>{rows.map(row=><tr key={JSON.stringify([row.source,row.kind,row.policy,row.final_kind,row.final_policy])}><td>{labels.label(row.source)}</td><td>{labelOf(row.kind,row.policy)}</td>{three&&<td>{labelOf(row.final_kind,row.final_policy,true)}</td>}<td>{number(row.queries)}</td><td>{percent(row.queries,total)}</td></tr>)}</tbody></table></div></details>
  </>}
 </section>;
}

function Distribution({title,rows,total}){
 const t=useT();
 return <section className="panel monitor-distribution"><h2>{t(title)}</h2>{!total?<div className="empty">{t('暂无已完成请求')}</div>:<><div className="monitor-stacked" role="img" aria-label={t(title)}>{rows.filter(r=>r.count).map(r=><span key={r.key} title={`${r.label}: ${number(r.count)} · ${percent(r.count,total)}`} style={{width:`${share(r.count,total)*100}%`,background:r.color}}/>)}</div><div className="monitor-legend">{rows.map(r=><div key={r.key}><span className="monitor-dot" style={{background:r.color}}/><span>{r.label}</span><strong>{number(r.count)}</strong><small>{percent(r.count,total)}</small></div>)}</div></>}</section>;
}

function Trend({samples,historical=false,persisted=false}){
 const t=useT(),valid=samples.filter(s=>s.incoming!==null),peak=Math.max(1,...valid.flatMap(s=>[s.incoming,s.outgoing])),first=samples[0]?.at,last=samples.at(-1)?.at,span=Math.max(30000,(last||0)-(first||0)),latest=samples.at(-1),hasLatest=latest?.incoming!==null&&latest?.incoming!==undefined;
 const x=at=>65+(at-(last-span))/span*790,y=value=>205-value/peak*165;
 function line(field){let d='',connected=false;for(const s of samples){if(s[field]===null){connected=false;continue}if(s.breakBefore)connected=false;d+=`${connected?'L':'M'} ${x(s.at)} ${y(s[field])} `;connected=true}return d}
 return <section className="panel monitor-trend"><header className="section-heading"><h2>{t('DNS 流入 / 响应速率')}</h2><p>{t(persisted?'后台每五秒保存采样，按分钟聚合；单位为请求 / 秒，服务停止或采样中断处不连线。':'最近六分钟当前页面采样，单位：请求 / 秒。响应指后台生成的 DNS 响应；采样中断处不连线。')}</p></header>
  <div className="monitor-rate-legend"><span><i style={{background:'var(--accent)'}}/>{t('流入')}: {hasLatest?number(latest.incoming):'—'}</span><span><i style={{background:'#12a5a0'}}/>{t('响应')}: {hasLatest?number(latest.outgoing):'—'}</span></div>
  {!valid.length?<div className="empty">{t(historical?'所选时间范围暂无可用速率采样':'等待下一次采样以计算速率')}</div>:<svg viewBox="0 0 900 250" role="img" aria-label={t('DNS 流入与响应速率趋势')} className="monitor-trend-chart">{[0,.5,1].map(r=><g key={r}><line x1="65" x2="855" y1={y(peak*r)} y2={y(peak*r)} className="monitor-grid-line"/><text x="52" y={y(peak*r)+5} textAnchor="end">{number(peak*r)}</text></g>)}{valid.length===1&&['incoming','outgoing'].map(field=><circle key={field} cx={x(valid[0].at)} cy={y(valid[0][field])} r={field==='incoming'?4:2.5} fill={field==='incoming'?'var(--accent)':'#12a5a0'}><title>{t(field==='incoming'?'流入':'响应')}: {number(valid[0][field])}</title></circle>)}<path d={line('incoming')} fill="none" stroke="var(--accent)" strokeWidth="2.5"/><path d={line('outgoing')} fill="none" stroke="#12a5a0" strokeWidth="2.5" strokeDasharray="6 3"/>{[last-span,last-span/2,last].map(at=><text key={at} x={x(at)} y="237" textAnchor="middle">{(span>12*3600000?new Date(at).toLocaleString(currentLocale(),{month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'}):new Date(at).toLocaleTimeString(currentLocale(),{hour:'2-digit',minute:'2-digit',second:'2-digit'}))}</text>)}</svg>}
 </section>;
}

function Policies({snapshot,limit=512}){
 const t=useT(),[field,setField]=useState('policy'),[filter,setFilter]=useState('');
 const distribution=useMemo(()=>policyDistribution(snapshot.policies,field),[snapshot.policies,field]);
 const name=r=>r.kind==='overflow'?t('超出明细容量'):r.kind==='unnamed'?t('未记录策略'):r.name;
 const top=distribution.slice(0,10),rest=distribution.slice(10).reduce((s,r)=>s+r.count,0),bars=rest?[...top,{key:'rest',kind:'rest',name:t('其余策略'),count:rest}]:top;
 const rows=snapshot.policies.filter(r=>`${r.policy} ${r.final_policy}`.toLowerCase().includes(filter.toLowerCase()));
 return <section className="panel monitor-policies"><header className="section-heading"><h2>{t('策略命中与占比')}</h2><p>{t('规则策略与最终策略分别统计，每个维度每条请求只计一次；未执行策略求值的请求保留为未记录策略。')}</p></header>
  <div className="tabs" role="group" aria-label={t('策略统计维度')}>{[['policy','策略 / 策略组'],['final_policy','最终策略']].map(([key,label])=><button key={key} className={field===key?'selected':''} aria-pressed={field===key} onClick={()=>setField(key)}>{t(label)}</button>)}</div>
  {!snapshot.completed?<div className="empty">{t('暂无已完成请求')}</div>:<div className="monitor-bars">{bars.map((r,i)=><div className="monitor-bar-row" key={r.key}><span className="monitor-rank">{i+1}</span><span className="monitor-policy-name" title={name(r)}>{name(r)}</span><div className="monitor-bar-track"><span style={{width:`${share(r.count,snapshot.completed)*100}%`}}/></div><strong>{number(r.count)}</strong><small>{percent(r.count,snapshot.completed)}</small></div>)}</div>}
  {snapshot.policies.some(r=>r.other)&&<p className="field-hint">{t('最多保留 {limit} 种策略流向；新增流向超过容量时归入超出明细容量，汇总不丢失。',{limit})}</p>}
  <details className="monitor-detail"><summary>{t('查看策略流向明细')}</summary><div className="toolbar"><input aria-label={t('筛选策略')} placeholder={t('筛选策略')} value={filter} onChange={e=>setFilter(e.target.value)}/></div><div className="table-wrap monitor-policy-table"><table><thead><tr><th>{t('策略 / 策略组')}</th><th>{t('最终策略')}</th><th>{t('DNS 处理去向')}</th><th>{t('DNS 请求数')}</th><th>{t('占已完成请求')}</th></tr></thead><tbody>{rows.map((r,i)=><tr key={i}><td>{r.other?t('超出明细容量'):r.policy||t('未记录策略')}</td><td>{r.other?t('超出明细容量'):r.final_policy||t('未记录策略')}</td><td>{r.other?t('超出明细容量'):t(destinationLabels[r.destination]||'未发往解析器')}</td><td>{number(r.queries)}</td><td>{percent(r.queries,snapshot.completed)}</td></tr>)}</tbody></table>{!rows.length&&<div className="empty">{t('无匹配数据')}</div>}</div></details>
 </section>;
}

export function MonitoringPanel({snapshot,backendName,focus,onFocus,backendURL,historyStatus,clientSettings=[],loading=false,unavailable=false}){
 const [mode,setMode]=useState('live'),[range,setRange]=useState('3600'),[from,setFrom]=useState(''),[to,setTo]=useState(''),[source,setSource]=useState('');
 const historical=useMonitoringData({snapshot,historyStatus,mode,range,from,to,source});
 const t=useT(),[samples,setSamples]=useState([]),[now,setNow]=useState(Date.now()),receivedAt=useRef(Date.now());
 useEffect(()=>{if(snapshot){receivedAt.current=Date.now();setSamples(old=>appendSample(old,snapshot))}},[snapshot]);
 useEffect(()=>{const timer=setInterval(()=>setNow(Date.now()),3000);return()=>clearInterval(timer)},[]);
 if(!snapshot)return <><div className="monitor-toolbar"><strong>{backendName||t('本地服务')}</strong><button onClick={()=>onFocus(!focus)}>{t(focus?'退出大屏':'大屏模式')}</button></div><div className="notice">{t(unavailable?'监控数据暂不可用，请重新连接后端。':loading?'正在读取监控统计…':'此后端未提供大屏监控统计，请升级后端后查看。')}</div></>;
 const view=mode==='history'?historical.history?.summary:source?historical.selected:snapshot;
 const stale=now-receivedAt.current>10000,total=view?.completed||0;
 const metrics=view?[['DNS 流入总数',number(view.received)],['上游处理请求',number(view.flows.filter(f=>f.destination==='real'||f.destination==='surge').reduce((sum,f)=>sum+f.queries,0))],['已生成响应',number(total)],[mode==='history'?'窗口末处理中':'处理中',number(view.in_flight)],[mode==='history'?'期间新增未确认':'重启前未确认',number(view.unconfirmed)],['平均处理耗时',total?`${number(view.latency_ms_sum/total)} ms`:'—']]:[];
 const decisions=['DIRECT','SURGE','REJECT','UNKNOWN'].map((key,i)=>({key,label:key,count:view?.decisions[key]||0,color:['var(--success-text)','#8872db','var(--warning-text)','var(--muted)'][i]}));
 const rcodes=Object.entries(view?.response_codes||{}).sort((a,b)=>b[1]-a[1]).map(([key,count],i)=>({key,label:key,count,color:key==='NOERROR'?'var(--success-text)':key==='NXDOMAIN'?'var(--warning-text)':['var(--danger-text)','#8872db','var(--muted)'][i%3]}));
 return <div className="monitor-screen"><div className="monitor-toolbar"><div><strong>{backendName||t('本地服务')}</strong><span className={'badge '+(stale?'bad':'good')}>{t(mode==='history'?'历史数据':stale?'数据已过期':'每 3 秒刷新')}</span><small>{backendURL&&<>{backendURL} · </>}{t('统计起点')}: {new Date(snapshot.started_at).toLocaleString(currentLocale())} · {t('最近采样')}: {new Date(snapshot.observed_at).toLocaleTimeString(currentLocale())}</small></div><button onClick={()=>onFocus(!focus)}>{t(focus?'退出大屏':'大屏模式')}</button></div>
  <MonitoringControls mode={mode} onMode={setMode} range={range} onRange={setRange} from={from} onFrom={setFrom} to={to} onTo={setTo} source={source} onSource={setSource} sources={snapshot.sources||[]} snapshot={snapshot} reload={historical.reload} clientSettings={clientSettings}/>
  {historyStatus?.storage_error&&<div role="alert" className="notice error">{t('历史统计保存失败')}: {message(historyStatus.storage_error)}</div>}
  {historical.error&&<div role="alert" className="notice error">{message(historical.error)}<button onClick={historical.reload}>{t('重试')}</button></div>}
  {mode==='history'&&historical.loading&&<div className="notice">{t('正在读取历史数据…')}</div>}
  {mode==='history'&&!historical.loading&&!historical.error&&!historical.history&&<div className="notice">{t('此后端暂不支持历史监控，请升级后端。')}</div>}
  {mode==='history'&&historical.history&&<p className="field-hint">{new Date(historical.history.from).toLocaleString(currentLocale())} → {new Date(historical.history.to).toLocaleString(currentLocale())} · {t('历史保留七天，时间按分钟聚合。')}</p>}
  {source&&<p className="notice">{t('当前统计来源：{source}',{source:sourceName(source,t,clientSettings)})}</p>}
  {mode==='history'&&historical.history?.points.length===0?<div className="notice">{t('所选时间范围暂无历史采样')}</div>:view&&<><div className="metrics monitor-metrics">{metrics.map(([label,value])=><div className="metric" key={label}><span>{t(label)}</span><strong>{value}</strong></div>)}</div>
  <div className="monitor-flow-grid"><FlowChart snapshot={view} sources={mode==='history'?historical.history?.sources:snapshot.sources} selected={source} clientSettings={clientSettings}/><ClientPolicyChart rows={mode==='history'?historical.history?.source_policy_flows:view.source_policy_flows} clientSettings={clientSettings}/></div>
  <div className="monitor-primary-grid"><Policies snapshot={view} limit={source&&source!=='legacy'?64:512}/>
  <SourceStatistics sources={mode==='history'?historical.history?.sources:snapshot.sources} total={mode==='history'?historical.history?.total_received:snapshot.received} selected={source} onSelect={setSource} clientSettings={clientSettings}/></div>
  <div className="monitor-secondary-grid"><Trend samples={historical.history?historicalSamples(historical.history):mode==='history'||source?[]:samples} historical={mode==='history'} persisted={Boolean(historical.history)}/><Distribution title="分流决策占比" rows={decisions} total={total}/><Distribution title="DNS 响应码占比" rows={rcodes} total={total}/></div></>}
  <p className="monitor-footnote">{t('统计自监控功能启用起累计并跨重启保存，不依赖查询记录开关。流向表示 DNS 请求的最终处理路径；上游重试、后台刷新、网络字节和后续上网流量不在此统计范围。')}</p>
 </div>;
}
