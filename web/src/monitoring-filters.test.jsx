import React from 'react';
import {it,expect,vi,afterEach} from 'vitest';
import {render,screen,fireEvent,waitFor,cleanup,act} from '@testing-library/react';
import {MonitoringPanel} from './monitoring';
import {saveMonitoringPreferences} from './monitoring-preferences';
import {request} from './client';
vi.mock('./client',()=>({request:vi.fn()}));
const p=JSON.stringify(['named','Private group']),f=JSON.stringify(['named','Private node']);
const options={destinations:['cache','surge'],policies:[{value:p,name:'Private group',kind:'named'}],final_policies:[{value:f,name:'Private node',kind:'named'}],domains:['private.example','other.example'],response_codes:['NOERROR','NXDOMAIN']};
const rows=[{source:'192.0.2.1',domain:'private.example',destination:'cache',code:'NXDOMAIN',n:3},{source:'192.0.2.1',domain:'other.example',destination:'surge',code:'NOERROR',n:2},{source:'192.0.2.2',domain:'private.example',destination:'cache',code:'NOERROR',n:5}];
function counts(list){
 const completed=list.reduce((sum,row)=>sum+row.n,0),sources=[...new Set(list.map(row=>row.source))].map(source=>({source,counts:countsFor(list.filter(row=>row.source===source))}));
 return {...countsFor(list),sources,domains:[...new Set(list.map(row=>row.domain))].map(domain=>({domain,queries:list.filter(row=>row.domain===domain).reduce((sum,row)=>sum+row.n,0)})),dimension_completed:completed,filter_options:options,source_policy_flows:list.map(row=>({source:row.source,policy:'Private group',kind:'named',final_policy:'Private node',final_kind:'named',queries:row.n}))};
}
function countsFor(list){const n=list.reduce((sum,row)=>sum+row.n,0);return {started_at:'2026-10-13T00:00:00Z',observed_at:'2026-10-13T00:01:00Z',session_started_at:'session',received:n,completed:n,in_flight:0,unconfirmed:0,latency_ms_sum:n,dimensions_supported:true,decisions:{SURGE:n},response_codes:Object.fromEntries(['NOERROR','NXDOMAIN'].map(code=>[code,list.filter(row=>row.code===code).reduce((sum,row)=>sum+row.n,0)])),flows:list.map(row=>({ingress:'addressed',destination:row.destination,queries:row.n})),policies:[{policy:'Private group',final_policy:'Private node',queries:n}]}}
function serve(){request.mockImplementation(async path=>{const q=new URL(path,'http://fixture/'),params=q.searchParams,list=rows.filter(row=>(!params.get('source')||row.source===params.get('source'))&&(!params.get('domain')||row.domain===params.get('domain'))&&(!params.get('destination')||row.destination===params.get('destination'))&&(!params.get('response_code')||row.code===params.get('response_code'))),value={...counts(list),completed_only:['domain','destination','policy','final_policy','response_code'].some(key=>params.has(key))};return q.pathname.endsWith('/history')?{from:value.started_at,to:value.observed_at,summary:value,sources:value.sources,source_policy_flows:value.source_policy_flows,points:[{from:value.started_at,to:value.observed_at,seconds:60,rate_valid:true,counts:value}]}:value})}
afterEach(()=>{cleanup();localStorage.clear();vi.resetAllMocks()});
it('applies all six filters to live totals, rankings and the historical view',async()=>{
 serve();render(<MonitoringPanel snapshot={counts(rows)} historyStatus={{supported:true}} onFocus={()=>{}}/>);
 await screen.findByRole('button',{name:'筛选域名：private.example'});expect(document.querySelector('.monitor-domain-shares')).toHaveTextContent('80.00%');
 fireEvent.click(screen.getByRole('button',{name:'筛选域名：private.example'}));await waitFor(()=>expect(document.querySelector('.monitor-metrics')).toHaveTextContent('筛选完成请求8'));
 for(const [name,value] of [['请求来源','192.0.2.1'],['DNS 处理去向','cache'],['策略 / 策略组',p],['最终策略',f],['响应码','NXDOMAIN']]){fireEvent.change(screen.getByRole('combobox',{name,exact:true}),{target:{value}});await waitFor(()=>expect(document.querySelector('.monitor-metrics')).toHaveTextContent('筛选完成请求3'))}
 expect(document.querySelector('.monitor-domain-shares')).toHaveTextContent('100.00%');expect(document.querySelector('.monitor-source-shares')).not.toHaveTextContent('192.0.2.2');expect(document.querySelector('.monitor-flow')).not.toHaveTextContent('Surge DNS');
 fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));await screen.findByText('窗口末处理中');expect(document.querySelector('.monitor-metrics')).toHaveTextContent('筛选完成请求3');
 const last=request.mock.calls.filter(([path])=>path.includes('/history?')).at(-1)[0],query=new URL(last,'http://fixture/').searchParams;for(const [key,value] of Object.entries({source:'192.0.2.1',destination:'cache',policy:p,final_policy:f,domain:'private.example',response_code:'NXDOMAIN'}))expect(query.get(key)).toBe(value);
 fireEvent.change(screen.getByRole('combobox',{name:'响应码',exact:true}),{target:{value:'NOERROR'}});await waitFor(()=>expect(document.querySelector('.monitor-metrics')).toHaveTextContent('筛选完成请求0'));expect(screen.getByText('暂无域名统计')).toBeInTheDocument();
});
it('protects domain and policy values in options and rankings while sending true identities',async()=>{
 serve();await saveMonitoringPreferences({protectSensitive:true});const {container}=render(<MonitoringPanel snapshot={counts(rows)} onFocus={()=>{}}/>);
 await screen.findByRole('button',{name:'筛选域名：域名 1',exact:true});for(const secret of ['private.example','other.example','Private group','Private node','192.0.2.1','192.0.2.2'])expect(container.innerHTML).not.toContain(secret);
 const domain=screen.getByRole('combobox',{name:'域名',exact:true}),option=domain.options[1].value;fireEvent.change(domain,{target:{value:option}});await waitFor(()=>expect(request.mock.calls.some(([path])=>new URL(path,'http://fixture/').searchParams.get('domain')===options.domains[0])).toBe(true));
 await act(async()=>{});for(const secret of ['private.example','Private group','Private node'])expect(container.innerHTML).not.toContain(secret);
});

it('reports unavailable old historical dimensions without discarding working live statistics',async()=>{
 serve();const handler=request.getMockImplementation();request.mockImplementation(path=>{const url=new URL(path,'http://fixture/');if(url.pathname.endsWith('/history')&&url.searchParams.has('domain'))return Promise.reject(Object.assign(new Error('monitoring dimensions are unavailable for this history range'),{status:422}));return handler(path)});
 render(<MonitoringPanel snapshot={counts(rows)} historyStatus={{supported:true}} onFocus={()=>{}}/>);await screen.findByRole('button',{name:'筛选域名：private.example'});fireEvent.change(screen.getByRole('combobox',{name:'域名',exact:true}),{target:{value:'private.example'}});
 await waitFor(()=>expect(document.querySelector('.monitor-metrics')).toHaveTextContent('筛选完成请求8'));expect(await screen.findByRole('alert')).toHaveTextContent('此时段尚未采集组合筛选维度');
 fireEvent.click(screen.getByRole('button',{name:'历史数据',exact:true}));await waitFor(()=>expect(document.querySelector('.monitor-metrics')).toBeNull());expect(await screen.findByRole('alert')).toHaveTextContent('无法按这些条件统计');
});
