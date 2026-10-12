import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,cleanup,waitFor,act} from '@testing-library/react';
import {MonitoringPanel} from './monitoring';
import {historyRange,historicalSamples} from './monitoring-history';
import {selectBackend} from './client';

const base={started_at:'2026-10-12T00:00:00Z',session_started_at:'a',observed_at:'2026-10-12T00:03:15Z',unconfirmed:0,in_flight:0,latency_ms_sum:10};
const a={...base,received:4,completed:4,decisions:{SURGE:4},response_codes:{NOERROR:4},flows:[{ingress:'addressed',destination:'surge',queries:4}],policies:[{policy:'Proxy',final_policy:'Node A',destination:'surge',queries:4}]};
const b={...base,received:6,completed:6,decisions:{DIRECT:6},response_codes:{NOERROR:6},flows:[{ingress:'intercepted',destination:'real',queries:6}],policies:[{policy:'DIRECT',final_policy:'DIRECT',destination:'real',queries:6}]};
const snapshot={...base,received:10,completed:10,decisions:{SURGE:4,DIRECT:6},response_codes:{NOERROR:10},flows:[...a.flows,...b.flows],policies:[...a.policies,...b.policies],sources:[{source:'192.0.2.10',counts:a},{source:'192.0.2.20',counts:b}]};
const props={snapshot,historyStatus:{supported:true},onFocus:()=>{}};
const history=counts=>({from:'2026-10-12T00:00:00Z',to:'2026-10-12T00:04:00Z',summary:counts,sources:snapshot.sources,total_received:10,points:[{from:'2026-10-12T00:03:10Z',to:'2026-10-12T00:03:15Z',seconds:5,rate_valid:true,counts}]});
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
 fireEvent.click(screen.getByRole('button',{name:'查看 192.0.2.10 的流向'}));
 await waitFor(()=>expect(document.querySelector('.monitor-policy-name')).toHaveTextContent('Proxy'));
 expect(document.querySelector('.monitor-bars small')).toHaveTextContent('100.0%');
 expect(document.querySelectorAll('.monitor-policy-name')).toHaveLength(1);
 fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));
 await screen.findByText('窗口末处理中');
 expect(document.querySelector('.monitor-policy-name')).toHaveTextContent('Proxy');
 fireEvent.change(screen.getByLabelText('请求来源'),{target:{value:'192.0.2.20'}});
 await waitFor(()=>expect(document.querySelector('.monitor-policy-name')).toHaveTextContent('DIRECT'));
 expect(document.querySelector('.monitor-bars small')).toHaveTextContent('100.0%');
 expect(document.querySelectorAll('.monitor-policy-name')).toHaveLength(1);
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
 await waitFor(()=>expect(document.querySelector('.monitor-policy-name')).toHaveTextContent('DIRECT'));
 await act(async()=>releaseA());
 expect(document.querySelector('.monitor-policy-name')).toHaveTextContent('DIRECT');
 expect(screen.getByText('当前统计来源：192.0.2.20')).toBeInTheDocument();
});

it('shows a localized historical-query failure without fabricated statistics',async()=>{
 vi.stubGlobal('fetch',vi.fn(async()=>({ok:false,status:503,text:async()=>JSON.stringify({error:'monitoring history storage is unavailable'})})));
 render(<MonitoringPanel {...props}/>);fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));
 expect(await screen.findByRole('alert')).toHaveTextContent('历史监控存储暂不可用');expect(document.querySelector('.monitor-metrics')).toBeNull();
 expect(screen.getByRole('button',{name:'重试'})).toBeInTheDocument();
});
