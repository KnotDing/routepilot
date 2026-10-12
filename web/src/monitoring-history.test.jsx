import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,act} from '@testing-library/react';
import {MonitoringPanel} from './monitoring';
import {historyRange,historicalSamples} from './monitoring-history';
import {selectBackend} from './client';

const base={started_at:'2026-10-12T00:00:00Z',session_started_at:'a',observed_at:'2026-10-12T00:03:15Z',unconfirmed:0,in_flight:0,latency_ms_sum:10};
const a={...base,received:4,completed:4,decisions:{SURGE:4},response_codes:{NOERROR:4},flows:[{ingress:'addressed',destination:'surge',queries:4}],policies:[{policy:'Proxy',final_policy:'Node A',destination:'surge',queries:4}]};
const b={...base,received:6,completed:6,decisions:{DIRECT:6},response_codes:{NOERROR:6},flows:[{ingress:'intercepted',destination:'real',queries:6}],policies:[{policy:'DIRECT',final_policy:'DIRECT',destination:'real',queries:6}]};
a.source_policy_flows=[{source:'192.0.2.10',kind:'named',policy:'Proxy',final_kind:'named',final_policy:'Node A',queries:4}];
b.source_policy_flows=[{source:'192.0.2.20',kind:'named',policy:'DIRECT',final_kind:'named',final_policy:'DIRECT',queries:6}];
const snapshot={source_policy_flows:[...a.source_policy_flows,...b.source_policy_flows],...base,received:10,completed:10,decisions:{SURGE:4,DIRECT:6},response_codes:{NOERROR:10},flows:[...a.flows,...b.flows],policies:[...a.policies,...b.policies],sources:[{source:'192.0.2.10',counts:a},{source:'192.0.2.20',counts:b}]};
const props={snapshot,historyStatus:{supported:true},onFocus:()=>{}};
const history=counts=>({from:'2026-10-12T00:00:00Z',to:'2026-10-12T00:04:00Z',summary:counts,source_policy_flows:counts.source_policy_flows,sources:snapshot.sources,total_received:10,points:[{from:'2026-10-12T00:03:10Z',to:'2026-10-12T00:03:15Z',seconds:5,rate_valid:true,counts}]});
function valueFor(url){const q=new URL(url,'http://localhost'),source=q.searchParams.get('source'),counts=source==='192.0.2.10'?a:source==='192.0.2.20'?b:snapshot;return q.pathname.endsWith('/history')?history(counts):counts}
afterEach(()=>{cleanup();localStorage.clear();selectBackend('');vi.unstubAllGlobals()});

it('uses server time for whole-minute windows and rejects invalid custom ranges',()=>{
 expect(historyRange(snapshot,'live','3600','','')).toEqual({from:'2026-10-11T23:58:00.000Z',to:'2026-10-12T00:04:00.000Z'});
 expect(()=>historyRange(snapshot,'history','custom','bad','bad')).toThrow();
 expect(()=>historyRange(snapshot,'history','custom','2026-10-01T00:00:00Z','2026-10-12T00:00:00Z')).toThrow();
 const points=historicalSamples({points:[{from:'2026-10-12T00:00:00Z',to:'2026-10-12T00:01:00Z',seconds:60,rate_valid:true,counts:{received:120,completed:60}},{from:'2026-10-12T00:03:00Z',to:'2026-10-12T00:04:00Z',seconds:60,rate_valid:true,counts:{received:60,completed:60}}]});
 expect(points[0].incoming).toBe(2);expect(points[1].breakBefore).toBe(true);
});

it('filters live and historical policy paths by source and uses that source denominator',async()=>{
 vi.stubGlobal('fetch',vi.fn(async url=>({ok:true,status:200,json:async()=>valueFor(url)})));
 render(<MonitoringPanel {...props}/>);
 expect(screen.getByRole('img',{name:'客户端到策略或策略组的流向图'})).toHaveTextContent('192.0.2.10 → Proxy: 4');
 expect(screen.getByRole('img',{name:'客户端到策略或策略组的流向图'})).not.toHaveTextContent('192.0.2.20 → Proxy');
 expect(screen.queryByText('按源统计 DNS 流入')).not.toBeInTheDocument();
 expect(screen.getByRole('heading',{name:'请求来源占比'}).closest('.monitor-flow')).toBeNull();
 expect(screen.getByRole('heading',{name:'请求来源占比'}).closest('.panel')).toHaveClass('monitor-source-shares');
 expect(screen.getByRole('img',{name:'DNS 来源到实际处理去向的流向图'})).toHaveTextContent('192.0.2.10 → Surge DNS: 4');
 expect(screen.getByRole('img',{name:'DNS 来源到实际处理去向的流向图'})).not.toHaveTextContent('直接请求');
 expect(document.querySelector('.monitor-source-bars')).toHaveTextContent('40.00%');
 fireEvent.click(screen.getByRole('button',{name:'查看 192.0.2.10 的流向'}));
 await waitFor(()=>expect(document.querySelector('.monitor-policies .monitor-policy-name')).toHaveTextContent('Proxy'));
 expect(document.querySelector('.monitor-bars small')).toHaveTextContent('100.0%');
 expect(document.querySelectorAll('.monitor-policies .monitor-policy-name')).toHaveLength(1);
 fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));
 await screen.findByText('窗口末处理中');
 expect(document.querySelector('.monitor-policies .monitor-policy-name')).toHaveTextContent('Proxy');
 fireEvent.change(screen.getByLabelText('请求来源'),{target:{value:'192.0.2.20'}});
 await waitFor(()=>expect(document.querySelector('.monitor-policies .monitor-policy-name')).toHaveTextContent('DIRECT'));
 expect(document.querySelector('.monitor-bars small')).toHaveTextContent('100.0%');
 expect(document.querySelectorAll('.monitor-policies .monitor-policy-name')).toHaveLength(1);
});

