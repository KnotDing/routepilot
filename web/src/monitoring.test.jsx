import React from 'react';
import {it,expect,afterEach,vi} from 'vitest';
import {render,screen,fireEvent,cleanup,act} from '@testing-library/react';
import {MonitoringPanel} from './monitoring';
import {appendSample,flowLayout,sourceFlows,policyDistribution} from './monitoring-data';

const snapshot={started_at:'2026-10-12T00:00:00Z',session_started_at:'session-a',observed_at:'2026-10-12T00:00:03Z',received:12,completed:10,in_flight:2,unconfirmed:0,latency_ms_sum:20,
 flows:[{ingress:'addressed',destination:'real',queries:3},{ingress:'addressed',destination:'cache',queries:2},{ingress:'intercepted',destination:'real',queries:1},{ingress:'intercepted',destination:'surge',queries:4}],
 policies:[{policy:'Proxy',final_policy:'Node A',queries:4},{policy:'Proxy',final_policy:'Node B',queries:3},{policy:'',final_policy:'',queries:2},{policy:'',final_policy:'',other:true,queries:1}],decisions:{DIRECT:3,SURGE:6,UNKNOWN:1},response_codes:{NOERROR:9,SERVFAIL:1}};
afterEach(()=>{cleanup();localStorage.clear();vi.useRealTimers()});

it('preserves exact edge totals and one scale for all ribbon and node heights',()=>{
 const {left,right,edges,scale}=flowLayout(snapshot.flows,10);
 expect(edges.reduce((s,e)=>s+e.count,0)).toBe(10);
 for(const n of left){expect(n.height).toBe(n.count*scale);expect(edges.filter(e=>e.source===n.key).reduce((s,e)=>s+e.count,0)).toBe(n.count)}
 for(const n of right){expect(n.height).toBe(n.count*scale);expect(edges.filter(e=>e.target===n.key).reduce((s,e)=>s+e.count,0)).toBe(n.count)}
 const tiny=flowLayout([{ingress:'addressed',destination:'cache',queries:1},{ingress:'addressed',destination:'real',queries:999999}],1000000);
 expect(tiny.right.find(n=>n.key==='cache').height/tiny.right.find(n=>n.key==='real').height).toBeCloseTo(1/999999,10);
});

it('counts each policy dimension once and keeps unknown and capacity overflow separate',()=>{
 const policies=policyDistribution(snapshot.policies,'policy'),final=policyDistribution(snapshot.policies,'final_policy');
 expect(policies.find(r=>r.name==='Proxy').count).toBe(7);
 expect(policies.find(r=>r.kind==='unnamed').count).toBe(2);
 expect(policies.find(r=>r.kind==='overflow').count).toBe(1);
 expect(final.find(r=>r.name==='Node A').count).toBe(4);
 for(const list of [policies,final])expect(list.reduce((s,r)=>s+r.count,0)).toBe(10);
});

it('keeps proportional bands positive when one client hits eleven policy nodes',()=>{
 const targets=Array.from({length:11},(_,i)=>`Policy ${i}`),flows=targets.map((destination,i)=>({ingress:'192.0.2.10',destination,queries:i+1}));
 const {left,right,edges,scale,height}=flowLayout(flows,66,['192.0.2.10'],targets);
 expect(scale).toBeGreaterThan(0);expect(edges.reduce((sum,e)=>sum+e.count,0)).toBe(66);
 expect(left[0].height).toBe(66*scale);
 for(const n of right){expect(n.height).toBe(n.count*scale);expect(n.y+n.height).toBeLessThanOrEqual(height+50)}
});

it('separates two-line labels for tiny clients and policies without inflating their bands',()=>{
 const clients=['large','tiny-a','tiny-b','tiny-c'],policies=['main',...Array.from({length:10},(_,i)=>`small-${i}`)];
 const flows=[{ingress:'large',destination:'main',queries:100000},...policies.slice(1).map((destination,i)=>({ingress:clients[1+i%3],destination,queries:1}))];
 const {left,right,scale}=flowLayout(flows,100010,clients,policies);
 for(const side of [left,right])for(let i=1;i<side.length;i++)expect(side[i].y+side[i].height/2-side[i-1].y-side[i-1].height/2).toBeGreaterThanOrEqual(52);
 expect(right[1].height).toBe(scale);expect(right[1].height/right[0].height).toBeCloseTo(1/100000,10);
});

it('uses observed source destinations and preserves shares when grouping small sources',()=>{
 const sources=Array.from({length:12},(_,i)=>({source:`192.0.2.${i+1}`,counts:{completed:i+1,flows:[{ingress:'addressed',destination:i%2?'real':'surge',queries:i+1}]}}));
 const total=78,result=sourceFlows({completed:total},sources),layout=flowLayout(result.flows,total,result.keys);
 expect(result.keys).toHaveLength(11);
 expect(result.flows.reduce((sum,f)=>sum+f.queries,0)).toBe(total);
 expect(result.flows.filter(f=>f.ingress==='remaining-sources')).toEqual(expect.arrayContaining([{ingress:'remaining-sources',destination:'surge',queries:1},{ingress:'remaining-sources',destination:'real',queries:2}]));
 expect(layout.edges.reduce((sum,e)=>sum+e.count,0)).toBe(total);
 expect(layout.scale).toBeGreaterThan(0);
 expect(sourceFlows({completed:total},sources.slice(1))).toBeNull();
 expect(sourceFlows(snapshot,[{source:'old',counts:{completed:10,flows:[]}}])).toBeNull();
 const selected=sourceFlows(snapshot,[],'192.0.2.10');
 expect(selected.flows.find(f=>f.destination==='real').queries).toBe(4);
});