it('ignores an old source response after selection changes',async()=>{
 let releaseA;
 vi.stubGlobal('fetch',vi.fn(url=>{
  const q=new URL(url,'http://localhost');
  if(q.pathname.endsWith('/monitoring')&&q.searchParams.get('source')==='192.0.2.10')return new Promise(resolve=>{releaseA=()=>resolve({ok:true,status:200,json:async()=>a})});
  return Promise.resolve({ok:true,status:200,json:async()=>valueFor(url)});
 }));
 render(<MonitoringPanel {...props}/>);
 fireEvent.change(screen.getByLabelText('请求来源'),{target:{value:'192.0.2.10'}});
 await waitFor(()=>expect(releaseA).toBeTypeOf('function'));
 fireEvent.change(screen.getByLabelText('请求来源'),{target:{value:'192.0.2.20'}});
 await waitFor(()=>expect(document.querySelector('.monitor-policies .monitor-policy-name')).toHaveTextContent('DIRECT'));
 await act(async()=>releaseA());
 expect(document.querySelector('.monitor-policies .monitor-policy-name')).toHaveTextContent('DIRECT');
 expect(screen.getByText('当前统计来源：192.0.2.20')).toBeInTheDocument();
});

it('excludes legacy from source denominators in live and history while retaining stored totals',async()=>{
 const recorded={...snapshot,received:16,completed:16,flows:[...snapshot.flows,...b.flows],sources:[...snapshot.sources,{source:'legacy',counts:b}]};
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:true,status:200,json:async()=>({...history(recorded),sources:recorded.sources,total_received:16})})));
 render(<MonitoringPanel {...props} snapshot={recorded}/>);
 const check=()=>{
  expect(screen.queryByText('启用前未采集来源')).not.toBeInTheDocument();
  expect(screen.queryByRole('option',{name:'启用前未采集来源'})).not.toBeInTheDocument();
  expect(document.querySelector('.monitor-source-bars')).toHaveTextContent('40.00%');
  expect(document.querySelector('.monitor-source-bars')).toHaveTextContent('60.00%');
  expect(document.querySelector('.monitor-metrics')).toHaveTextContent('16');
  expect(screen.getByRole('img',{name:'DNS 来源到实际处理去向的流向图'})).toHaveTextContent('192.0.2.10 → Surge DNS: 4 · 40.0%');
 };
 check();
 fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));
 await screen.findByText('窗口末处理中');check();
});

it('shows a localized historical-query failure without fabricated statistics',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:false,status:503,text:async()=>JSON.stringify({error:'monitoring history storage is unavailable'})})));
 render(<MonitoringPanel {...props}/>);fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));
 expect(await screen.findByRole('alert')).toHaveTextContent('历史监控存储暂不可用');expect(document.querySelector('.monitor-metrics')).toBeNull();
 expect(screen.getByRole('button',{name:'重试'})).toBeInTheDocument();
});

it('shows configured client names while keeping source requests keyed by IP',async()=>{
 vi.stubGlobal('fetch',vi.fn(async url=>({ok:true,status:200,json:async()=>valueFor(url)})));
 render(<MonitoringPanel {...props} clientSettings={[{ip:'192.0.2.10',name:'工作电脑'}]}/>);
 fireEvent.click(screen.getByRole('button',{name:'查看 工作电脑 (192.0.2.10) 的流向'}));
 await screen.findByText('当前统计来源：工作电脑 (192.0.2.10)');
 expect(fetch.mock.calls.some(([url])=>new URL(url,'http://localhost').searchParams.get('source')==='192.0.2.10')).toBe(true);
});

it('shows names alone in both charts and toggles them to IPs without changing source filters',()=>{
 render(<MonitoringPanel {...props} clientSettings={[{ip:'192.0.2.10',name:'工作电脑'}]}/>);
 for(const css of ['.monitor-flow','.monitor-client-policy']){
  const chart=document.querySelector(css),label=chart.querySelector('.monitor-source-label[role=button]');
  expect(label.querySelector('tspan')).toHaveTextContent('工作电脑');expect(label.querySelector('tspan')).not.toHaveTextContent('192.0.2.10');
  fireEvent.click(label);expect(label.querySelector('tspan')).toHaveTextContent('192.0.2.10');
  fireEvent.keyDown(label,{key:'Enter'});expect(label.querySelector('tspan')).toHaveTextContent('工作电脑');
 }
 expect(screen.getByLabelText('请求来源')).toHaveValue('');
 expect(document.querySelector('.monitor-client-policy svg')).toHaveTextContent('Proxy → Node A: 4');
 expect(document.querySelector('.monitor-source-shares table')).toBeNull();
});

it('degrades old policy-only rows without inventing final policy flows',()=>{
 const old={...snapshot,source_policy_flows:snapshot.source_policy_flows.map(({final_kind,final_policy,...row})=>row)};
 render(<MonitoringPanel {...props} snapshot={old}/>);
 expect(screen.getByText('此后端未提供最终策略流向，暂展示客户端到命中策略。')).toBeInTheDocument();
 expect(document.querySelector('.monitor-client-policy svg')).not.toHaveTextContent('Node A');
});