it('calculates rates from actual elapsed time and breaks gaps, resets and duplicate samples',()=>{
 let points=appendSample([],snapshot);
 expect(points[0].incoming).toBeNull();
 points=appendSample(points,{...snapshot,observed_at:'2026-10-12T00:00:06Z',received:18,completed:13});
 expect(points.at(-1).incoming).toBe(2);expect(points.at(-1).outgoing).toBe(1);
 expect(appendSample(points,{...snapshot,observed_at:'2026-10-12T00:00:06Z',received:18,completed:13})).toBe(points);
 points=appendSample(points,{...snapshot,observed_at:'2026-10-12T00:01:06Z',received:100,completed:100});
 expect(points.at(-1).incoming).toBeNull();
 points=appendSample(points,{...snapshot,session_started_at:'session-b',observed_at:'2026-10-12T00:01:09Z'});
 expect(points).toHaveLength(1);expect(points[0].incoming).toBeNull();
});

it('shows exact denominators, policy paths and large-screen controls',()=>{
 const focus=vi.fn();render(<MonitoringPanel snapshot={snapshot} backendName="Remote A" focus={false} onFocus={focus}/>);
 expect(screen.getByText('Remote A')).toBeInTheDocument();
 expect(screen.getAllByText('70.0%')).toHaveLength(1);
 fireEvent.click(screen.getByRole('button',{name:'最终策略'}));
 expect(screen.getAllByText('Node A').length).toBeGreaterThan(0);
 fireEvent.click(screen.getByRole('button',{name:'大屏模式'}));expect(focus).toHaveBeenCalledWith(true);
 fireEvent.click(screen.getByText('查看策略流向明细'));
 fireEvent.change(screen.getByRole('textbox',{name:'筛选策略'}),{target:{value:'Node B'}});
 expect(screen.getByRole('cell',{name:'Node B'})).toBeInTheDocument();
 expect(screen.queryByRole('cell',{name:'Node A'})).not.toBeInTheDocument();
});

it('distinguishes old backend, connection failure and empty traffic without invented data',()=>{
 const {rerender}=render(<MonitoringPanel onFocus={()=>{}}/>);
 expect(screen.getByText(/此后端未提供/)).toBeInTheDocument();
 rerender(<MonitoringPanel unavailable onFocus={()=>{}}/>);expect(screen.getByText(/监控数据暂不可用/)).toBeInTheDocument();
 rerender(<MonitoringPanel snapshot={{...snapshot,received:0,completed:0,flows:[],policies:[],decisions:{},response_codes:{}}} onFocus={()=>{}}/>);
 expect(screen.getAllByText('暂无已完成请求')).toHaveLength(4);
 expect(screen.getByText('等待下一次采样以计算速率')).toBeInTheDocument();
});

it('shows unavailable rates after a sampling gap and ages data using receipt time',()=>{
 vi.useFakeTimers();
 const {rerender}=render(<MonitoringPanel snapshot={snapshot} onFocus={()=>{}}/>);
 // An old server clock does not make a freshly received response stale.
 expect(screen.getByText('每 3 秒刷新')).toBeInTheDocument();
 rerender(<MonitoringPanel snapshot={{...snapshot,observed_at:'2026-10-12T00:00:06Z',received:18,completed:13}} onFocus={()=>{}}/>);
 expect(document.querySelector('.monitor-rate-legend')).toHaveTextContent('流入: 2');
 rerender(<MonitoringPanel snapshot={{...snapshot,observed_at:'2026-10-12T00:01:06Z',received:100,completed:100}} onFocus={()=>{}}/>);
 expect(document.querySelector('.monitor-rate-legend')).toHaveTextContent('流入: —');
 act(()=>vi.advanceTimersByTime(12000));
 expect(screen.getByText('数据已过期')).toBeInTheDocument();
});

it('keeps exit available when monitoring data is lost in large-screen mode',()=>{
 const focus=vi.fn();render(<MonitoringPanel focus unavailable onFocus={focus}/>);
 fireEvent.click(screen.getByRole('button',{name:'退出大屏'}));
 expect(focus).toHaveBeenCalledWith(false);
});

it('keeps the middle column identical across both stages and preserves the common scale',()=>{
 const first=flowLayout([{ingress:'a',destination:'Proxy',queries:7},{ingress:'b',destination:'Proxy',queries:3}],10,['a','b'],['Proxy'],3,{sourceX:210,targetX:600,nodeCount:2});
 const second=flowLayout([{ingress:'Proxy',destination:'Node A',queries:6},{ingress:'Proxy',destination:'Node B',queries:4}],10,['Proxy'],['Node A','Node B'],3,{sourceX:600,targetX:1040,nodeCount:2});
 expect(first.scale).toBe(second.scale);expect(first.right[0].y).toBe(second.left[0].y);expect(first.right[0].height).toBe(second.left[0].height);
 expect(first.edges.reduce((sum,e)=>sum+e.count,0)).toBe(10);expect(second.edges.reduce((sum,e)=>sum+e.count,0)).toBe(10);
});
